package converter

import (
	"bufio"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

const maxInputLineBytes = 1024 * 1024

var (
	uuidPattern        = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	hostLabelPattern   = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)
	safeTokenPattern   = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	allZeroUUIDPattern = regexp.MustCompile(`^0{8}-0{4}-0{4}-0{4}-0{12}$`)
)

func ParseText(content string, startPort int) (ParseResult, error) {
	var result ParseResult
	var validationErrors ValidationErrors

	if !utf8.ValidString(content) {
		return ParseResult{}, ValidationErrors{{Message: "输入 TXT 不是有效的 UTF-8 文本"}}
	}

	if startPort < 1 || startPort > 65535 {
		validationErrors = append(validationErrors, ValidationError{Message: "起始 SOCKS5 端口必须在 1 到 65535 之间"})
	}

	content = strings.TrimPrefix(content, "\uFEFF")
	scanner := bufio.NewScanner(strings.NewReader(content))
	scanner.Buffer(make([]byte, 64*1024), maxInputLineBytes)

	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}

		node, err := parseLink(line, lineNumber)
		if err != nil {
			validationErrors = append(validationErrors, ValidationError{Line: lineNumber, Message: err.Error()})
			continue
		}
		result.Nodes = append(result.Nodes, node)
	}
	if err := scanner.Err(); err != nil {
		validationErrors = append(validationErrors, ValidationError{Message: fmt.Sprintf("读取 TXT 失败：%v", err)})
	}

	if len(result.Nodes) == 0 && len(validationErrors) == 0 {
		validationErrors = append(validationErrors, ValidationError{Message: "未找到可转换的 VLESS 节点"})
	}

	seenNodes := make(map[string]int, len(result.Nodes))
	for _, node := range result.Nodes {
		key := duplicateKey(node)
		if firstLine, exists := seenNodes[key]; exists {
			validationErrors = append(validationErrors, ValidationError{
				Line:    node.SourceLine,
				Message: fmt.Sprintf("节点与第 %d 行重复", firstLine),
			})
			continue
		}
		seenNodes[key] = node.SourceLine
	}

	if len(result.Nodes) > 0 && startPort >= 1 && startPort <= 65535 {
		lastPort := startPort + len(result.Nodes) - 1
		if lastPort > 65535 {
			validationErrors = append(validationErrors, ValidationError{
				Message: fmt.Sprintf("端口范围超出 65535：起始端口 %d，共 %d 个节点", startPort, len(result.Nodes)),
			})
		} else {
			seenPorts := make(map[int]struct{}, len(result.Nodes))
			for index, node := range result.Nodes {
				port := startPort + index
				if _, exists := seenPorts[port]; exists {
					validationErrors = append(validationErrors, ValidationError{Message: fmt.Sprintf("生成了重复的本地端口 %d", port)})
					continue
				}
				seenPorts[port] = struct{}{}
				result.Mappings = append(result.Mappings, Mapping{
					Index:       index + 1,
					InboundTag:  indexedTag("ads", index+1),
					OutboundTag: indexedTag("reality", index+1),
					ListenPort:  port,
					NodeName:    node.Name,
				})
			}
		}
	}

	if len(validationErrors) > 0 {
		return ParseResult{}, validationErrors
	}
	return result, nil
}

func parseLink(raw string, lineNumber int) (Node, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return Node{}, fmt.Errorf("链接格式无效：%v", err)
	}
	if !strings.EqualFold(parsed.Scheme, "vless") {
		return Node{}, fmt.Errorf("协议不是 VLESS")
	}
	if parsed.User == nil || parsed.User.Username() == "" {
		return Node{}, fmt.Errorf("缺少 UUID")
	}
	if _, hasPassword := parsed.User.Password(); hasPassword {
		return Node{}, fmt.Errorf("VLESS 用户信息不能包含密码")
	}

	uuid := parsed.User.Username()
	if !validUUID(uuid) {
		return Node{}, fmt.Errorf("UUID 格式无效")
	}

	server := parsed.Hostname()
	if !validHost(server) {
		return Node{}, fmt.Errorf("服务器地址无效")
	}
	portText := parsed.Port()
	if portText == "" {
		return Node{}, fmt.Errorf("缺少服务端端口")
	}
	serverPort, err := strconv.Atoi(portText)
	if err != nil || serverPort < 1 || serverPort > 65535 {
		return Node{}, fmt.Errorf("服务端端口必须在 1 到 65535 之间")
	}

	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return Node{}, fmt.Errorf("查询参数编码无效：%v", err)
	}

	network, _, err := aliasValue(query, "type", "network")
	if err != nil {
		return Node{}, err
	}
	if network == "" {
		return Node{}, fmt.Errorf("缺少 type/network")
	}
	if !strings.EqualFold(network, "tcp") {
		return Node{}, fmt.Errorf("仅支持 TCP，收到 %q", safeDisplay(network))
	}

	encryption, _, err := aliasValue(query, "encryption")
	if err != nil {
		return Node{}, err
	}
	if encryption == "" {
		return Node{}, fmt.Errorf("缺少 encryption")
	}
	if !strings.EqualFold(encryption, "none") {
		return Node{}, fmt.Errorf("VLESS encryption 必须为 none")
	}

	security, _, err := aliasValue(query, "security")
	if err != nil {
		return Node{}, err
	}
	if !strings.EqualFold(security, "reality") {
		if security == "" {
			return Node{}, fmt.Errorf("缺少 security=reality")
		}
		return Node{}, fmt.Errorf("不是 Reality 节点")
	}

	publicKey, _, err := aliasValue(query, "pbk", "publicKey")
	if err != nil {
		return Node{}, err
	}
	if publicKey == "" {
		return Node{}, fmt.Errorf("缺少 Reality 公钥 pbk/publicKey")
	}
	if !validPublicKey(publicKey) {
		return Node{}, fmt.Errorf("Reality 公钥格式无效")
	}

	fingerprint, _, err := aliasValue(query, "fp", "fingerprint")
	if err != nil {
		return Node{}, err
	}
	if fingerprint == "" {
		return Node{}, fmt.Errorf("缺少 fingerprint")
	}
	if !safeTokenPattern.MatchString(fingerprint) {
		return Node{}, fmt.Errorf("fingerprint 格式无效")
	}

	serverName, _, err := aliasValue(query, "sni", "serverName")
	if err != nil {
		return Node{}, err
	}
	if serverName == "" {
		return Node{}, fmt.Errorf("缺少 SNI/serverName")
	}
	if !validHost(serverName) {
		return Node{}, fmt.Errorf("SNI/serverName 格式无效")
	}

	shortID, _, err := aliasValue(query, "sid", "shortId")
	if err != nil {
		return Node{}, err
	}
	if shortID == "" {
		return Node{}, fmt.Errorf("缺少 short ID")
	}
	if !validShortID(shortID) {
		return Node{}, fmt.Errorf("short ID 必须为不超过 16 个字符的偶数长度十六进制字符串")
	}

	spiderX, spiderXPresent, err := aliasValue(query, "spx")
	if err != nil {
		return Node{}, err
	}
	if !spiderXPresent {
		spiderX = "/"
	} else if spiderX == "" || !strings.HasPrefix(spiderX, "/") {
		return Node{}, fmt.Errorf("spx 解码后必须是以 / 开头的路径")
	}

	flow, _, err := aliasValue(query, "flow")
	if err != nil {
		return Node{}, err
	}
	if flow != "" && !safeTokenPattern.MatchString(flow) {
		return Node{}, fmt.Errorf("flow 格式无效")
	}

	return Node{
		SourceLine:  lineNumber,
		Name:        strings.TrimSpace(parsed.Fragment),
		Server:      server,
		ServerPort:  serverPort,
		UUID:        strings.ToLower(uuid),
		Network:     "tcp",
		Encryption:  "none",
		Security:    "reality",
		PublicKey:   publicKey,
		Fingerprint: fingerprint,
		ServerName:  serverName,
		ShortID:     strings.ToLower(shortID),
		SpiderX:     spiderX,
		Flow:        flow,
		RawLink:     raw,
	}, nil
}

func aliasValue(query url.Values, names ...string) (string, bool, error) {
	var selected string
	found := false
	for _, name := range names {
		values, exists := query[name]
		if !exists {
			continue
		}
		for _, value := range values {
			if !found {
				selected = value
				found = true
				continue
			}
			if value != selected {
				return "", true, fmt.Errorf("参数 %s 的重复值或别名值不一致", strings.Join(names, "/"))
			}
		}
	}
	return selected, found, nil
}

func validUUID(value string) bool {
	if !uuidPattern.MatchString(value) || allZeroUUIDPattern.MatchString(strings.ToLower(value)) {
		return false
	}
	compact := strings.ReplaceAll(value, "-", "")
	decoded, err := hex.DecodeString(compact)
	return err == nil && len(decoded) == 16
}

func validHost(value string) bool {
	if value == "" || strings.ContainsAny(value, " \t\r\n") {
		return false
	}
	if net.ParseIP(value) != nil {
		return true
	}
	if strings.IndexFunc(value, func(character rune) bool {
		return (character < '0' || character > '9') && character != '.'
	}) == -1 {
		return false
	}
	if strings.ContainsAny(value, "/:?#[]@") {
		return false
	}
	value = strings.TrimSuffix(value, ".")
	if value == "" || len(value) > 253 {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if !hostLabelPattern.MatchString(label) {
			return false
		}
	}
	return true
}

func validPublicKey(value string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		decoded, err = base64.URLEncoding.DecodeString(value)
	}
	return err == nil && len(decoded) == 32
}

func validShortID(value string) bool {
	if len(value) == 0 || len(value) > 16 || len(value)%2 != 0 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func duplicateKey(node Node) string {
	return strings.Join([]string{
		strings.ToLower(node.Server),
		strconv.Itoa(node.ServerPort),
		strings.ToLower(node.UUID),
		node.PublicKey,
		strings.ToLower(node.ServerName),
		strings.ToLower(node.ShortID),
	}, "\x00")
}

func safeDisplay(value string) string {
	const maxRunes = 32
	value = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, value)
	runes := []rune(value)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes]) + "..."
	}
	return value
}
