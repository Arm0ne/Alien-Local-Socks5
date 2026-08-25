package subscription

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxResponseBytes = 10 * 1024 * 1024
	maxRedirects     = 3
)

type Document struct {
	SourceTXT      string
	URL            string
	ExpiresAt      *time.Time
	FetchedAt      time.Time
	MetadataNotice string
}

type Fetcher struct {
	Client *http.Client
}

func (fetcher Fetcher) Fetch(ctx context.Context, rawURL string) (Document, error) {
	rawURL = strings.TrimSpace(rawURL)
	parsedURL, err := url.Parse(rawURL)
	if err != nil || parsedURL.Host == "" {
		return Document{}, fmt.Errorf("订阅地址无效")
	}
	if !strings.EqualFold(parsedURL.Scheme, "https") && !strings.EqualFold(parsedURL.Scheme, "http") {
		return Document{}, fmt.Errorf("订阅地址必须使用 HTTP 或 HTTPS")
	}

	client := fetcher.Client
	if client == nil {
		client = &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				Proxy:                 nil,
				TLSHandshakeTimeout:   8 * time.Second,
				ResponseHeaderTimeout: 12 * time.Second,
				DisableKeepAlives:     true,
			},
			CheckRedirect: func(_ *http.Request, via []*http.Request) error {
				if len(via) >= maxRedirects {
					return fmt.Errorf("订阅重定向次数过多")
				}
				return nil
			},
		}
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return Document{}, fmt.Errorf("创建订阅请求失败：%w", err)
	}
	request.Header.Set("Accept", "text/plain, text/*;q=0.9, */*;q=0.1")
	request.Header.Set("User-Agent", "AlienLocalSocks5/1.3")
	response, err := client.Do(request)
	if err != nil {
		return Document{}, fmt.Errorf("拉取订阅失败：%s", summarize(err.Error()))
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return Document{}, fmt.Errorf("拉取订阅失败：服务器返回 HTTP %d", response.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return Document{}, fmt.Errorf("读取订阅内容失败：%w", err)
	}
	if len(body) > maxResponseBytes {
		return Document{}, fmt.Errorf("订阅内容超过 10 MiB 限制")
	}
	sourceTXT, err := decodeSource(body)
	if err != nil {
		return Document{}, err
	}
	expiresAt, notice := parseExpiry(response.Header.Get("subscription-userinfo"))
	return Document{
		SourceTXT:      sourceTXT,
		URL:            rawURL,
		ExpiresAt:      expiresAt,
		FetchedAt:      time.Now(),
		MetadataNotice: notice,
	}, nil
}

func decodeSource(body []byte) (string, error) {
	if !utf8.Valid(body) {
		return "", fmt.Errorf("订阅内容不是有效的 UTF-8 或 Base64 文本")
	}
	text := strings.TrimSpace(strings.TrimPrefix(string(body), "\uFEFF"))
	if containsVLESS(text) {
		return text, nil
	}

	compact := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\r' || r == '\n' {
			return -1
		}
		return r
	}, text)
	if compact == "" {
		return "", fmt.Errorf("订阅内容为空")
	}
	encodings := []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	}
	for _, encoding := range encodings {
		decoded, err := encoding.DecodeString(compact)
		if err != nil || !utf8.Valid(decoded) {
			continue
		}
		decodedText := strings.TrimSpace(strings.TrimPrefix(string(decoded), "\uFEFF"))
		if containsVLESS(decodedText) {
			return decodedText, nil
		}
	}
	return "", fmt.Errorf("订阅内容不是纯文本或 Base64 编码的 VLESS 列表")
}

func containsVLESS(text string) bool {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "vless://") {
			return true
		}
	}
	return false
}

func parseExpiry(value string) (*time.Time, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, ""
	}
	for _, part := range strings.Split(value, ";") {
		key, rawValue, found := strings.Cut(strings.TrimSpace(part), "=")
		if !found || !strings.EqualFold(strings.TrimSpace(key), "expire") {
			continue
		}
		expire, err := strconv.ParseInt(strings.TrimSpace(rawValue), 10, 64)
		if err != nil || expire <= 0 {
			return nil, "订阅到期时间格式无效"
		}
		timestamp := time.Unix(expire, 0).Local()
		return &timestamp, ""
	}
	return nil, ""
}

func summarize(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > 180 {
		return string(runes[:180]) + "..."
	}
	return value
}
