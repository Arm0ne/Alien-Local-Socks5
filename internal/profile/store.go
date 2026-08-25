package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"

	"realityconverter/internal/securestore"
)

const currentVersion = 1

const (
	SourceKindFile         = "file"
	SourceKindSubscription = "subscription"
)

var ErrNotFound = errors.New("尚未导入节点")

type Data struct {
	Version                  int    `json:"version"`
	SourceTXT                string `json:"sourceTxt"`
	StartPort                int    `json:"startPort"`
	SourceKind               string `json:"sourceKind,omitempty"`
	SubscriptionURL          string `json:"subscriptionUrl,omitempty"`
	SubscriptionExpiresAt    int64  `json:"subscriptionExpiresAt,omitempty"`
	SubscriptionFetchedAt    int64  `json:"subscriptionFetchedAt,omitempty"`
	SubscriptionMetadataNote string `json:"subscriptionMetadataNote,omitempty"`
	ListenPorts              []int  `json:"listenPorts,omitempty"`
}

type Store struct {
	Path string
}

func (store Store) Load() (Data, error) {
	cipherText, err := os.ReadFile(store.Path)
	if errors.Is(err, os.ErrNotExist) {
		return Data{}, ErrNotFound
	}
	if err != nil {
		return Data{}, fmt.Errorf("读取节点档案失败：%w", err)
	}
	plainText, err := securestore.Unprotect(cipherText)
	if err != nil {
		return Data{}, err
	}
	defer clear(plainText)
	var data Data
	if err := json.Unmarshal(plainText, &data); err != nil {
		return Data{}, fmt.Errorf("节点档案格式无效：%w", err)
	}
	if data.Version != currentVersion || data.SourceTXT == "" {
		return Data{}, fmt.Errorf("节点档案版本不受支持")
	}
	if data.SourceKind == "" {
		data.SourceKind = SourceKindFile
	}
	if data.SourceKind != SourceKindFile && data.SourceKind != SourceKindSubscription {
		return Data{}, fmt.Errorf("节点档案来源类型不受支持")
	}
	if data.SourceKind == SourceKindSubscription && data.SubscriptionURL == "" {
		return Data{}, fmt.Errorf("订阅节点档案缺少订阅地址")
	}
	return data, nil
}

func (store Store) Save(sourceTXT string, startPort int) error {
	return store.SaveData(Data{Version: currentVersion, SourceTXT: sourceTXT, StartPort: startPort, SourceKind: SourceKindFile})
}

func (store Store) SaveSubscription(sourceTXT, subscriptionURL string, expiresAt, fetchedAt *time.Time, metadataNotice string, startPort int, listenPorts []int) error {
	data := Data{
		Version:                  currentVersion,
		SourceTXT:                sourceTXT,
		StartPort:                startPort,
		SourceKind:               SourceKindSubscription,
		SubscriptionURL:          subscriptionURL,
		SubscriptionMetadataNote: metadataNotice,
		ListenPorts:              append([]int(nil), listenPorts...),
	}
	if expiresAt != nil {
		data.SubscriptionExpiresAt = expiresAt.Unix()
	}
	if fetchedAt != nil {
		data.SubscriptionFetchedAt = fetchedAt.Unix()
	}
	return store.SaveData(data)
}

func (store Store) SaveData(data Data) error {
	data.Version = currentVersion
	plainText, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("生成节点档案失败：%w", err)
	}
	defer clear(plainText)
	cipherText, err := securestore.Protect(plainText)
	if err != nil {
		return err
	}
	defer clear(cipherText)
	if err := os.MkdirAll(filepath.Dir(store.Path), 0o700); err != nil {
		return fmt.Errorf("创建数据目录失败：%w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(store.Path), ".profile-*.dat")
	if err != nil {
		return fmt.Errorf("创建节点档案临时文件失败：%w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(cipherText); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("写入节点档案失败：%w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("同步节点档案失败：%w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("关闭节点档案失败：%w", err)
	}
	return moveReplace(temporaryPath, store.Path)
}

func (store Store) Delete() error {
	if err := os.Remove(store.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("删除节点档案失败：%w", err)
	}
	return nil
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
	if err := windows.MoveFileEx(source, destination, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH); err != nil {
		return fmt.Errorf("保存节点档案失败：%w", err)
	}
	return nil
}
