package convert

import (
	"strconv"

	"node-box/internal/subscription/clash/model"
	"node-box/internal/subscription/clash/model/clash"
	"node-box/internal/subscription/clash/model/singbox"
)

func tuic(p *clash.Proxies, s *singbox.SingBoxOut, _ model.SingBoxVer) ([]singbox.SingBoxOut, error) {
	tls(p, s, true)
	s.UUID = p.Uuid
	s.CongestionController = p.CongestionController
	s.UdpRelayMode = p.UdpRelayMode
	s.UdpOverStream = bool(p.UdpOverStream)
	s.ZeroRttHandshake = bool(p.ReduceRtt)
	if p.HeartbeatInterval != 0 {
		s.Heartbeat = strconv.Itoa(int(p.HeartbeatInterval)) + "ms"
	}
	if p.IP != "" {
		s.Server = p.IP
	}
	return []singbox.SingBoxOut{*s}, nil
}
