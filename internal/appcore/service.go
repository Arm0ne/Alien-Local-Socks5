package appcore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"realityconverter/internal/converter"
	"realityconverter/internal/profile"
	"realityconverter/internal/session"
	"realityconverter/internal/subscription"
	"realityconverter/internal/xrayruntime"
)

const maxInputFileBytes = 10 * 1024 * 1024

type Service struct {
	mu        sync.Mutex
	dataDir   string
	store     profile.Store
	session   *session.Manager
	sourceTXT string
	result    converter.ParseResult
	source    SourceInfo
}

type SourceInfo struct {
	Kind            string
	SubscriptionURL string
	ExpiresAt       *time.Time
	FetchedAt       *time.Time
	MetadataNotice  string
}

type SubscriptionDraft struct {
	SourceTXT      string
	Result         converter.ParseResult
	URL            string
	ExpiresAt      *time.Time
	FetchedAt      time.Time
	MetadataNotice string
}

func New() (*Service, error) {
	dataDirectory, err := xrayruntime.AppDataDirectory()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dataDirectory, 0o700); err != nil {
		return nil, fmt.Errorf("创建软件数据目录失败：%w", err)
	}
	cleanupStaleConfigs(filepath.Join(dataDirectory, "session"))
	return &Service{
		dataDir: dataDirectory,
		store:   profile.Store{Path: filepath.Join(dataDirectory, "profile.dat")},
		session: session.NewManager(),
	}, nil
}

func (service *Service) Load() (converter.ParseResult, error) {
	data, err := service.store.Load()
	if errors.Is(err, profile.ErrNotFound) {
		return converter.ParseResult{}, profile.ErrNotFound
	}
	if err != nil {
		return converter.ParseResult{}, err
	}
	result, err := converter.ParseText(data.SourceTXT, data.StartPort)
	if err != nil {
		return converter.ParseResult{}, fmt.Errorf("已保存节点无法加载：%w", err)
	}
	result, err = applyStoredPorts(result, data.ListenPorts)
	if err != nil {
		return converter.ParseResult{}, fmt.Errorf("已保存节点端口映射无效：%w", err)
	}
	service.mu.Lock()
	service.sourceTXT = data.SourceTXT
	service.result = result
	service.source = sourceInfoFromProfile(data)
	service.mu.Unlock()
	return result, nil
}

func (service *Service) ImportFile(ctx context.Context, path string, startPort int) (converter.ParseResult, error) {
	if service.session.IsRunning() {
		return converter.ParseResult{}, fmt.Errorf("请先停止当前代理，再导入节点")
	}
	info, err := os.Stat(path)
	if err != nil {
		return converter.ParseResult{}, fmt.Errorf("读取节点 TXT 失败：%w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > maxInputFileBytes {
		return converter.ParseResult{}, fmt.Errorf("节点 TXT 无效或超过 10 MiB")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return converter.ParseResult{}, fmt.Errorf("读取节点 TXT 失败：%w", err)
	}
	return service.configure(ctx, string(content), startPort, SourceInfo{Kind: profile.SourceKindFile})
}

func (service *Service) FetchSubscription(ctx context.Context, rawURL string, startPort int) (SubscriptionDraft, error) {
	if service.session.IsRunning() {
		return SubscriptionDraft{}, fmt.Errorf("请先停止当前代理，再替换订阅")
	}
	document, err := (subscription.Fetcher{}).Fetch(ctx, rawURL)
	if err != nil {
		return SubscriptionDraft{}, err
	}
	result, err := converter.ParseText(document.SourceTXT, startPort)
	if err != nil {
		return SubscriptionDraft{}, fmt.Errorf("订阅节点解析失败：%w", err)
	}
	return SubscriptionDraft{
		SourceTXT:      document.SourceTXT,
		Result:         result,
		URL:            document.URL,
		ExpiresAt:      document.ExpiresAt,
		FetchedAt:      document.FetchedAt,
		MetadataNotice: document.MetadataNotice,
	}, nil
}

func (service *Service) ApplySubscription(ctx context.Context, draft SubscriptionDraft, startPort int) (converter.ParseResult, error) {
	if service.session.IsRunning() {
		return converter.ParseResult{}, fmt.Errorf("请先停止当前代理，再替换订阅")
	}
	service.mu.Lock()
	previous := service.result
	service.mu.Unlock()
	result := preservePorts(previous, draft.Result, startPort)
	return service.configureResult(ctx, draft.SourceTXT, startPort, result, SourceInfo{
		Kind:            profile.SourceKindSubscription,
		SubscriptionURL: draft.URL,
		ExpiresAt:       draft.ExpiresAt,
		FetchedAt:       &draft.FetchedAt,
		MetadataNotice:  draft.MetadataNotice,
	})
}

func (service *Service) SourceInfo() SourceInfo {
	service.mu.Lock()
	defer service.mu.Unlock()
	return copySourceInfo(service.source)
}

func (service *Service) Reconfigure(ctx context.Context, startPort int) (converter.ParseResult, error) {
	if service.session.IsRunning() {
		return converter.ParseResult{}, fmt.Errorf("请先停止当前代理，再修改端口")
	}
	service.mu.Lock()
	source := service.sourceTXT
	info := service.source
	service.mu.Unlock()
	if source == "" {
		return converter.ParseResult{}, profile.ErrNotFound
	}
	return service.configure(ctx, source, startPort, info)
}

func (service *Service) RemoveNode(ctx context.Context, index, startPort int) (converter.ParseResult, error) {
	if service.session.IsRunning() {
		return converter.ParseResult{}, fmt.Errorf("请先停止当前代理，再删除节点")
	}
	service.mu.Lock()
	source := service.sourceTXT
	current := service.result
	info := service.source
	service.mu.Unlock()
	if source == "" || index < 0 || index >= len(current.Nodes) {
		return converter.ParseResult{}, fmt.Errorf("选中的节点不存在")
	}

	remainingSource, err := sourceWithoutNode(current.Nodes, index)
	if err != nil {
		return converter.ParseResult{}, err
	}
	if remainingSource == "" {
		if err := service.store.Delete(); err != nil {
			return converter.ParseResult{}, err
		}
		service.mu.Lock()
		service.sourceTXT = ""
		service.result = converter.ParseResult{}
		service.source = SourceInfo{}
		service.mu.Unlock()
		return converter.ParseResult{}, nil
	}
	remainingResult, err := converter.ParseText(remainingSource, startPort)
	if err != nil {
		return converter.ParseResult{}, err
	}
	remainingResult = preservePorts(current, remainingResult, startPort)
	return service.configureResult(ctx, remainingSource, startPort, remainingResult, info)
}

func sourceWithoutNode(nodes []converter.Node, index int) (string, error) {
	if index < 0 || index >= len(nodes) {
		return "", fmt.Errorf("选中的节点不存在")
	}
	links := make([]string, 0, len(nodes)-1)
	for nodeIndex, node := range nodes {
		if nodeIndex != index {
			links = append(links, node.RawLink)
		}
	}
	return strings.Join(links, "\r\n"), nil
}

func (service *Service) Start(ctx context.Context, startPort int) (converter.ParseResult, error) {
	service.mu.Lock()
	source := service.sourceTXT
	current := service.result
	info := service.source
	service.mu.Unlock()
	if source == "" || len(current.Nodes) == 0 {
		return converter.ParseResult{}, profile.ErrNotFound
	}
	if len(current.Mappings) == 0 || current.Mappings[0].ListenPort != startPort {
		var err error
		current, err = service.configure(ctx, source, startPort, info)
		if err != nil {
			return converter.ParseResult{}, err
		}
	}
	configJSON, err := converter.BuildConfig(current)
	if err != nil {
		return converter.ParseResult{}, err
	}
	xrayPath, err := xrayruntime.Ensure()
	if err != nil {
		return converter.ParseResult{}, err
	}
	if err := service.session.Start(ctx, xrayPath, filepath.Join(service.dataDir, "session"), configJSON, current); err != nil {
		return converter.ParseResult{}, err
	}
	return current, nil
}

func (service *Service) Stop(ctx context.Context) error {
	return service.session.Stop(ctx)
}

func (service *Service) IsRunning() bool {
	return service.session.IsRunning()
}

func (service *Service) Events() <-chan session.Event {
	return service.session.Events()
}

func (service *Service) configure(ctx context.Context, source string, startPort int, info SourceInfo) (converter.ParseResult, error) {
	result, err := converter.ParseText(source, startPort)
	if err != nil {
		return converter.ParseResult{}, err
	}
	return service.configureResult(ctx, source, startPort, result, info)
}

func (service *Service) configureResult(ctx context.Context, source string, startPort int, result converter.ParseResult, info SourceInfo) (converter.ParseResult, error) {
	configJSON, err := converter.BuildConfig(result)
	if err != nil {
		return converter.ParseResult{}, err
	}
	xrayPath, err := xrayruntime.Ensure()
	if err != nil {
		return converter.ParseResult{}, err
	}
	if err := checkConfiguration(ctx, service.dataDir, xrayPath, configJSON, result.Nodes); err != nil {
		return converter.ParseResult{}, err
	}
	var saveErr error
	if info.Kind == profile.SourceKindSubscription {
		saveErr = service.store.SaveSubscription(source, info.SubscriptionURL, info.ExpiresAt, info.FetchedAt, info.MetadataNotice, startPort, mappingPorts(result))
	} else {
		saveErr = service.store.Save(source, startPort)
	}
	if saveErr != nil {
		return converter.ParseResult{}, saveErr
	}
	service.mu.Lock()
	service.sourceTXT = source
	service.result = result
	service.source = copySourceInfo(info)
	service.mu.Unlock()
	return result, nil
}

func mappingPorts(result converter.ParseResult) []int {
	ports := make([]int, len(result.Mappings))
	for index, mapping := range result.Mappings {
		ports[index] = mapping.ListenPort
	}
	return ports
}

func applyStoredPorts(result converter.ParseResult, ports []int) (converter.ParseResult, error) {
	if len(ports) == 0 {
		return result, nil
	}
	if len(ports) != len(result.Mappings) {
		return converter.ParseResult{}, fmt.Errorf("保存的端口数量与节点数量不一致")
	}
	seen := make(map[int]struct{}, len(ports))
	for index, port := range ports {
		if port < 1 || port > 65535 {
			return converter.ParseResult{}, fmt.Errorf("保存的端口 %d 超出范围", port)
		}
		if _, exists := seen[port]; exists {
			return converter.ParseResult{}, fmt.Errorf("保存的端口 %d 重复", port)
		}
		seen[port] = struct{}{}
		result.Mappings[index].ListenPort = port
	}
	return result, nil
}

func sourceInfoFromProfile(data profile.Data) SourceInfo {
	info := SourceInfo{Kind: data.SourceKind, SubscriptionURL: data.SubscriptionURL, MetadataNotice: data.SubscriptionMetadataNote}
	if info.Kind == "" {
		info.Kind = profile.SourceKindFile
	}
	if data.SubscriptionExpiresAt > 0 {
		expiresAt := time.Unix(data.SubscriptionExpiresAt, 0).Local()
		info.ExpiresAt = &expiresAt
	}
	if data.SubscriptionFetchedAt > 0 {
		fetchedAt := time.Unix(data.SubscriptionFetchedAt, 0).Local()
		info.FetchedAt = &fetchedAt
	}
	return info
}

func copySourceInfo(info SourceInfo) SourceInfo {
	copy := info
	if info.ExpiresAt != nil {
		expiresAt := *info.ExpiresAt
		copy.ExpiresAt = &expiresAt
	}
	if info.FetchedAt != nil {
		fetchedAt := *info.FetchedAt
		copy.FetchedAt = &fetchedAt
	}
	return copy
}

func preservePorts(previous, next converter.ParseResult, startPort int) converter.ParseResult {
	if len(previous.Nodes) == 0 || len(previous.Mappings) == 0 || len(next.Nodes) == 0 || len(previous.Nodes) != len(previous.Mappings) || len(next.Nodes) != len(next.Mappings) {
		return next
	}
	oldPorts := make(map[string]int, len(previous.Nodes))
	usedPorts := make(map[int]struct{}, len(previous.Mappings))
	maxPort := startPort - 1
	for index, node := range previous.Nodes {
		port := previous.Mappings[index].ListenPort
		oldPorts[converter.NodeKey(node)] = port
		usedPorts[port] = struct{}{}
		if port > maxPort {
			maxPort = port
		}
	}
	nextPort := maxPort + 1
	for index, node := range next.Nodes {
		mapping := next.Mappings[index]
		if oldPort, exists := oldPorts[converter.NodeKey(node)]; exists {
			mapping.ListenPort = oldPort
		} else {
			for nextPort <= 65535 {
				if _, exists := usedPorts[nextPort]; !exists {
					break
				}
				nextPort++
			}
			if nextPort > 65535 {
				return next
			}
			mapping.ListenPort = nextPort
			usedPorts[nextPort] = struct{}{}
			nextPort++
		}
		mapping.Index = index + 1
		next.Mappings[index] = mapping
	}
	return next
}

func checkConfiguration(ctx context.Context, dataDirectory, xrayPath string, configJSON []byte, nodes []converter.Node) error {
	checkDirectory := filepath.Join(dataDirectory, "check")
	if err := os.MkdirAll(checkDirectory, 0o700); err != nil {
		return fmt.Errorf("创建配置检查目录失败：%w", err)
	}
	temporary, err := os.CreateTemp(checkDirectory, ".config-*.json")
	if err != nil {
		return fmt.Errorf("创建配置检查文件失败：%w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(configJSON); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("写入配置检查文件失败：%w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("关闭配置检查文件失败：%w", err)
	}
	checkContext, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return (converter.ProcessChecker{}).Check(checkContext, xrayPath, temporaryPath, nodes)
}

func cleanupStaleConfigs(directory string) {
	matches, _ := filepath.Glob(filepath.Join(directory, ".running-config-*.json"))
	for _, path := range matches {
		_ = os.Remove(path)
	}
}
