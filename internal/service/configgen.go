package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"vpn-service/internal/config"
	"vpn-service/internal/models"
)

type ConfigGenerator struct {
	templatePath string
	vpn          config.VPNConfig
}

type hysteriaUser struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password"`
}

type socksUser struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func NewConfigGenerator(vpn config.VPNConfig) *ConfigGenerator {
	return &ConfigGenerator{templatePath: vpn.SingBox.TemplatePath, vpn: vpn}
}

func (g *ConfigGenerator) Generate(users []models.User) ([]byte, error) {
	tpl, err := os.ReadFile(g.templatePath)
	if err != nil {
		return nil, fmt.Errorf("read template: %w", err)
	}

	hy2Users := make([]hysteriaUser, 0, len(users)+1)
	socksUsers := make([]socksUser, 0, len(users)+1)

	if g.vpn.Fallback.HY2Password != "" {
		hy2Users = append(hy2Users, hysteriaUser{Password: g.vpn.Fallback.HY2Password})
	}
	if g.vpn.Fallback.SocksUsername != "" && g.vpn.Fallback.SocksPassword != "" {
		socksUsers = append(socksUsers, socksUser{
			Username: g.vpn.Fallback.SocksUsername,
			Password: g.vpn.Fallback.SocksPassword,
		})
	}

	for _, user := range users {
		hy2Users = append(hy2Users, hysteriaUser{Username: user.HY2Username, Password: user.HY2Password})
		socksUsers = append(socksUsers, socksUser{Username: user.SocksUsername, Password: user.SocksPassword})
	}

	hy2JSON, err := json.Marshal(hy2Users)
	if err != nil {
		return nil, fmt.Errorf("marshal hysteria users: %w", err)
	}
	socksJSON, err := json.Marshal(socksUsers)
	if err != nil {
		return nil, fmt.Errorf("marshal socks users: %w", err)
	}

	rendered := bytes.Clone(tpl)
	rendered = bytes.ReplaceAll(rendered, []byte("\"__HY2_USERS__\""), hy2JSON)
	rendered = bytes.ReplaceAll(rendered, []byte("\"__SOCKS_USERS__\""), socksJSON)
	rendered = bytes.ReplaceAll(rendered, []byte("\"__HYSTERIA_PORT__\""), []byte(strconv.Itoa(g.vpn.HysteriaPort)))
	rendered = bytes.ReplaceAll(rendered, []byte("\"__SOCKS_PORT__\""), []byte(strconv.Itoa(g.vpn.SocksPort)))

	rendered, err = replaceQuotedPlaceholder(rendered, "__NL_SERVER_ADDRESS__", g.vpn.NLAddress)
	if err != nil {
		return nil, err
	}
	rendered, err = replaceQuotedPlaceholder(rendered, "__NL_PASSWORD__", g.vpn.NLPassword)
	if err != nil {
		return nil, err
	}

	var payload any
	if err := json.Unmarshal(rendered, &payload); err != nil {
		return nil, fmt.Errorf("validate generated json: %w", err)
	}

	pretty, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("format generated config: %w", err)
	}
	return pretty, nil
}

func replaceQuotedPlaceholder(source []byte, placeholder, value string) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal placeholder %s: %w", placeholder, err)
	}
	return bytes.ReplaceAll(source, []byte(fmt.Sprintf("\"%s\"", placeholder)), encoded), nil
}
