package tunnel

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"os"

	"github.com/hashicorp/yamux"
)

type Client struct {
	serverAddr string
	tlsConfig  *tls.Config

	conn    net.Conn
	session *yamux.Session

	wgLocalAddr *net.UDPAddr
	wgConn      *net.UDPConn
}

func NewClient(configPath, serverAddrFlag string) (*Client, error) {
	cfg, err := LoadClientConfig(configPath)
	if err != nil {
		return nil, err
	}

	// CLI flag can override server address if provided.
	if serverAddrFlag != "" {
		cfg.ServerAddr = serverAddrFlag
	}

	rootCAs, err := x509.SystemCertPool()
	if err != nil {
		rootCAs = x509.NewCertPool()
	}

	// Optional: load additional CA from file if present in config.
	if cfg.CACertFile != "" {
		if caCertPEM, err := os.ReadFile(cfg.CACertFile); err == nil {
			if ok := rootCAs.AppendCertsFromPEM(caCertPEM); !ok {
				log.Println("warning: could not append CA cert to root CAs")
			}
		} else {
			log.Printf("warning: could not read CA cert file %s: %v", cfg.CACertFile, err)
		}
	}

	tlsCfg := &tls.Config{
		RootCAs:    rootCAs,
		MinVersion: tls.VersionTLS12,
	}

	wgAddr, err := net.ResolveUDPAddr("udp", cfg.WireGuardLocal)
	if err != nil {
		return nil, fmt.Errorf("resolve WireGuard local UDP addr: %w", err)
	}

	return &Client{
		serverAddr:  cfg.ServerAddr,
		tlsConfig:   tlsCfg,
		wgLocalAddr: wgAddr,
	}, nil
}

func (c *Client) Start() error {
	conn, err := tls.Dial("tcp", c.serverAddr, c.tlsConfig)
	if err != nil {
		return fmt.Errorf("tls dial: %w", err)
	}
	c.conn = conn

	session, err := yamux.Client(conn, nil)
	if err != nil {
		return fmt.Errorf("yamux client: %w", err)
	}
	c.session = session

	// Open a dedicated yamux stream for WireGuard UDP forwarding.
	stream, err := c.session.Open()
	if err != nil {
		return fmt.Errorf("open yamux stream for WireGuard: %w", err)
	}

	// Bind a local UDP socket where WireGuard will talk to us.
	wgConn, err := net.ListenUDP("udp", c.wgLocalAddr)
	if err != nil {
		return fmt.Errorf("listen UDP for WireGuard on %s: %w", c.wgLocalAddr.String(), err)
	}
	c.wgConn = wgConn

	// Start bidirectional forwarding between local WireGuard UDP and remote endpoint over the stream.
	go c.udpToStream(wgConn, stream)
	go c.streamToUDP(wgConn, stream)

	return nil
}

func (c *Client) Close() error {
	if c.wgConn != nil {
		_ = c.wgConn.Close()
	}
	if c.session != nil {
		_ = c.session.Close()
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// udpToStream reads UDP datagrams from the local WireGuard socket and forwards them
// over the yamux stream using a simple length-prefixed framing: [uint16 length][payload].
func (c *Client) udpToStream(udpConn *net.UDPConn, stream net.Conn) {
	defer stream.Close()

	buf := make([]byte, 65535)
	for {
		n, _, err := udpConn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("udpToStream read error: %v", err)
			return
		}
		if n == 0 {
			continue
		}

		if n > 0xFFFF {
			log.Printf("udpToStream datagram too large (%d bytes), dropping", n)
			continue
		}

		header := make([]byte, 2)
		binary.BigEndian.PutUint16(header, uint16(n))

		if _, err := stream.Write(header); err != nil {
			log.Printf("udpToStream write length error: %v", err)
			return
		}
		if _, err := stream.Write(buf[:n]); err != nil {
			log.Printf("udpToStream write payload error: %v", err)
			return
		}
	}
}

// streamToUDP reads framed packets from the yamux stream and writes them as UDP
// datagrams back to the local WireGuard socket.
func (c *Client) streamToUDP(udpConn *net.UDPConn, stream net.Conn) {
	defer stream.Close()

	header := make([]byte, 2)
	buf := make([]byte, 65535)

	for {
		if _, err := io.ReadFull(stream, header); err != nil {
			log.Printf("streamToUDP read length error: %v", err)
			return
		}
		length := binary.BigEndian.Uint16(header)
		if length == 0 {
			continue
		}

		if int(length) > len(buf) {
			log.Printf("streamToUDP length too large (%d), dropping", length)
			// Drain and drop.
			if _, err := io.ReadFull(stream, buf[:len(buf)]); err != nil {
				log.Printf("streamToUDP drain error: %v", err)
				return
			}
			continue
		}

		if _, err := io.ReadFull(stream, buf[:length]); err != nil {
			log.Printf("streamToUDP read payload error: %v", err)
			return
		}

		if _, err := udpConn.Write(buf[:length]); err != nil {
			log.Printf("streamToUDP udp write error: %v", err)
			return
		}
	}
}


