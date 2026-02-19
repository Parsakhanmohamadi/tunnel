package tunnel

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ServerConfig defines configuration for the tunnel server (abroad).
type ServerConfig struct {
	// ListenAddr is the TCP address the TLS server listens on, e.g. ":8443" or "0.0.0.0:443".
	ListenAddr string `yaml:"listen_addr"`

	// TLSCertFile and TLSKeyFile are the certificate and key used by the TLS listener.
	TLSCertFile string `yaml:"tls_cert_file"`
	TLSKeyFile  string `yaml:"tls_key_file"`

	// WireGuardRemote is the UDP address of the WireGuard interface on the abroad server,
	// typically "127.0.0.1:51820".
	WireGuardRemote string `yaml:"wireguard_remote"`
}

// ClientConfig defines configuration for the tunnel client (Iran server).
type ClientConfig struct {
	// ServerAddr is the address of the tunnel server, e.g. "myserver.com:8443".
	ServerAddr string `yaml:"server_addr"`

	// CACertFile is an optional path to a CA certificate to trust for the server.
	// If empty, system CAs are used.
	CACertFile string `yaml:"ca_cert_file"`

	// WireGuardLocal is the local UDP address where WireGuard listens on the Iran server,
	// typically "127.0.0.1:51820".
	WireGuardLocal string `yaml:"wireguard_local"`
}

func LoadServerConfig(path string) (*ServerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read server config: %w", err)
	}
	var cfg ServerConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal server config: %w", err)
	}

	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":8443"
	}
	if cfg.WireGuardRemote == "" {
		cfg.WireGuardRemote = "127.0.0.1:51820"
	}

	return &cfg, nil
}

func LoadClientConfig(path string) (*ClientConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read client config: %w", err)
	}
	var cfg ClientConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal client config: %w", err)
	}

	if cfg.ServerAddr == "" {
		return nil, fmt.Errorf("client config: server_addr is required")
	}
	if cfg.WireGuardLocal == "" {
		cfg.WireGuardLocal = "127.0.0.1:51820"
	}

	return &cfg, nil
}

