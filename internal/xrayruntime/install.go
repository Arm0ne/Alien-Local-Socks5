package xrayruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

const (
	Version        = "26.3.27"
	ExpectedSHA256 = "15c2d007954ac53ba69b80ec91242786b3c0b71d52649165b4ca1d5cc96ef8f1"
)

func AppDataDirectory() (string, error) {
	root, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("无法确定当前用户的本地数据目录：%w", err)
	}
	return filepath.Join(root, "RealityLocal"), nil
}

func Ensure() (string, error) {
	baseDirectory, err := AppDataDirectory()
	if err != nil {
		return "", err
	}
	if len(embeddedXray) == 0 {
		return developmentXray()
	}
	return Install(baseDirectory, embeddedXray, ExpectedSHA256)
}

func Install(baseDirectory string, executable []byte, expectedHash string) (string, error) {
	if len(executable) == 0 {
		return "", fmt.Errorf("软件未包含 Xray-core")
	}
	if !matchesSHA256(executable, expectedHash) {
		return "", fmt.Errorf("内置 Xray-core 完整性检查失败")
	}

	runtimeDirectory := filepath.Join(baseDirectory, "runtime", "xray-"+Version)
	if err := os.MkdirAll(runtimeDirectory, 0o700); err != nil {
		return "", fmt.Errorf("创建 Xray 运行目录失败：%w", err)
	}
	targetPath := filepath.Join(runtimeDirectory, "xray.exe")
	if current, err := os.ReadFile(targetPath); err == nil && matchesSHA256(current, expectedHash) {
		return targetPath, nil
	}

	temporary, err := os.CreateTemp(runtimeDirectory, ".xray-*.exe")
	if err != nil {
		return "", fmt.Errorf("创建 Xray 临时文件失败：%w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(executable); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("释放 Xray-core 失败：%w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("同步 Xray-core 失败：%w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", fmt.Errorf("关闭 Xray 临时文件失败：%w", err)
	}
	if err := moveReplace(temporaryPath, targetPath); err != nil {
		return "", fmt.Errorf("安装 Xray-core 失败：%w", err)
	}
	return targetPath, nil
}

func developmentXray() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("XRAY_EXE")); configured != "" {
		absolute, err := filepath.Abs(configured)
		if err == nil {
			if info, statErr := os.Stat(absolute); statErr == nil && info.Mode().IsRegular() {
				return absolute, nil
			}
		}
	}
	executablePath, _ := os.Executable()
	adjacent := filepath.Join(filepath.Dir(executablePath), "xray.exe")
	if info, err := os.Stat(adjacent); err == nil && info.Mode().IsRegular() {
		return adjacent, nil
	}
	return "", fmt.Errorf("开发版本未找到 xray.exe；请设置 XRAY_EXE 或将 xray.exe 放在程序旁边")
}

func matchesSHA256(content []byte, expected string) bool {
	actual := sha256.Sum256(content)
	return strings.EqualFold(hex.EncodeToString(actual[:]), strings.TrimSpace(expected))
}

func moveReplace(sourcePath, destinationPath string) error {
	source, err := windows.UTF16PtrFromString(sourcePath)
	if err != nil {
		return err
	}
	destination, err := windows.UTF16PtrFromString(destinationPath)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(source, destination, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
