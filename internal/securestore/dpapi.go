package securestore

import (
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var entropy = []byte("RealityLocal/Profile/v1")

func Protect(plainText []byte) ([]byte, error) {
	if len(plainText) == 0 {
		return nil, fmt.Errorf("不能加密空数据")
	}
	input := dataBlob(plainText)
	additionalEntropy := dataBlob(entropy)
	var output windows.DataBlob
	if err := windows.CryptProtectData(
		&input,
		nil,
		&additionalEntropy,
		0,
		nil,
		windows.CRYPTPROTECT_UI_FORBIDDEN,
		&output,
	); err != nil {
		return nil, fmt.Errorf("Windows 凭据加密失败：%w", err)
	}
	defer windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(output.Data))))
	runtime.KeepAlive(plainText)
	runtime.KeepAlive(entropy)
	return append([]byte(nil), unsafe.Slice(output.Data, output.Size)...), nil
}

func Unprotect(cipherText []byte) ([]byte, error) {
	if len(cipherText) == 0 {
		return nil, fmt.Errorf("加密档案为空")
	}
	input := dataBlob(cipherText)
	additionalEntropy := dataBlob(entropy)
	var output windows.DataBlob
	if err := windows.CryptUnprotectData(
		&input,
		nil,
		&additionalEntropy,
		0,
		nil,
		windows.CRYPTPROTECT_UI_FORBIDDEN,
		&output,
	); err != nil {
		return nil, fmt.Errorf("无法解密节点档案：%w", err)
	}
	defer windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(output.Data))))
	runtime.KeepAlive(cipherText)
	runtime.KeepAlive(entropy)
	return append([]byte(nil), unsafe.Slice(output.Data, output.Size)...), nil
}

func dataBlob(data []byte) windows.DataBlob {
	return windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
}
