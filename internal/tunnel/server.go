package tunnel

import (
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"

	"github.com/hashicorp/yamux"
)

type Server struct {
	addr           string
	wireGuardRemote string
	tlsConfig      *tls.Config
	listener       net.Listener
}

func NewServer(configPath, listenAddr string) (*Server, error) {
	cfg, err := LoadServerConfig(configPath)
	if err != nil {
		return nil, err
	}

	// CLI flag can override listen address if provided.
	if listenAddr != "" {
		cfg.ListenAddr = listenAddr
	}

	cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
	if err != nil {
		return nil, fmt.Errorf("load server cert/key: %w", err)
	}

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	return &Server{
		addr:            cfg.ListenAddr,
		wireGuardRemote: cfg.WireGuardRemote,
		tlsConfig:       tlsCfg,
	}, nil
}

func (s *Server) Start() error {
	ln, err := tls.Listen("tcp", s.addr, s.tlsConfig)
	if err != nil {
		return fmt.Errorf("tls listen: %w", err)
	}
	s.listener = ln

	go s.acceptLoop()
	return nil
}

func (s *Server) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			log.Printf("accept error: %v", err)
			return
		}

		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()

	session, err := yamux.Server(conn, nil)
	if err != nil {
		log.Printf("yamux server session error: %v", err)
		return
	}
	defer session.Close()

	// For the first version we treat every new stream as a WireGuard UDP tunnel stream.
	for {
		stream, err := session.Accept()
		if err != nil {
			log.Printf("yamux accept stream error: %v", err)
			return
		}

		go s.handleWireGuardStream(stream)
	}
}

func (s *Server) Close() error {
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

// handleWireGuardStream bridges a single yamux stream to the WireGuard UDP socket
// on the abroad server, using the same length-prefixed framing as the client side.
func (s *Server) handleWireGuardStream(stream net.Conn) {
	defer stream.Close()

	remoteAddr, err := net.ResolveUDPAddr("udp", s.wireGuardRemote)
	if err != nil {
		log.Printf("resolve WireGuard remote UDP addr error: %v", err)
		return
	}

	wgConn, err := net.DialUDP("udp", nil, remoteAddr)
	if err != nil {
		log.Printf("dial WireGuard UDP %s error: %v", remoteAddr.String(), err)
		return
	}
	defer wgConn.Close()

	// Bidirectional forwarding: stream -> UDP and UDP -> stream.
	go streamToUDPServer(wgConn, stream)
	udpToStreamServer(wgConn, stream)
}

// udpToStreamServer reads from the WireGuard UDP socket and sends framed packets
// over the yamux stream.
func udpToStreamServer(udpConn *net.UDPConn, stream net.Conn) {
	buf := make([]byte, 65535)
	header := make([]byte, 2)

	for {
		n, _, err := udpConn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("udpToStreamServer read error: %v", err)
			return
		}
		if n == 0 {
			continue
		}
		if n > 0xFFFF {
			log.Printf("udpToStreamServer datagram too large (%d bytes), dropping", n)
			continue
		}

		binary.BigEndian.PutUint16(header, uint16(n))
		if _, err := stream.Write(header); err != nil {
			log.Printf("udpToStreamServer write length error: %v", err)
			return
		}
		if _, err := stream.Write(buf[:n]); err != nil {
			log.Printf("udpToStreamServer write payload error: %v", err)
			return
		}
	}
}

// streamToUDPServer reads framed packets from the yamux stream and writes them to
// the WireGuard UDP socket.
func streamToUDPServer(udpConn *net.UDPConn, stream net.Conn) {
	header := make([]byte, 2)
	buf := make([]byte, 65535)

	for {
		if _, err := io.ReadFull(stream, header); err != nil {
			log.Printf("streamToUDPServer read length error: %v", err)
			return
		}
		length := binary.BigEndian.Uint16(header)
		if length == 0 {
			continue
		}

		if int(length) > len(buf) {
			log.Printf("streamToUDPServer length too large (%d), dropping", length)
			// Drain and drop.
			if _, err := io.ReadFull(stream, buf[:len(buf)]); err != nil {
				log.Printf("streamToUDPServer drain error: %v", err)
				return
			}
			continue
		}

		if _, err := io.ReadFull(stream, buf[:length]); err != nil {
			log.Printf("streamToUDPServer read payload error: %v", err)
			return
		}

		if _, err := udpConn.Write(buf[:length]); err != nil {
			log.Printf("streamToUDPServer udp write error: %v", err)
			return
		}
	}
}


