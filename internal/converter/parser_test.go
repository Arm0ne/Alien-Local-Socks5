package converter

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestParseNodeCounts(t *testing.T) {
	for _, count := range []int{1, 5, 8, 12} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			var links []string
			for index := 1; index <= count; index++ {
				links = append(links, testLink(index, nil))
			}
			result, err := ParseText(strings.Join(links, "\n"), DefaultStartPort)
			if err != nil {
				t.Fatalf("ParseText returned error: %v", err)
			}
			if len(result.Nodes) != count || len(result.Mappings) != count {
				t.Fatalf("got %d nodes and %d mappings, want %d", len(result.Nodes), len(result.Mappings), count)
			}
			last := result.Mappings[len(result.Mappings)-1]
			if last.ListenPort != DefaultStartPort+count-1 {
				t.Fatalf("last port = %d", last.ListenPort)
			}
		})
	}
}

func TestParseCommentsEncodingAliasesAndFingerprints(t *testing.T) {
	link := testLink(1, func(values url.Values) {
		values.Del("type")
		values.Set("network", "tcp")
		values.Del("pbk")
		values.Set("publicKey", testPublicKey(1))
		values.Del("fp")
		values.Set("fingerprint", "firefox")
		values.Del("sni")
		values.Set("serverName", "www.example.com")
		values.Del("sid")
		values.Set("shortId", "a1b2c3d4")
	})
	link = strings.Split(link, "#")[0] + "#Hong%20Kong%20Node"
	content := "\uFEFF\r\n# comment\r\n  // another comment\r\n\r\n" + link

	result, err := ParseText(content, 22000)
	if err != nil {
		t.Fatalf("ParseText returned error: %v", err)
	}
	node := result.Nodes[0]
	if node.SourceLine != 5 {
		t.Fatalf("source line = %d, want 5", node.SourceLine)
	}
	if node.Name != "Hong Kong Node" {
		t.Fatalf("name = %q", node.Name)
	}
	if node.SpiderX != "/" {
		t.Fatalf("spiderX = %q", node.SpiderX)
	}
	if node.Fingerprint != "firefox" {
		t.Fatalf("fingerprint = %q", node.Fingerprint)
	}
}

func TestParseDefaultsOmittedVLESSEncryptionToNone(t *testing.T) {
	link := testLink(1, func(values url.Values) {
		values.Del("encryption")
		values.Set("packetEncoding", "xudp")
	})
	result, err := ParseText(link, DefaultStartPort)
	if err != nil {
		t.Fatalf("ParseText returned error: %v", err)
	}
	if result.Nodes[0].Encryption != "none" {
		t.Fatalf("encryption = %q, want none", result.Nodes[0].Encryption)
	}
}

func TestParseIPv6Server(t *testing.T) {
	link := testLink(1, nil)
	parsed, err := url.Parse(link)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Host = "[2001:db8::1]:443"
	result, err := ParseText(parsed.String(), DefaultStartPort)
	if err != nil {
		t.Fatalf("ParseText returned error: %v", err)
	}
	if result.Nodes[0].Server != "2001:db8::1" {
		t.Fatalf("server = %q", result.Nodes[0].Server)
	}
}

func TestParseRejectsInvalidLinks(t *testing.T) {
	tests := []struct {
		name       string
		line       string
		wantPhrase string
	}{
		{"non-vless", "https://example.com", "协议不是 VLESS"},
		{"bad-uuid", replaceUUID(testLink(1, nil), "not-a-uuid"), "UUID 格式无效"},
		{"missing-uuid", replaceUUID(testLink(1, nil), ""), "缺少 UUID"},
		{"missing-server", replaceHost(testLink(1, nil), ":443"), "服务器地址无效"},
		{"missing-port", replaceHost(testLink(1, nil), "server.example.com"), "缺少服务端端口"},
		{"bad-port", replaceHost(testLink(1, nil), "server.example.com:70000"), "端口"},
		{"bad-numeric-host", replaceHost(testLink(1, nil), "999.999.999.999:443"), "服务器地址无效"},
		{"non-tcp", testLink(1, func(v url.Values) { v.Set("type", "ws") }), "仅支持 TCP"},
		{"non-reality", testLink(1, func(v url.Values) { v.Set("security", "tls") }), "不是 Reality"},
		{"bad-encryption", testLink(1, func(v url.Values) { v.Set("encryption", "aes-128-gcm") }), "必须为 none"},
		{"missing-key", testLink(1, func(v url.Values) { v.Del("pbk") }), "缺少 Reality 公钥"},
		{"bad-key", testLink(1, func(v url.Values) { v.Set("pbk", "secret") }), "公钥格式无效"},
		{"missing-fingerprint", testLink(1, func(v url.Values) { v.Del("fp") }), "缺少 fingerprint"},
		{"missing-sni", testLink(1, func(v url.Values) { v.Del("sni") }), "缺少 SNI"},
		{"missing-sid", testLink(1, func(v url.Values) { v.Del("sid") }), "缺少 short ID"},
		{"bad-sid", testLink(1, func(v url.Values) { v.Set("sid", "123") }), "short ID 必须"},
		{"bad-spx", testLink(1, func(v url.Values) { v.Set("spx", "relative") }), "spx 解码后"},
		{"alias-conflict", testLink(1, func(v url.Values) { v.Set("network", "ws") }), "别名值不一致"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseText("# first\n\n"+test.line, DefaultStartPort)
			if err == nil {
				t.Fatal("expected an error")
			}
			message := err.Error()
			if !strings.Contains(message, "第 3 行") || !strings.Contains(message, test.wantPhrase) {
				t.Fatalf("error = %q, want line and %q", message, test.wantPhrase)
			}
		})
	}
}

func TestParseRejectsDuplicateNode(t *testing.T) {
	link := testLink(1, nil)
	_, err := ParseText(link+"\n"+link, DefaultStartPort)
	if err == nil || !strings.Contains(err.Error(), "第 2 行：节点与第 1 行重复") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParsePortBounds(t *testing.T) {
	links := strings.Join([]string{testLink(1, nil), testLink(2, nil)}, "\n")
	if _, err := ParseText(links, 65534); err != nil {
		t.Fatalf("last port 65535 should pass: %v", err)
	}
	if _, err := ParseText(links, 65535); err == nil || !strings.Contains(err.Error(), "端口范围超出 65535") {
		t.Fatalf("unexpected overflow result: %v", err)
	}
	if _, err := ParseText(testLink(1, nil), 0); err == nil || !strings.Contains(err.Error(), "起始 SOCKS5 端口") {
		t.Fatalf("unexpected start port result: %v", err)
	}
}

func TestParseEmptyInput(t *testing.T) {
	_, err := ParseText("\n# comment\n// comment\n", DefaultStartPort)
	if err == nil || !strings.Contains(err.Error(), "未找到可转换") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseRejectsInvalidUTF8(t *testing.T) {
	_, err := ParseText(string([]byte{0xff, 0xfe}), DefaultStartPort)
	if err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func testLink(index int, modify func(url.Values)) string {
	values := url.Values{
		"type":       {"tcp"},
		"encryption": {"none"},
		"security":   {"reality"},
		"pbk":        {testPublicKey(index)},
		"fp":         {[]string{"chrome", "firefox", "safari", "edge"}[(index-1)%4]},
		"sni":        {fmt.Sprintf("sni-%d.example.com", index)},
		"sid":        {fmt.Sprintf("%016x", index)},
		"spx":        {"/"},
		"flow":       {"xtls-rprx-vision"},
	}
	if modify != nil {
		modify(values)
	}
	uri := &url.URL{
		Scheme:   "vless",
		User:     url.User(testUUID(index)),
		Host:     fmt.Sprintf("server-%d.example.com:%d", index, 40000+index),
		RawQuery: values.Encode(),
		Fragment: fmt.Sprintf("Node %d", index),
	}
	return uri.String()
}

func testUUID(index int) string {
	return fmt.Sprintf("%08x-1234-4abc-8def-%012x", index, index)
}

func testPublicKey(index int) string {
	key := make([]byte, 32)
	for position := range key {
		key[position] = byte(index + position)
	}
	return base64.RawURLEncoding.EncodeToString(key)
}

func replaceUUID(link, value string) string {
	parsed, _ := url.Parse(link)
	parsed.User = url.User(value)
	return parsed.String()
}

func replaceHost(link, value string) string {
	parsed, _ := url.Parse(link)
	parsed.Host = value
	return parsed.String()
}
