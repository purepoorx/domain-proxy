package proxy

import (
	"bufio"
	"fmt"
	"log/slog"
	"net"

	"domain-proxy/cert"
)

type Server struct {
	addr      string
	httpProxy *HTTPProxy
	socks5    *SOCKS5Handler
}

func NewServer(addr string, certStore *cert.Store, rewriter *Rewriter) *Server {
	mitm := NewMITMProxy(certStore, rewriter)
	return &Server{
		addr:      addr,
		httpProxy: NewHTTPProxy(addr, certStore, rewriter, mitm),
		socks5:    NewSOCKS5Handler(mitm, rewriter),
	}
}

func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	slog.Info("proxy listening (HTTP + SOCKS5)", "addr", s.addr)

	go s.httpProxy.Serve(ln)

	// Block forever — httpProxy.Serve handles accept loop
	select {}
}

// PeekConn wraps a net.Conn with a buffered reader for peeking at the first byte.
type PeekConn struct {
	net.Conn
	reader *bufio.Reader
}

func NewPeekConn(conn net.Conn) *PeekConn {
	return &PeekConn{
		Conn:   conn,
		reader: bufio.NewReader(conn),
	}
}

func (c *PeekConn) Read(b []byte) (int, error) {
	return c.reader.Read(b)
}

func (c *PeekConn) Peek(n int) ([]byte, error) {
	return c.reader.Peek(n)
}
