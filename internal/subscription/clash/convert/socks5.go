package convert

import (
	"node-box/internal/subscription/clash/model/clash"
	"node-box/internal/subscription/clash/model/singbox"
)

func httpOpts(p *clash.Proxies, s *singbox.SingBoxOut) error {
	tls(p, s)
	s.Username = p.Username
	return nil
}

func socks5(p *clash.Proxies, s *singbox.SingBoxOut) error {
	tls(p, s)
	s.Username = p.Username
	return nil
}
