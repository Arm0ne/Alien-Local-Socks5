# Reality 链接转换器

Windows 便携式转换工具，将 TXT 中的 VLESS + TCP + Reality 链接转换为 Xray-core 原生 JSON。每个节点获得一个只监听 `127.0.0.1` 的固定 SOCKS5 端口。

当前交付范围仅包含转换器，不包含 Xray 启动器。

## 使用

将以下文件放在同一目录：

```text
reality-converter.exe
xray.exe
nodes.txt
```

打开 `reality-converter.exe`：

1. 选择 `nodes.txt`。
2. 将输出位置设置为同目录的 `config.json`。
3. 设置起始 SOCKS5 端口，默认 `21001`。
4. 点击“转换并检查”。

转换器会调用输出目录中的：

```text
xray.exe run -test -config <临时配置>
```

只有检查通过后才会写入正式 `config.json`。失败不会覆盖已有配置。

## 输入格式

每行一个完整 `vless://` 链接。空行以及以 `#`、`//` 开头的注释行会被忽略。

当前仅接受：

- VLESS
- TCP
- Reality
- `encryption=none`

支持 `type/network`、`pbk/publicKey`、`fp/fingerprint`、`sni/serverName`、`sid/shortId` 参数别名，并正确解码 URL 查询参数和节点名称。

## 构建

开发机需要 Go。构建脚本只用于开发，生成的 EXE 不依赖 PowerShell 或额外 .NET Runtime。

```powershell
.\scripts\build.ps1
```

产物位于 `dist\reality-converter.exe`。

仓库中的 `nodes.example.txt` 只用于本地配置检查，不对应可连接的真实节点。
