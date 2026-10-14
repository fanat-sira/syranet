package node

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func Handler(s *Supervisor, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if len(token) < 32 || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.Error(w, "unauthorized", 401)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/status":
			_ = json.NewEncoder(w).Encode(s.Status())
		case r.Method == http.MethodPut && r.URL.Path == "/config":
			r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
			dec := json.NewDecoder(r.Body)
			dec.DisallowUnknownFields()
			var spec Spec
			if dec.Decode(&spec) != nil {
				http.Error(w, "invalid JSON", 400)
				return
			}
			var extra any
			if dec.Decode(&extra) != io.EOF {
				http.Error(w, "invalid JSON", 400)
				return
			}
			if e := s.Apply(r.Context(), spec); e != nil {
				http.Error(w, e.Error(), 503)
				return
			}
			_ = json.NewEncoder(w).Encode(s.Status())
		default:
			http.Error(w, "not found", 404)
		}
	})
}

type Client struct {
	URL, Token string
	HTTP       *http.Client
}

func NewClient(address, token, ca string) (*Client, error) {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	if ca != "" {
		p, e := os.ReadFile(ca)
		if e != nil {
			return nil, errors.New("agent CA cannot be read")
		}
		roots, e := x509.SystemCertPool()
		if e != nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM(p) {
			return nil, errors.New("invalid agent CA")
		}
		tr.TLSClientConfig.RootCAs = roots
	}
	return &Client{URL: strings.TrimRight(address, "/"), Token: token, HTTP: &http.Client{Transport: tr, Timeout: 40 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *Client) request(ctx context.Context, method, path string, body io.Reader) (Status, error) {
	req, e := http.NewRequestWithContext(ctx, method, c.URL+path, body)
	if e != nil {
		return Status{}, errors.New("invalid agent request")
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	res, e := c.HTTP.Do(req)
	if e != nil {
		return Status{}, errors.New("node unreachable or HTTPS verification failed")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return Status{}, errors.New("node rejected operation; check token, TLS files and configuration")
	}
	var status Status
	if json.NewDecoder(io.LimitReader(res.Body, 65536)).Decode(&status) != nil {
		return Status{}, errors.New("invalid node status")
	}
	return status, nil
}
func (c *Client) Status(ctx context.Context) (Status, error) {
	return c.request(ctx, "GET", "/status", nil)
}
func (c *Client) Apply(ctx context.Context, spec Spec) (Status, error) {
	b, e := json.Marshal(spec)
	if e != nil {
		return Status{}, errors.New("invalid spec")
	}
	return c.request(ctx, "PUT", "/config", strings.NewReader(string(b)))
}
