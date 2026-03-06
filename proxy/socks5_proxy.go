package proxy

import (
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net"
)

const (
	socks5Version = 0x05

	authNone     = 0x00
	authNoAccept = 0xFF

	cmdConnect = 0x01

	atypIPv4   = 0x01
	atypDomain = 0x03
	atypIPv6   = 0x04

	repSuccess          = 0x00
	repGeneralFailure   = 0x01
	repConnRefused      = 0x05
	repAddrNotSupported = 0x08
)

type SOCKS5Handler struct {
	mitm     *MITMProxy
	rewriter *Rewriter
}

func NewSOCKS5Handler(mitm *MITMProxy, rewriter *Rewriter) *SOCKS5Handler {
	return &SOCKS5Handler{
		mitm:     mitm,
		rewriter: rewriter,
	}
}

func (s *SOCKS5Handler) HandleConn(conn net.Conn) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("socks5 panic recovered", "error", r)
		}
	}()

	if err := s.handshake(conn); err != nil {
		slog.Error("socks5 handshake failed", "error", err)
		conn.Close()
		return
	}

	host, port, err := s.readRequest(conn)
	if err != nil {
		slog.Error("socks5 read request failed", "error", err)
		conn.Close()
		return
	}

	slog.Info("SOCKS5 CONNECT", "host", host, "port", port)

	s.sendReply(conn, repSuccess)

	if port == "443" {
		if _, rewritten := s.rewriter.Rewrite(host); rewritten {
			s.mitm.HandleMITMConn(conn, host, port)
			return
		}
	}

	s.mitm.HandlePlainTunnel(conn, host, port)
}

func (s *SOCKS5Handler) handshake(conn net.Conn) error {
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return fmt.Errorf("read handshake header: %w", err)
	}

	if header[0] != socks5Version {
		return fmt.Errorf("unsupported SOCKS version: %d", header[0])
	}

	numMethods := int(header[1])
	methods := make([]byte, numMethods)
	if _, err := io.ReadFull(conn, methods); err != nil {
		return fmt.Errorf("read methods: %w", err)
	}

	supportsNoAuth := false
	for _, m := range methods {
		if m == authNone {
			supportsNoAuth = true
			break
		}
	}

	if !supportsNoAuth {
		conn.Write([]byte{socks5Version, authNoAccept})
		return fmt.Errorf("no acceptable auth method")
	}

	_, err := conn.Write([]byte{socks5Version, authNone})
	return err
}

func (s *SOCKS5Handler) readRequest(conn net.Conn) (string, string, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return "", "", fmt.Errorf("read request header: %w", err)
	}

	if header[0] != socks5Version {
		return "", "", fmt.Errorf("unsupported version: %d", header[0])
	}

	if header[1] != cmdConnect {
		s.sendReply(conn, repGeneralFailure)
		return "", "", fmt.Errorf("unsupported command: %d", header[1])
	}

	var host string
	switch header[3] {
	case atypIPv4:
		addr := make([]byte, 4)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", "", err
		}
		host = net.IP(addr).String()

	case atypDomain:
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return "", "", err
		}
		domain := make([]byte, lenBuf[0])
		if _, err := io.ReadFull(conn, domain); err != nil {
			return "", "", err
		}
		host = string(domain)

	case atypIPv6:
		addr := make([]byte, 16)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", "", err
		}
		host = net.IP(addr).String()

	default:
		s.sendReply(conn, repAddrNotSupported)
		return "", "", fmt.Errorf("unsupported address type: %d", header[3])
	}

	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return "", "", err
	}
	port := fmt.Sprintf("%d", binary.BigEndian.Uint16(portBuf))

	return host, port, nil
}

func (s *SOCKS5Handler) sendReply(conn net.Conn, rep byte) {
	reply := []byte{
		socks5Version,
		rep,
		0x00,       // reserved
		atypIPv4,   // address type
		0, 0, 0, 0, // bind addr
		0, 0, // bind port
	}
	conn.Write(reply)
}
