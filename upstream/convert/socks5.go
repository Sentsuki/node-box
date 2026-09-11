package convert

import (
	"node-box/upstream/model/clash"
	"node-box/upstream/model/singbox"
)

func httpOpts(p *clash.Proxies, s *singbox.SingBoxOut) error {
	tls(p, s, bool(p.Tls))
	s.Username = p.Username
	return nil
}

func socks5(p *clash.Proxies, s *singbox.SingBoxOut) error {
	tls(p, s, bool(p.Tls))
	s.Username = p.Username
	return nil
}
