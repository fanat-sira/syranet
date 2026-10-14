// Package node implements a restricted authenticated sing-box supervisor.
package node

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"os"
	"time"
)

type User struct {
	Name     string `json:"name"`
	Password string `json:"password"`
}
type SocksUser struct {
	Username string `json:"username"`
	Password string `json:"password"`
}
type TLS struct {
	Enabled         bool   `json:"enabled"`
	ServerName      string `json:"server_name,omitempty"`
	CertificatePath string `json:"certificate_path,omitempty"`
	KeyPath         string `json:"key_path,omitempty"`
}
type OutTLS struct {
	Enabled         bool   `json:"enabled"`
	ServerName      string `json:"server_name"`
	CertificatePath string `json:"certificate_path,omitempty"`
}
type Inbound struct {
	Type   string `json:"type"`
	Tag    string `json:"tag"`
	Listen string `json:"listen"`
	Port   int    `json:"listen_port"`
	Users  any    `json:"users"`
	TLS    *TLS   `json:"tls,omitempty"`
}
type Outbound struct {
	Type     string  `json:"type"`
	Tag      string  `json:"tag"`
	Server   string  `json:"server,omitempty"`
	Port     int     `json:"server_port,omitempty"`
	Password string  `json:"password,omitempty"`
	TLS      *OutTLS `json:"tls,omitempty"`
}
type Spec struct {
	Log struct {
		Level string `json:"level"`
	} `json:"log"`
	Inbounds  []Inbound  `json:"inbounds"`
	Outbounds []Outbound `json:"outbounds"`
	Route     struct {
		Final string `json:"final"`
	} `json:"route"`
}

func Hash(p []byte) string { s := sha256.Sum256(p); return hex.EncodeToString(s[:]) }
func (s Spec) Validate() error {
	if s.Log.Level != "error" || len(s.Inbounds) < 1 || len(s.Inbounds) > 2 || len(s.Outbounds) != 1 || s.Route.Final != "egress" {
		return errors.New("unsupported node configuration")
	}
	for _, in := range s.Inbounds {
		if net.ParseIP(in.Listen) == nil || in.Port < 1 || in.Port > 65535 {
			return errors.New("invalid listener")
		}
		if in.Type == "hysteria2" {
			if in.TLS == nil || !in.TLS.Enabled || in.TLS.ServerName == "" || in.TLS.CertificatePath != "/certs/fullchain.pem" || in.TLS.KeyPath != "/certs/privkey.pem" {
				return errors.New("invalid inbound TLS")
			}
			data, _ := json.Marshal(in.Users)
			var users []User
			if json.Unmarshal(data, &users) != nil {
				return errors.New("invalid Hysteria2 users")
			}
			seen := map[string]bool{}
			for _, u := range users {
				if u.Name == "" || len(u.Password) < 16 || seen[u.Password] {
					return errors.New("invalid or duplicate Hysteria2 credential")
				}
				seen[u.Password] = true
			}
		} else if in.Type == "socks" {
			data, _ := json.Marshal(in.Users)
			var users []SocksUser
			if json.Unmarshal(data, &users) != nil || in.TLS != nil {
				return errors.New("invalid SOCKS users")
			}
			for _, u := range users {
				if u.Username == "" || len(u.Password) < 16 {
					return errors.New("invalid SOCKS credential")
				}
			}
		} else {
			return errors.New("unsupported inbound")
		}
	}
	if s.Inbounds[0].Type != "hysteria2" || (len(s.Inbounds) == 2 && s.Inbounds[1].Type != "socks") {
		return errors.New("invalid inbound order")
	}
	o := s.Outbounds[0]
	if o.Tag != "egress" {
		return errors.New("invalid route")
	}
	if o.Type == "hysteria2" {
		if o.Server == "" || o.Port < 1 || o.Port > 65535 || len(o.Password) < 32 || o.TLS == nil || !o.TLS.Enabled || o.TLS.ServerName == "" || (o.TLS.CertificatePath != "" && o.TLS.CertificatePath != "/certs/ca.pem") {
			return errors.New("invalid upstream")
		}
	} else if o.Type != "direct" {
		return errors.New("unsupported outbound")
	}
	return nil
}
func CertificateFingerprint(certPath, keyPath, name string) (string, error) {
	pair, e := tls.LoadX509KeyPair(certPath, keyPath)
	if e != nil {
		return "", errors.New("TLS certificate/key cannot be loaded")
	}
	cert := pair.Leaf
	// LoadX509KeyPair sets Leaf on the supported Go toolchain.
	if cert == nil {
		return "", errors.New("TLS certificate has no leaf")
	}
	if time.Now().Before(cert.NotBefore) || !time.Now().Before(cert.NotAfter) || cert.VerifyHostname(name) != nil {
		return "", errors.New("TLS certificate expired, not yet valid or SNI mismatch")
	}
	b, e := os.ReadFile(certPath)
	if e != nil {
		return "", errors.New("TLS certificate cannot be read")
	}
	return Hash(b), nil
}

// Encode uses canonical JSON ordering even when Users were decoded through HTTP.
func Encode(s Spec) ([]byte, error) {
	b, e := json.Marshal(s)
	if e != nil {
		return nil, e
	}
	var v any
	if e = json.Unmarshal(b, &v); e != nil {
		return nil, e
	}
	return json.MarshalIndent(v, "", "  ")
}
