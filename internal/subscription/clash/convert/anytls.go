package convert

import (
	"strconv"

	"node-box/internal/subscription/clash/model"
	"node-box/internal/subscription/clash/model/clash"
	"node-box/internal/subscription/clash/model/singbox"
)

func anytls(p *clash.Proxies, s *singbox.SingBoxOut, _ model.SingBoxVer) ([]singbox.SingBoxOut, error) {
	tls(p, s, true)
	s.TcpFastOpen = false // anytls 不支持 TCP Fast Open

	if p.IdleSessionCheckInterval != 0 {
		s.IdleSessionCheckInterval = strconv.Itoa(int(p.IdleSessionCheckInterval)) + "s"
	}
	if p.IdleSessionTimeout != 0 {
		s.IdleSessionTimeout = strconv.Itoa(int(p.IdleSessionTimeout)) + "s"
	}
	if p.MinIdleSession != 0 {
		s.MinIdleSession = int(p.MinIdleSession)
	}
	return []singbox.SingBoxOut{*s}, nil
}
