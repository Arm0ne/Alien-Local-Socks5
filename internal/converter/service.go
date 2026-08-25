package converter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

const (
	maxInputFileBytes = 10 * 1024 * 1024
	defaultCheckTime  = 30 * time.Second
)

type ConvertOptions struct {
	InputPath  string
	OutputPath string
	StartPort  int
	Timeout    time.Duration
}

type ConfigChecker interface {
	Check(ctx context.Context, xrayPath, configPath string, nodes []Node) error
}

type ProcessChecker struct{}

func ConvertFile(ctx context.Context, options ConvertOptions, checker ConfigChecker) (ParseResult, error) {
	inputPath, err := filepath.Abs(strings.TrimSpace(options.InputPath))
	if err != nil || strings.TrimSpace(options.InputPath) == "" {
		return ParseResult{}, fmt.Errorf("输入 TXT 路径无效")
	}
	outputPath, err := filepath.Abs(strings.TrimSpace(options.OutputPath))
	if err != nil || strings.TrimSpace(options.OutputPath) == "" {
		return ParseResult{}, fmt.Errorf("输出 JSON 路径无效")
	}
	if strings.EqualFold(inputPath, outputPath) {
		return ParseResult{}, fmt.Errorf("输入 TXT 和输出 JSON 不能是同一个文件")
	}
	if !strings.EqualFold(filepath.Ext(inputPath), ".txt") {
		return ParseResult{}, fmt.Errorf("输入文件必须是 .txt 文件")
	}
	if !strings.EqualFold(filepath.Ext(outputPath), ".json") {
		return ParseResult{}, fmt.Errorf("输出文件必须是 .json 文件")
	}

	info, err := os.Stat(inputPath)
	if err != nil {
		return ParseResult{}, fmt.Errorf("无法读取输入 TXT：%w", err)
	}
	if !info.Mode().IsRegular() {
		return ParseResult{}, fmt.Errorf("输入路径不是普通文件")
	}
	if info.Size() > maxInputFileBytes {
		return ParseResult{}, fmt.Errorf("输入 TXT 超过 10 MiB 限制")
	}

	content, err := os.ReadFile(inputPath)
	if err != nil {
		return ParseResult{}, fmt.Errorf("读取输入 TXT 失败：%w", err)
	}
	result, err := ParseText(string(content), options.StartPort)
	if err != nil {
		return ParseResult{}, err
	}
	configJSON, err := BuildConfig(result)
	if err != nil {
		return ParseResult{}, fmt.Errorf("生成 Xray JSON 失败：%w", err)
	}
	var structure any
	if err := json.Unmarshal(configJSON, &structure); err != nil {
		return ParseResult{}, fmt.Errorf("内部 JSON 完整性检查失败：%w", err)
	}

	outputDirectory := filepath.Dir(outputPath)
	if info, err := os.Stat(outputDirectory); err != nil || !info.IsDir() {
		return ParseResult{}, fmt.Errorf("输出目录不存在或不可访问：%s", outputDirectory)
	}
	xrayPath := filepath.Join(outputDirectory, "xray.exe")
	if info, err := os.Stat(xrayPath); err != nil || !info.Mode().IsRegular() {
		return ParseResult{}, fmt.Errorf("未找到配置检查程序：%s", xrayPath)
	}

	temporary, err := os.CreateTemp(outputDirectory, ".reality-config-*.json")
	if err != nil {
		return ParseResult{}, fmt.Errorf("创建临时配置失败：%w", err)
	}
	temporaryPath := temporary.Name()
	temporaryMoved := false
	defer func() {
		if !temporaryMoved {
			_ = os.Remove(temporaryPath)
		}
	}()

	if _, err := temporary.Write(configJSON); err != nil {
		_ = temporary.Close()
		return ParseResult{}, fmt.Errorf("写入临时配置失败：%w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return ParseResult{}, fmt.Errorf("同步临时配置失败：%w", err)
	}
	if err := temporary.Close(); err != nil {
		return ParseResult{}, fmt.Errorf("关闭临时配置失败：%w", err)
	}

	if checker == nil {
		checker = ProcessChecker{}
	}
	checkTimeout := options.Timeout
	if checkTimeout <= 0 {
		checkTimeout = defaultCheckTime
	}
	checkContext, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	if err := checker.Check(checkContext, xrayPath, temporaryPath, result.Nodes); err != nil {
		return ParseResult{}, err
	}

	if err := replaceFile(temporaryPath, outputPath); err != nil {
		return ParseResult{}, fmt.Errorf("写入正式配置失败：%w", err)
	}
	temporaryMoved = true
	return result, nil
}

func (ProcessChecker) Check(ctx context.Context, xrayPath, configPath string, nodes []Node) error {
	command := exec.CommandContext(ctx, xrayPath, "run", "-test", "-config", configPath)
	command.Dir = filepath.Dir(xrayPath)
	command.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_NO_WINDOW,
	}
	output, err := command.CombinedOutput()
	detail := strings.TrimSpace(Redact(string(output), nodes))
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("Xray 配置检查超时")
	}
	if err == nil {
		return nil
	}
	if detail == "" {
		detail = Redact(err.Error(), nodes)
	}
	return fmt.Errorf("Xray 配置检查失败：\r\n%s", detail)
}

func replaceFile(sourcePath, destinationPath string) error {
	source, err := windows.UTF16PtrFromString(sourcePath)
	if err != nil {
		return err
	}
	destination, err := windows.UTF16PtrFromString(destinationPath)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(
		source,
		destination,
		windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH,
	)
}
