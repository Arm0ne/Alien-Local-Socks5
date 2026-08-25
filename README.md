# Reality Local

Reality Local 是一个 Windows 单文件桌面程序，将 TXT 中的 VLESS + TCP + Reality 节点转换为多个固定的本地 SOCKS5 端口，并在同一个界面中管理 Xray-core 的启动、停止、监听状态和出口 IP 检测。

用户不需要操作 `xray.exe`、`config.json`、PowerShell 或启动脚本。

## 使用

运行：

```text
dist\Reality Local.exe
```

操作流程：

1. 点击“导入节点”，选择每行一个 `vless://` 链接的 TXT。
2. 确认起始端口，默认 `21001`。
3. 配置检查通过后点击“启动”。
4. 等待顶部显示 SOCKS5 已监听数量和出口检测结果。
5. 在 ADS 中使用表格对应的本地地址，例如 `127.0.0.1:21001`。
6. 使用结束后点击“停止”或关闭软件。

主界面只显示当前状态、本地 SOCKS5 地址、出口 IP 和最后检测时间，不显示节点名称、UUID、公钥或延迟。

## 运行行为

- Xray-core `v26.3.27` 已嵌入 `Reality Local.exe`。
- 只监听 `127.0.0.1`。
- 不启用 TUN、透明代理或 Windows 系统代理。
- 不修改 V2Ray、Clash、sing-box 或其他 Xray 配置。
- 每个本地端口固定对应一个 Reality 出站。
- 未匹配流量进入 `blackhole`，不会自动直连。
- 只停止本软件创建的 Xray 子进程。
- 关闭软件会停止本软件管理的全部 SOCKS5 端口。

节点 TXT 会使用 Windows DPAPI 按当前 Windows 用户加密保存。Xray 明文配置只在启动期间临时生成，端口开始监听后立即删除。日志不会输出完整 UUID、公钥或 VLESS 链接。

## 出口检测

启动成功后，软件通过每个本地 SOCKS5 端口分别访问 IP 检测服务：

- 域名通过对应 SOCKS5 代理解析。
- 检测失败不会回退到本机直连。
- 每次最多并发检测 3 个端口。
- 检测失败只标记对应端口，不会停止其他端口。
- 可以检测全部端口或单独检测选中的端口。

示例节点不可连接时，会出现“SOCKS5 已监听”但“检测失败”。这表示本地 Xray 已启动，远程节点或出口不可用。

## 端口冲突

如果端口被其他程序占用，软件不会结束占用进程，也不会静默修改端口。界面会列出冲突端口并推荐一段空闲范围，用户确认后再使用新端口启动。

## 输入格式

每行一个完整 `vless://` 链接。空行以及以 `#`、`//` 开头的注释行会被忽略。

当前仅接受 VLESS + TCP + Reality，并支持：

- `type` / `network`
- `pbk` / `publicKey`
- `fp` / `fingerprint`
- `sni` / `serverName`
- `sid` / `shortId`
- URL 编码的 `spx` 和节点名称

仓库中的 `nodes.example.txt` 只用于配置格式测试，不对应可连接的真实节点。

## 构建

开发机需要 Go。构建脚本会验证内置 Xray 的 SHA-256、运行测试，并生成单文件 EXE：

```powershell
.\scripts\build.ps1
```

构建产物：

```text
dist\Reality Local.exe
dist\reality-converter.exe
```

`reality-converter.exe` 是保留的 `v1.0.0` 独立转换器；新用户应使用 `Reality Local.exe`。

第三方组件信息见 `THIRD-PARTY-NOTICES.txt`。
