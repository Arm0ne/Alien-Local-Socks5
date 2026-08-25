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
	service.mu.Lock()
	service.sourceTXT = data.SourceTXT
	service.result = result
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
	return service.configure(ctx, string(content), startPort)
}

func (service *Service) Reconfigure(ctx context.Context, startPort int) (converter.ParseResult, error) {
	if service.session.IsRunning() {
		return converter.ParseResult{}, fmt.Errorf("请先停止当前代理，再修改端口")
	}
	service.mu.Lock()
	source := service.sourceTXT
	service.mu.Unlock()
	if source == "" {
		return converter.ParseResult{}, profile.ErrNotFound
	}
	return service.configure(ctx, source, startPort)
}

func (service *Service) RemoveNode(ctx context.Context, index, startPort int) (converter.ParseResult, error) {
	if service.session.IsRunning() {
		return converter.ParseResult{}, fmt.Errorf("请先停止当前代理，再删除节点")
	}
	service.mu.Lock()
	source := service.sourceTXT
	current := service.result
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
		service.mu.Unlock()
		return converter.ParseResult{}, nil
	}
	return service.configure(ctx, remainingSource, startPort)
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
	service.mu.Unlock()
	if source == "" || len(current.Nodes) == 0 {
		return converter.ParseResult{}, profile.ErrNotFound
	}
	if len(current.Mappings) == 0 || current.Mappings[0].ListenPort != startPort {
		var err error
		current, err = service.configure(ctx, source, startPort)
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

func (service *Service) configure(ctx context.Context, source string, startPort int) (converter.ParseResult, error) {
	result, err := converter.ParseText(source, startPort)
	if err != nil {
		return converter.ParseResult{}, err
	}
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
	if err := service.store.Save(source, startPort); err != nil {
		return converter.ParseResult{}, err
	}
	service.mu.Lock()
	service.sourceTXT = source
	service.result = result
	service.mu.Unlock()
	return result, nil
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
