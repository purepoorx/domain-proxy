package proxy

import (
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"domain-proxy/cert"
)

type HTTPProxy struct {
	addr     string
	mitm     *MITMProxy
	rewriter *Rewriter
	socks5   *SOCKS5Handler
}

func NewHTTPProxy(addr string, certStore *cert.Store, rewriter *Rewriter, mitm *MITMProxy) *HTTPProxy {
	return &HTTPProxy{
		addr:     addr,
		mitm:     mitm,
		rewriter: rewriter,
		socks5:   NewSOCKS5Handler(mitm, rewriter),
	}
}

func (p *HTTPProxy) Serve(ln net.Listener) error {
	server := &http.Server{
		Handler: p,
		ConnState: func(conn net.Conn, state http.ConnState) {},
	}
	server.ConnContext = nil

	// Wrap listener to peek first byte and dispatch SOCKS5
	wrappedLn := &peekListener{
		Listener: ln,
		socks5:   p.socks5,
	}

	slog.Info("HTTP proxy listening", "addr", p.addr)
	return server.Serve(wrappedLn)
}

func (p *HTTPProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		p.mitm.HandleConnect(w, r)
		return
	}

	p.handlePlainHTTP(w, r)
}

func (p *HTTPProxy) handlePlainHTTP(w http.ResponseWriter, r *http.Request) {
	host := r.URL.Hostname()
	if target, injectHeader, rewritten := p.rewriter.Rewrite(host); rewritten {
		r.URL.Host = target
		r.Host = target
		if injectHeader {
			r.Header.Set("X-Target-Host", host)
		}
	}

	transport := &http.Transport{
		DialContext:            (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}

	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, r.URL.String(), r.Body)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	outReq.Header = r.Header.Clone()

	resp, err := transport.RoundTrip(outReq)
	if err != nil {
		slog.Error("upstream request failed", "error", err)
		http.Error(w, "upstream error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// peekListener wraps a net.Listener and peeks at the first byte of each
// connection to detect SOCKS5 traffic (first byte 0x05). SOCKS5 connections
// are dispatched directly; HTTP connections are passed to the http.Server.
type peekListener struct {
	net.Listener
	socks5 *SOCKS5Handler
}

func (l *peekListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}

		pc := NewPeekConn(conn)
		firstByte, err := pc.Peek(1)
		if err != nil {
			conn.Close()
			continue
		}

		if firstByte[0] == 0x05 {
			// SOCKS5 connection — handle in goroutine, then loop back to accept
			go l.socks5.HandleConn(pc)
			continue
		}

		// HTTP connection — return to http.Server
		return pc, nil
	}
}
