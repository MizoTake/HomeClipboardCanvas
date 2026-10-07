package config

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"time"

	"homeclipboardcanvas/internal/board"
)

type Config struct {
	Bind            string   `json:"bind"`
	Port            int      `json:"port"`
	TTLMinutes      int      `json:"ttlMinutes"`
	MaxItems        int      `json:"maxItems"`
	MaxMemoryMB     int      `json:"maxMemoryMB"`
	MaxTextMB       int      `json:"maxTextMB"`
	MaxImageMB      int      `json:"maxImageMB"`
	AllowedNetworks []string `json:"allowedNetworks"`
	Certificate     string   `json:"certificate"`
	PrivateKey      string   `json:"privateKey"`
	Token           string   `json:"token"`
	LogLevel        string   `json:"logLevel"`
	DevHTTP         bool     `json:"-"`
}

func Default() Config {
	return Config{Bind: "0.0.0.0", Port: 8443, TTLMinutes: 60, MaxItems: 100, MaxMemoryMB: 200, MaxTextMB: 1, MaxImageMB: 10, LogLevel: "INFO"}
}

func Load(path string) (Config, error) {
	c := Default()
	f, err := os.Open(path)
	if err != nil {
		return c, err
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 64<<10))
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil {
		return c, err
	}
	if d.Decode(new(any)) != io.EOF {
		return c, errors.New("config must contain one JSON object")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return c, err
	}
	for _, p := range []*string{&c.Certificate, &c.PrivateKey} {
		if *p != "" && !filepath.IsAbs(*p) {
			*p = filepath.Join(filepath.Dir(abs), *p)
		}
	}
	return c, nil
}

func (c Config) Networks() ([]netip.Prefix, error) {
	var prefixes []netip.Prefix
	for _, cidr := range c.AllowedNetworks {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid allowed network: %s", cidr)
		}
		prefixes = append(prefixes, prefix.Masked())
	}
	return prefixes, nil
}
func (c Config) Limits() board.Limits {
	return board.Limits{MaxItems: c.MaxItems, MaxMemory: int64(c.MaxMemoryMB) << 20, MaxText: int64(c.MaxTextMB) << 20, MaxImage: int64(c.MaxImageMB) << 20, TTL: time.Duration(c.TTLMinutes) * time.Minute}
}

func (c Config) Validate() error {
	ip, err := netip.ParseAddr(c.Bind)
	if err != nil {
		return errors.New("bind must be an IP address")
	}
	if c.DevHTTP && !ip.IsLoopback() {
		return errors.New("--dev-http requires a loopback bind address")
	}
	if c.Port < 1 || c.Port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	if c.TTLMinutes < 1 || c.TTLMinutes > 1440 || c.MaxItems < 1 || c.MaxItems > 1000 || c.MaxMemoryMB < 1 || c.MaxMemoryMB > 4096 || c.MaxTextMB < 1 || c.MaxTextMB > 10 || c.MaxImageMB < 1 || c.MaxImageMB > 64 {
		return errors.New("memory, item and TTL limits are outside supported bounds")
	}
	if _, err := c.Networks(); err != nil {
		return err
	}
	secret, err := base64.RawURLEncoding.DecodeString(c.Token)
	if err != nil || len(secret) != 32 {
		return errors.New("a 32-byte base64url token is required; set HCC_TOKEN or config.token")
	}
	if !c.DevHTTP {
		if c.Certificate == "" || c.PrivateKey == "" {
			return errors.New("HTTPS certificates required; localhost development may use --dev-http")
		}
		if _, err := tls.LoadX509KeyPair(c.Certificate, c.PrivateKey); err != nil {
			return errors.New("unable to load TLS certificate and private key")
		}
	}
	switch c.LogLevel {
	case "DEBUG", "INFO", "WARN", "ERROR":
	default:
		return errors.New("logLevel must be DEBUG, INFO, WARN or ERROR")
	}
	return nil
}
