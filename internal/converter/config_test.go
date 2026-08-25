package converter

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func TestBuildConfigStructure(t *testing.T) {
	result, err := ParseText(testLink(1, nil)+"\n"+testLink(2, nil), 21001)
	if err != nil {
		t.Fatal(err)
	}
	data, err := BuildConfig(result)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, forbidden := range []string{`"listen_port"`, `"server_port"`, `"public_key"`, `"freedom"`} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("config contains forbidden field/value %s", forbidden)
		}
	}

	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	inbounds := config["inbounds"].([]any)
	outbounds := config["outbounds"].([]any)
	if len(inbounds) != 2 || len(outbounds) != 3 {
		t.Fatalf("got %d inbounds and %d outbounds", len(inbounds), len(outbounds))
	}
	firstInbound := inbounds[0].(map[string]any)
	if firstInbound["listen"] != "127.0.0.1" || firstInbound["port"] != float64(21001) || firstInbound["tag"] != "ads-01" {
		t.Fatalf("unexpected first inbound: %#v", firstInbound)
	}
	settings := firstInbound["settings"].(map[string]any)
	if settings["auth"] != "noauth" || settings["udp"] != true {
		t.Fatalf("unexpected socks settings: %#v", settings)
	}
	block := outbounds[0].(map[string]any)
	if block["tag"] != "block" || block["protocol"] != "blackhole" {
		t.Fatalf("first outbound is not block: %#v", block)
	}
	reality := outbounds[1].(map[string]any)
	stream := reality["streamSettings"].(map[string]any)
	realitySettings := stream["realitySettings"].(map[string]any)
	if stream["network"] != "tcp" || stream["security"] != "reality" || realitySettings["spiderX"] != "/" {
		t.Fatalf("unexpected Reality settings: %#v", stream)
	}

	routing := config["routing"].(map[string]any)
	rules := routing["rules"].([]any)
	if len(rules) != 3 {
		t.Fatalf("routing rule count = %d", len(rules))
	}
	firstRule := rules[0].(map[string]any)
	if firstRule["outboundTag"] != "reality-01" || firstRule["inboundTag"].([]any)[0] != "ads-01" {
		t.Fatalf("unexpected first route: %#v", firstRule)
	}
	catchAll := rules[len(rules)-1].(map[string]any)
	if catchAll["network"] != "tcp,udp" || catchAll["outboundTag"] != "block" {
		t.Fatalf("unexpected catch-all route: %#v", catchAll)
	}
}

func TestBuildConfigOmitsEmptyFlow(t *testing.T) {
	result, err := ParseText(testLink(1, func(values url.Values) { values.Del("flow") }), DefaultStartPort)
	if err != nil {
		t.Fatal(err)
	}
	data, err := BuildConfig(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"flow"`) {
		t.Fatal("empty flow should be omitted")
	}
}

func TestBuildConfigRejectsEmptyOrMismatchedInput(t *testing.T) {
	if _, err := BuildConfig(ParseResult{}); err == nil {
		t.Fatal("empty result should fail")
	}
	if _, err := BuildConfig(ParseResult{Nodes: []Node{{}}}); err == nil {
		t.Fatal("mismatched mappings should fail")
	}
	result, err := ParseText(testLink(1, nil)+"\n"+testLink(2, nil), DefaultStartPort)
	if err != nil {
		t.Fatal(err)
	}
	result.Mappings[1].ListenPort = result.Mappings[0].ListenPort
	if _, err := BuildConfig(result); err == nil || !strings.Contains(err.Error(), "端口") {
		t.Fatalf("duplicate mapping port should fail: %v", err)
	}
}
