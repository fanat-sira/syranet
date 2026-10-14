package service

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/url"
	"strings"
)

const alphaNum = "abcdefghijklmnopqrstuvwxyz0123456789"

type KeyGenerator struct {
	ruAddress string
	hy2Port   int
	socksPort int
	hy2Name   string
}

func NewKeyGenerator(ruAddress string, hy2Port, socksPort int, hy2Name string) *KeyGenerator {
	return &KeyGenerator{
		ruAddress: strings.TrimSpace(ruAddress),
		hy2Port:   hy2Port,
		socksPort: socksPort,
		hy2Name:   hy2Name,
	}
}

func (g *KeyGenerator) NewHysteriaCredentials() (string, string, error) {
	username, err := randomAlphaNum("u_", 8)
	if err != nil {
		return "", "", err
	}
	password, err := randomHex(8)
	if err != nil {
		return "", "", err
	}
	return username, password, nil
}

func (g *KeyGenerator) NewSocksCredentials() (string, string, error) {
	username, err := randomAlphaNum("s_", 8)
	if err != nil {
		return "", "", err
	}
	password, err := randomHex(8)
	if err != nil {
		return "", "", err
	}
	return username, password, nil
}

func (g *KeyGenerator) BuildHysteriaLink(username, password string) string {
	return fmt.Sprintf(
		"hy2://%s:%s@%s:%d?insecure=1#%s",
		url.QueryEscape(username),
		url.QueryEscape(password),
		g.ruAddress,
		g.hy2Port,
		url.QueryEscape(g.hy2Name),
	)
}

func (g *KeyGenerator) BuildSocksLink(username, password string) string {
	values := url.Values{}
	values.Set("server", g.ruAddress)
	values.Set("port", fmt.Sprintf("%d", g.socksPort))
	values.Set("user", username)
	values.Set("pass", password)
	return "https://t.me/socks?" + values.Encode()
}

func randomAlphaNum(prefix string, count int) (string, error) {
	builder := strings.Builder{}
	builder.Grow(len(prefix) + count)
	builder.WriteString(prefix)

	max := big.NewInt(int64(len(alphaNum)))
	for i := 0; i < count; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("generate random username: %w", err)
		}
		builder.WriteByte(alphaNum[n.Int64()])
	}

	return builder.String(), nil
}

func randomHex(bytesLen int) (string, error) {
	buffer := make([]byte, bytesLen)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate random password: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}
