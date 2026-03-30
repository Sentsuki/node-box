package convert

import (
	"fmt"

	"node-box/internal/subscription/clash/model"
	"node-box/internal/subscription/clash/model/clash"
	"node-box/internal/subscription/clash/model/singbox"
)

func anytls(p *clash.Proxies, s *singbox.SingBoxOut, v model.SingBoxVer) ([]singbox.SingBoxOut, error) {
	p.Tls = true
	tls(p, s)
	s.TcpFastOpen = false

	if p.IdleSessionCheckInterval != 0 {
		s.IdleSessionCheckInterval = fmt.Sprintf("%vs", p.IdleSessionCheckInterval)
	}
	if p.IdleSessionTimeout != 0 {
		s.IdleSessionTimeout = fmt.Sprintf("%vs", p.IdleSessionTimeout)
	}
	if p.MinIdleSession != 0 {
		s.MinIdleSession = int(p.MinIdleSession)
	}
	return []singbox.SingBoxOut{*s}, nil
}
