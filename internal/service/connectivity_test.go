package service

import (
	"encoding/json"
	"fmt"
	"github.com/fanat-sira/syranet/internal/config"
	"github.com/fanat-sira/syranet/internal/models"
	"github.com/fanat-sira/syranet/internal/node"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func availablePort(t *testing.T) int {
	t.Helper()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	p := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return p
}
func localCertificate(t *testing.T, dir string) (string, string) {
	t.Helper()
	cert, key := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	if exec.Command("openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "1", "-subj", "/CN=localhost", "-addext", "subjectAltName=DNS:localhost,IP:127.0.0.1", "-keyout", key, "-out", cert).Run() != nil {
		t.Fatal("cannot generate test certificate")
	}
	return cert, key
}
func runProcess(t *testing.T, binary, path string) *node.Process {
	t.Helper()
	p := &node.Process{Binary: binary}
	if output, e := exec.Command(binary, "check", "-c", path).CombinedOutput(); e != nil {
		t.Fatalf("real config rejected: %s", output)
	}
	if e := p.Start(path); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { p.Stop() })
	return p
}
func saveSpec(t *testing.T, dir, name string, s node.Spec, cert, key string) string {
	t.Helper()
	s.Inbounds[0].TLS.CertificatePath = cert
	s.Inbounds[0].TLS.KeyPath = key
	for i := range s.Outbounds {
		if s.Outbounds[i].TLS != nil && s.Outbounds[i].TLS.CertificatePath != "" {
			s.Outbounds[i].TLS.CertificatePath = cert
		}
	}
	b, e := node.Encode(s)
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(dir, name)
	if os.WriteFile(path, b, 0600) != nil {
		t.Fatal("cannot save fixture")
	}
	return path
}
func socksHTTP(t *testing.T, port int, target string) (string, error) {
	t.Helper()
	proxy, _ := url.Parse(fmt.Sprintf("socks5://127.0.0.1:%d", port))
	tr := &http.Transport{Proxy: http.ProxyURL(proxy), DisableKeepAlives: true}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 4 * time.Second}
	r, e := client.Get(target)
	if e != nil {
		return "", e
	}
	defer r.Body.Close()
	b, e := io.ReadAll(r.Body)
	return string(b), e
}
func TestRealHysteriaSingleMultiAndRevocation(t *testing.T) {
	binary := os.Getenv("SINGBOX_TEST_BINARY")
	if binary == "" {
		t.Skip("set SINGBOX_TEST_BINARY for real localhost Hysteria2 TCP/UDP tests")
	}
	dir := t.TempDir()
	cert, key := localCertificate(t, dir)
	echo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "syranet-through-hysteria") }))
	defer echo.Close()
	c, u := fixture()
	n := c.Nodes[0]
	n.PublicAddress = "127.0.0.1"
	n.ListenAddress = "127.0.0.1"
	n.Port = availablePort(t)
	n.TLS.ServerName = "localhost"
	c.Nodes[0] = n
	spec, e := Generate(c, n, []models.User{u}, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	serverPath := saveSpec(t, dir, "server.json", spec, cert, key)
	server := runProcess(t, binary, serverPath)
	socksPort := availablePort(t)
	clientSpec := map[string]any{"log": map[string]any{"level": "error"}, "inbounds": []any{map[string]any{"type": "socks", "tag": "client", "listen": "127.0.0.1", "listen_port": socksPort}}, "outbounds": []any{map[string]any{"type": "hysteria2", "tag": "proxy", "server": "127.0.0.1", "server_port": n.Port, "password": u.HY2Password, "tls": map[string]any{"enabled": true, "server_name": "localhost", "certificate_path": cert}}}, "route": map[string]any{"final": "proxy"}}
	b, _ := json.Marshal(clientSpec)
	clientPath := filepath.Join(dir, "client.json")
	os.WriteFile(clientPath, b, 0600)
	client := runProcess(t, binary, clientPath)
	if body, e := socksHTTP(t, socksPort, strings.Replace(echo.URL, "127.0.0.1", "localhost", 1)); e != nil || body != "syranet-through-hysteria" {
		t.Fatal("single-node verified TLS TCP tunnel failed", e)
	}
	// UDP traffic via SOCKS UDP association over the same Hysteria2 outbound.
	udp, e := net.ListenPacket("udp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer udp.Close()
	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, e := udp.ReadFrom(buf)
			if e != nil {
				return
			}
			udp.WriteTo(buf[:n], addr)
		}
	}()
	testUDP(t, socksPort, udp.LocalAddr().(*net.UDPAddr).Port)
	// Upstream routes through a local SOCKS listener bound to 127.0.0.2. The target
	// reports source IP, proving multi-node traffic exited from the upstream.
	sourceEcho := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		io.WriteString(w, host)
	}))
	defer sourceEcho.Close()
	upstream := n
	upstream.ID = "upstream"
	upstream.PublicAddress = "localhost"
	upstream.Entry = false
	upstream.Port = availablePort(t)
	upstream.TransitPassword = strings.Repeat("c", 64)
	upstream.TransitPasswordEnv = "TEST_TRANSIT"
	c.Nodes = append(c.Nodes, upstream)
	upspec, _ := Generate(c, upstream, nil, time.Now())
	upPath := saveSpec(t, dir, "upstream.json", upspec, cert, key)
	// sing-box direct outbound supports inet4_bind_address; use test-only routing
	// instrumentation to distinguish source from the entry on the same host.
	var raw map[string]any
	data, _ := os.ReadFile(upPath)
	json.Unmarshal(data, &raw)
	raw["outbounds"].([]any)[0].(map[string]any)["inet4_bind_address"] = "127.0.0.2"
	data, _ = json.Marshal(raw)
	os.WriteFile(upPath, data, 0600)
	runProcess(t, binary, upPath)
	c.Nodes[0].Outbound = "upstream"
	c.Nodes[1].TLS.CAPath = "/certs/ca.pem"
	spec, e = Generate(c, c.Nodes[0], []models.User{u}, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	server.Stop()
	client.Stop()
	serverPath = saveSpec(t, dir, "server.json", spec, cert, key)
	server = runProcess(t, binary, serverPath)
	client = runProcess(t, binary, clientPath)
	if source, e := socksHTTP(t, socksPort, sourceEcho.URL); e != nil || source != "127.0.0.2" {
		t.Fatalf("multi-node did not exit from upstream: %q %v", source, e)
	}
	u.Status = models.UserStatusSuspended
	spec, _ = Generate(c, c.Nodes[0], []models.User{u}, time.Now())
	server.Stop()
	client.Stop()
	serverPath = saveSpec(t, dir, "server.json", spec, cert, key)
	server = runProcess(t, binary, serverPath)
	client = runProcess(t, binary, clientPath)
	if _, e = socksHTTP(t, socksPort, echo.URL); e == nil {
		t.Fatal("revoked password still connects")
	}
	u.Status = models.UserStatusActive
	spec, _ = Generate(c, c.Nodes[0], []models.User{u}, time.Now())
	server.Stop()
	client.Stop()
	serverPath = saveSpec(t, dir, "server.json", spec, cert, key)
	runProcess(t, binary, serverPath)
	runProcess(t, binary, clientPath)
	if body, e := socksHTTP(t, socksPort, echo.URL); e != nil || body != "syranet-through-hysteria" {
		t.Fatal("reactivation failed", e)
	}
}
func testUDP(t *testing.T, socksPort, targetPort int) {
	t.Helper()
	tcp, e := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(socksPort)), time.Second)
	if e != nil {
		t.Fatal(e)
	}
	defer tcp.Close()
	tcp.SetDeadline(time.Now().Add(5 * time.Second))
	tcp.Write([]byte{5, 1, 0})
	reply := make([]byte, 2)
	if _, e = io.ReadFull(tcp, reply); e != nil || reply[1] != 0 {
		t.Fatal("SOCKS greeting failed")
	}
	tcp.Write([]byte{5, 3, 0, 1, 0, 0, 0, 0, 0, 0})
	header := make([]byte, 4)
	if _, e = io.ReadFull(tcp, header); e != nil || header[1] != 0 {
		t.Fatal("UDP associate failed")
	}
	var ip net.IP
	switch header[3] {
	case 1:
		ip = make([]byte, 4)
	case 4:
		ip = make([]byte, 16)
	default:
		t.Fatal("unexpected UDP relay address")
	}
	io.ReadFull(tcp, ip)
	p := make([]byte, 2)
	io.ReadFull(tcp, p)
	if ip.IsUnspecified() {
		ip = net.ParseIP("127.0.0.1")
	}
	relay := &net.UDPAddr{IP: ip, Port: int(p[0])<<8 | int(p[1])}
	conn, e := net.DialUDP("udp", nil, relay)
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	packet := append([]byte{0, 0, 0, 1, 127, 0, 0, 1, byte(targetPort >> 8), byte(targetPort)}, []byte("syranet-udp")...)
	conn.Write(packet)
	buf := make([]byte, 2048)
	n, e := conn.Read(buf)
	if e != nil || n < 10 || string(buf[n-len("syranet-udp"):n]) != "syranet-udp" {
		t.Fatal("real UDP tunnel failed", e)
	}
}

func TestRealSOCKSAuthentication(t *testing.T) {
	binary := os.Getenv("SINGBOX_TEST_BINARY")
	if binary == "" {
		t.Skip("set SINGBOX_TEST_BINARY")
	}
	dir := t.TempDir()
	cert, key := localCertificate(t, dir)
	c, u := fixture()
	n := c.Nodes[0]
	n.ListenAddress = "127.0.0.1"
	n.Port = availablePort(t)
	n.TLS.ServerName = "localhost"
	n.Socks = config.Socks{Enabled: true, ListenAddress: "127.0.0.1", Port: availablePort(t)}
	s, e := Generate(c, n, []models.User{u}, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	path := saveSpec(t, dir, "socks-server.json", s, cert, key)
	runProcess(t, binary, path)
	echo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "authenticated-socks") }))
	defer echo.Close()
	for _, correct := range []bool{true, false} {
		pass := u.SocksPassword
		if !correct {
			pass = "incorrect"
		}
		proxy := &url.URL{Scheme: "socks5", Host: net.JoinHostPort("127.0.0.1", strconv.Itoa(n.Socks.Port)), User: url.UserPassword(u.SocksUsername, pass)}
		tr := &http.Transport{Proxy: http.ProxyURL(proxy), DisableKeepAlives: true}
		client := &http.Client{Transport: tr, Timeout: 2 * time.Second}
		res, e := client.Get(echo.URL)
		if correct {
			if e != nil {
				t.Fatal("valid SOCKS credential rejected")
			}
			b, _ := io.ReadAll(res.Body)
			res.Body.Close()
			if string(b) != "authenticated-socks" {
				t.Fatal("SOCKS traffic failed")
			}
		} else if e == nil {
			res.Body.Close()
			t.Fatal("invalid SOCKS credential accepted")
		}
		tr.CloseIdleConnections()
	}
}
