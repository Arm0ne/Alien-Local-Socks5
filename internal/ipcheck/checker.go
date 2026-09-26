package ipcheck

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/proxy"
)

const maxResponseBytes = 256

var defaultEndpoints = []string{
	"https://api.ipify.org",
	"https://api64.ipify.org",
}

type Checker struct {
	Endpoints []string
	Timeout   time.Duration
}

func (checker Checker) Check(ctx context.Context, port int) (string, error) {
	if port < 1 || port > 65535 {
		return "", fmt.Errorf("SOCKS5 端口无效")
	}
	timeout := checker.Timeout
	if timeout <= 0 {
		timeout = 12 * time.Second
	}
	endpoints := checker.Endpoints
	if len(endpoints) == 0 {
		endpoints = defaultEndpoints
	}

	baseDialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 15 * time.Second}
	socksDialer, err := proxy.SOCKS5("tcp", fmt.Sprintf("127.0.0.1:%d", port), nil, baseDialer)
	if err != nil {
		return "", fmt.Errorf("创建 SOCKS5 检测连接失败：%w", err)
	}
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           dialContext(socksDialer),
		DisableKeepAlives:     true,
		TLSHandshakeTimeout:   6 * time.Second,
		ResponseHeaderTimeout: 8 * time.Second,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: timeout}

	var failures []string
	for _, endpoint := range endpoints {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}
		request.Header.Set("User-Agent", "RealityLocal/1.1")
		response, err := client.Do(request)
		if err != nil {
			failures = append(failures, summarize(err.Error()))
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
		_ = response.Body.Close()
		if readErr != nil {
			failures = append(failures, "读取检测结果失败")
			continue
		}
		if response.StatusCode != http.StatusOK {
			failures = append(failures, fmt.Sprintf("检测服务返回 HTTP %d", response.StatusCode))
			continue
		}
		if len(body) > maxResponseBytes {
			failures = append(failures, "检测服务响应过长")
			continue
		}
		address := strings.TrimSpace(string(body))
		if net.ParseIP(address) == nil {
			failures = append(failures, "检测服务未返回有效 IP")
			continue
		}
		return address, nil
	}
	return "", fmt.Errorf("出口检测失败：%s", strings.Join(failures, "；"))
}

func dialContext(dialer proxy.Dialer) func(context.Context, string, string) (net.Conn, error) {
	if contextDialer, ok := dialer.(proxy.ContextDialer); ok {
		return contextDialer.DialContext
	}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		type result struct {
			connection net.Conn
			err        error
		}
		completed := make(chan result)
		go func() {
			connection, err := dialer.Dial(network, address)
			select {
			case completed <- result{connection: connection, err: err}:
			case <-ctx.Done():
				if connection != nil {
					_ = connection.Close()
				}
			}
		}()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case outcome := <-completed:
			return outcome.connection, outcome.err
		}
	}
}

func summarize(message string) string {
	message = strings.ReplaceAll(message, "\r", " ")
	message = strings.ReplaceAll(message, "\n", " ")
	runes := []rune(strings.TrimSpace(message))
	if len(runes) > 160 {
		return string(runes[:160]) + "..."
	}
	return string(runes)
}
