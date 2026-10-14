package service

import (
	"errors"
	"fmt"
	"github.com/fanat-sira/syranet/internal/config"
	"github.com/fanat-sira/syranet/internal/models"
	"github.com/fanat-sira/syranet/internal/node"
	"time"
)

func Generate(c *config.Config, n config.Node, users []models.User, now time.Time) (node.Spec, error) {
	if !n.Enabled {
		n.Entry = false
		n.TransitPassword = ""
		n.Socks.Enabled = false
		n.Outbound = "direct"
	}
	var spec node.Spec
	spec.Log.Level = "error"
	hy := []node.User{}
	socks := []node.SocksUser{}
	if n.TransitPassword != "" {
		hy = append(hy, node.User{Name: "transit", Password: n.TransitPassword})
	}
	for _, u := range users {
		selected := u.NodeID
		if selected == "" {
			selected = c.VPN.DefaultNode
		}
		if !n.Entry || selected != n.ID || !u.IsSubscriptionActive(now) {
			continue
		}
		if len(u.HY2Password) < 16 {
			return spec, errors.New("invalid stored credential")
		}
		hy = append(hy, node.User{Name: fmt.Sprintf("user_%d", u.ID), Password: u.HY2Password})
		socks = append(socks, node.SocksUser{Username: u.SocksUsername, Password: u.SocksPassword})
	}
	spec.Inbounds = []node.Inbound{{Type: "hysteria2", Tag: "hy2", Listen: n.ListenAddress, Port: n.Port, Users: hy, TLS: &node.TLS{Enabled: true, ServerName: n.TLS.ServerName, CertificatePath: n.TLS.CertificatePath, KeyPath: n.TLS.KeyPath}}}
	if n.Socks.Enabled && len(socks) > 0 {
		spec.Inbounds = append(spec.Inbounds, node.Inbound{Type: "socks", Tag: "socks", Listen: n.Socks.ListenAddress, Port: n.Socks.Port, Users: socks})
	}
	out := node.Outbound{Type: "direct", Tag: "egress"}
	if n.Outbound != "direct" {
		up, ok := c.Node(n.Outbound)
		if !ok {
			return spec, errors.New("upstream unavailable")
		}
		out = node.Outbound{Type: "hysteria2", Tag: "egress", Server: up.PublicAddress, Port: up.Port, Password: up.TransitPassword, TLS: &node.OutTLS{Enabled: true, ServerName: up.TLS.ServerName, CertificatePath: up.TLS.CAPath}}
	}
	spec.Outbounds = []node.Outbound{out}
	spec.Route.Final = "egress"
	return spec, spec.Validate()
}
