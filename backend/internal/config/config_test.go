package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDevelopmentCannotBindLANAndHTTPSRequiresCertificates(t *testing.T) {
	c := Default()
	c.DevHTTP = true
	c.Bind = "0.0.0.0"
	if c.Validate() == nil {
		t.Fatal("development bound to LAN")
	}
	c.Bind = "127.0.0.1"
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c.DevHTTP = false
	if c.Validate() == nil {
		t.Fatal("HTTPS started without certificates")
	}
}

func TestPortableConfigResolvesPathsAndRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"certificate":"certificate.pem","privateKey":"private-key.pem","ttlMinutes":5}`), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Certificate != filepath.Join(dir, "certificate.pem") || c.TTLMinutes != 5 || c.MaxItems != 100 {
		t.Fatal("portable config not resolved")
	}
	os.WriteFile(path, []byte(`{"unknown":1}`), 0600)
	if _, err := Load(path); err == nil {
		t.Fatal("unknown option silently ignored")
	}
}
