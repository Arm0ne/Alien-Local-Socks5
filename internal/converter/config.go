package converter

import (
	"encoding/json"
	"fmt"
)

type xrayConfig struct {
	Log       logConfig     `json:"log"`
	Inbounds  []inbound     `json:"inbounds"`
	Outbounds []outbound    `json:"outbounds"`
	Routing   routingConfig `json:"routing"`
}

type logConfig struct {
	LogLevel string `json:"loglevel"`
}

type inbound struct {
	Tag      string          `json:"tag"`
	Listen   string          `json:"listen"`
	Port     int             `json:"port"`
	Protocol string          `json:"protocol"`
	Settings inboundSettings `json:"settings"`
}

type inboundSettings struct {
	Auth string `json:"auth"`
	UDP  bool   `json:"udp"`
}

type outbound struct {
	Tag            string          `json:"tag"`
	Protocol       string          `json:"protocol"`
	Settings       any             `json:"settings"`
	StreamSettings *streamSettings `json:"streamSettings,omitempty"`
}

type vlessSettings struct {
	VNext []vnext `json:"vnext"`
}

type vnext struct {
	Address string      `json:"address"`
	Port    int         `json:"port"`
	Users   []vlessUser `json:"users"`
}

type vlessUser struct {
	ID         string `json:"id"`
	Encryption string `json:"encryption"`
	Flow       string `json:"flow,omitempty"`
}

type streamSettings struct {
	Network         string          `json:"network"`
	Security        string          `json:"security"`
	RealitySettings realitySettings `json:"realitySettings"`
}

type realitySettings struct {
	ServerName  string `json:"serverName"`
	Fingerprint string `json:"fingerprint"`
	PublicKey   string `json:"publicKey"`
	ShortID     string `json:"shortId"`
	SpiderX     string `json:"spiderX"`
}

type routingConfig struct {
	DomainStrategy string        `json:"domainStrategy"`
	Rules          []routingRule `json:"rules"`
}

type routingRule struct {
	Type        string   `json:"type"`
	InboundTag  []string `json:"inboundTag,omitempty"`
	Network     string   `json:"network,omitempty"`
	OutboundTag string   `json:"outboundTag"`
}

func BuildConfig(result ParseResult) ([]byte, error) {
	if len(result.Nodes) == 0 {
		return nil, fmt.Errorf("不能生成 0 节点配置")
	}
	if len(result.Nodes) != len(result.Mappings) {
		return nil, fmt.Errorf("节点数量与端口映射数量不一致")
	}
	seenPorts := make(map[int]struct{}, len(result.Mappings))
	seenInboundTags := make(map[string]struct{}, len(result.Mappings))
	seenOutboundTags := make(map[string]struct{}, len(result.Mappings))
	for _, mapping := range result.Mappings {
		if mapping.ListenPort < 1 || mapping.ListenPort > 65535 {
			return nil, fmt.Errorf("本地端口 %d 超出范围", mapping.ListenPort)
		}
		if _, exists := seenPorts[mapping.ListenPort]; exists {
			return nil, fmt.Errorf("本地端口 %d 重复", mapping.ListenPort)
		}
		if _, exists := seenInboundTags[mapping.InboundTag]; exists || mapping.InboundTag == "" {
			return nil, fmt.Errorf("入站 tag 为空或重复")
		}
		if _, exists := seenOutboundTags[mapping.OutboundTag]; exists || mapping.OutboundTag == "" {
			return nil, fmt.Errorf("出站 tag 为空或重复")
		}
		seenPorts[mapping.ListenPort] = struct{}{}
		seenInboundTags[mapping.InboundTag] = struct{}{}
		seenOutboundTags[mapping.OutboundTag] = struct{}{}
	}

	config := xrayConfig{
		Log: logConfig{LogLevel: "warning"},
		Outbounds: []outbound{{
			Tag:      "block",
			Protocol: "blackhole",
			Settings: map[string]any{},
		}},
		Routing: routingConfig{DomainStrategy: "AsIs"},
	}

	for index, node := range result.Nodes {
		mapping := result.Mappings[index]
		config.Inbounds = append(config.Inbounds, inbound{
			Tag:      mapping.InboundTag,
			Listen:   "127.0.0.1",
			Port:     mapping.ListenPort,
			Protocol: "socks",
			Settings: inboundSettings{Auth: "noauth", UDP: true},
		})
		config.Outbounds = append(config.Outbounds, outbound{
			Tag:      mapping.OutboundTag,
			Protocol: "vless",
			Settings: vlessSettings{VNext: []vnext{{
				Address: node.Server,
				Port:    node.ServerPort,
				Users: []vlessUser{{
					ID:         node.UUID,
					Encryption: "none",
					Flow:       node.Flow,
				}},
			}}},
			StreamSettings: &streamSettings{
				Network:  "tcp",
				Security: "reality",
				RealitySettings: realitySettings{
					ServerName:  node.ServerName,
					Fingerprint: node.Fingerprint,
					PublicKey:   node.PublicKey,
					ShortID:     node.ShortID,
					SpiderX:     node.SpiderX,
				},
			},
		})
		config.Routing.Rules = append(config.Routing.Rules, routingRule{
			Type:        "field",
			InboundTag:  []string{mapping.InboundTag},
			OutboundTag: mapping.OutboundTag,
		})
	}

	config.Routing.Rules = append(config.Routing.Rules, routingRule{
		Type:        "field",
		Network:     "tcp,udp",
		OutboundTag: "block",
	})

	return json.MarshalIndent(config, "", "  ")
}
