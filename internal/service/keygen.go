package service

import (
	"github.com/fanat-sira/syranet/internal/config"
	"github.com/fanat-sira/syranet/internal/service/credentials"
	"net"
	"net/url"
	"strconv"
)

// 256-bit random secrets are generated independently for every protocol.
func Credential() (string, error) { return credentials.Generate() }

type KeyGenerator struct{}

func NewKeyGenerator() *KeyGenerator { return &KeyGenerator{} }
func (g *KeyGenerator) NewHysteriaCredentials() (string, string, error) {
	return credentialPair("u_")
}
func (g *KeyGenerator) NewSocksCredentials() (string, string, error) {
	return credentialPair("s_")
}

func credentialPair(prefix string) (string, string, error) {
	password, err := Credential()
	if err != nil {
		return "", "", err
	}
	label, err := Credential()
	if err != nil {
		return "", "", err
	}
	return prefix + label[:24], password, nil
}
func BuildHysteriaLink(n config.Node, password string) string {
	u := url.URL{Scheme: "hysteria2", User: url.User(password), Host: net.JoinHostPort(n.PublicAddress, strconv.Itoa(n.Port)), Path: "/", RawQuery: url.Values{"sni": {n.TLS.ServerName}}.Encode(), Fragment: n.Name}
	return u.String()
}
func BuildSocksLink(n config.Node, username, password string) string {
	return "https://t.me/socks?" + url.Values{"server": {n.PublicAddress}, "port": {strconv.Itoa(n.Socks.Port)}, "user": {username}, "pass": {password}}.Encode()
}
