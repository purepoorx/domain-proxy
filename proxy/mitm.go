package proxy

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"

	"domain-proxy/cert"
)

type MITMProxy struct {
	certStore *cert.Store
	rewriter  *Rewriter
}

func NewMITMProxy(certStore *cert.Store, rewriter *Rewriter) *MITMProxy {
	return &MITMProxy{
		certStore: certStore,
		rewriter:  rewriter,
	}
}

func (m *MITMProxy) HandleConnect(w http.ResponseWriter, r *http.Request) {
	host, port, err := parseHostPort(r.Host, "443")
	if err != nil {
		http.Error(w, "bad host", http.StatusBadRequest)
		return
	}

	slog.Info("CONNECT request", "host", host, "port", port)

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		slog.Error("hijack failed", "error", err)
		return
	}

	_, err = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	if err != nil {
		clientConn.Close()
		slog.Error("write 200 failed", "error", err)
		return
	}

	_, _, needMITM := m.rewriter.Rewrite(host)
	if needMITM {
		go m.doMITM(clientConn, host, port)
	} else {
		go m.doPlainTunnel(clientConn, host, port)
	}
}

func (m *MITMProxy) doMITM(clientConn net.Conn, originalHost, port string) {
	defer clientConn.Close()

	tlsCert, err := m.certStore.GetOrCreateCert(originalHost)
	if err != nil {
		slog.Error("get cert failed", "host", originalHost, "error", err)
		return
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{*tlsCert},
	}
	tlsConn := tls.Server(clientConn, tlsConfig)
	if err := tlsConn.Handshake(); err != nil {
		slog.Error("TLS handshake with client failed", "host", originalHost, "error", err)
		return
	}
	defer tlsConn.Close()

	reader := bufio.NewReader(tlsConn)
	for {
		req, err := http.ReadRequest(reader)
		if err != nil {
			if err != io.EOF {
				slog.Debug("read request from client failed", "error", err)
			}
			return
		}

		targetHost, injectHeader, _ := m.rewriter.Rewrite(originalHost)

		if req.URL.Host == "" {
			req.URL.Host = originalHost
		}
		req.URL.Scheme = "https"
		req.URL.Host = targetHost
		req.Host = targetHost
		if injectHeader {
			req.Header.Set("X-Target-Host", originalHost)
		}

		slog.Info("MITM forwarding",
			"method", req.Method,
			"original", originalHost,
			"target", targetHost,
			"path", req.URL.Path,
		)

		targetAddr := net.JoinHostPort(targetHost, port)
		serverTLSConn, err := tls.Dial("tcp", targetAddr, &tls.Config{
			ServerName: targetHost,
		})
		if err != nil {
			slog.Error("connect to target failed", "target", targetAddr, "error", err)
			writeErrorResponse(tlsConn, http.StatusBadGateway, "failed to connect to upstream")
			return
		}

		if err := req.Write(serverTLSConn); err != nil {
			serverTLSConn.Close()
			slog.Error("write request to target failed", "error", err)
			return
		}

		resp, err := http.ReadResponse(bufio.NewReader(serverTLSConn), req)
		if err != nil {
			serverTLSConn.Close()
			slog.Error("read response from target failed", "error", err)
			writeErrorResponse(tlsConn, http.StatusBadGateway, "failed to read upstream response")
			return
		}

		if err := resp.Write(tlsConn); err != nil {
			resp.Body.Close()
			serverTLSConn.Close()
			slog.Error("write response to client failed", "error", err)
			return
		}

		resp.Body.Close()
		serverTLSConn.Close()
	}
}

func (m *MITMProxy) doPlainTunnel(clientConn net.Conn, host, port string) {
	defer clientConn.Close()

	targetAddr := net.JoinHostPort(host, port)
	serverConn, err := net.Dial("tcp", targetAddr)
	if err != nil {
		slog.Error("connect to target failed", "target", targetAddr, "error", err)
		return
	}
	defer serverConn.Close()

	relay(clientConn, serverConn)
}

func (m *MITMProxy) HandleMITMConn(clientConn net.Conn, host, port string) {
	m.doMITM(clientConn, host, port)
}

func (m *MITMProxy) HandlePlainTunnel(clientConn net.Conn, host, port string) {
	m.doPlainTunnel(clientConn, host, port)
}

func relay(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	copyConn := func(dst, src net.Conn) {
		defer wg.Done()
		io.Copy(dst, src)
	}

	go copyConn(a, b)
	go copyConn(b, a)
	wg.Wait()
}

func parseHostPort(addr, defaultPort string) (string, string, error) {
	if !strings.Contains(addr, ":") {
		return addr, defaultPort, nil
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", "", fmt.Errorf("split host port %q: %w", addr, err)
	}
	if port == "" {
		port = defaultPort
	}
	return host, port, nil
}

func writeErrorResponse(w io.Writer, statusCode int, msg string) {
	resp := &http.Response{
		StatusCode: statusCode,
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
	}
	resp.Header.Set("Content-Type", "text/plain")
	resp.Body = io.NopCloser(strings.NewReader(msg))
	resp.ContentLength = int64(len(msg))
	resp.Write(w)
}
