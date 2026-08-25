# Xray-core 本地 Reality 转换器需求

## 一、项目目标

开发一套 Windows 客户端工具，将 VLESS + Reality 节点转换为本地多个 SOCKS5 端口，供 ADS 指纹浏览器使用。

ADS 本身不需要支持 VLESS 或 Reality，只连接本机 SOCKS5；Xray-core 负责连接远程 Reality 节点。

```text
ADS 浏览器
  ↓ SOCKS5
127.0.0.1:21001
  ↓
Xray-core
  ↓ VLESS + Reality
远程节点
```

## 二、核心限制

Xray-core 必须完全独立运行：

- 只监听 `127.0.0.1`。
- 不启用 TUN。
- 不启用透明代理。
- 不修改 Windows 系统代理。
- 不接管系统其他流量。
- 不修改客户已有的 V2Ray、Clash 或 sing-box 配置。
- 不监听 `0.0.0.0`。
- 每个 ADS 端口固定对应一个 Reality 出站。
- 不自动做节点负载均衡或随机切换。
- 只使用用户明确提供的节点。

## 三、工具一：Reality 链接转换器

开发一个 Windows 独立工具，最好是不依赖 Python、PowerShell 或额外 .NET 运行时的便携式 EXE。

### 输入格式

- 输入一个 TXT 文件。
- 每行一个 `vless://` 链接。
- 支持单个节点和批量节点。
- 忽略空行。
- 忽略以 `#` 或 `//` 开头的注释行。
- 支持 URL 编码的查询参数和节点名称。

示例：

```text
vless://UUID@server:port?type=tcp&encryption=none&security=reality&pbk=PUBLIC_KEY&fp=chrome&sni=example.com&sid=1234&spx=%2F&flow=xtls-rprx-vision#node-name
```

### 需要解析的参数

- `server`
- `server_port`
- `uuid`
- `type` 或 `network`
- `encryption`
- `security`
- `pbk` 或 `publicKey`
- `fp` 或 `fingerprint`
- `sni` 或 `serverName`
- `sid` 或 `shortId`
- `spx`
- `flow`
- URL fragment 中的节点名称

当前支持：

- VLESS
- TCP
- Reality
- `flow=xtls-rprx-vision` 或链接中提供的其他 flow

如果输入不是 VLESS + TCP + Reality，或者缺少关键参数，必须显示具体错误和行号。

## 四、Xray JSON 输出

必须生成 Xray-core 原生 JSON，不能混用 sing-box 配置格式。

### SOCKS5 入站

每个节点生成一个 SOCKS5 入站：

- tag：`ads-01`、`ads-02`、`ads-03`……
- listen：`127.0.0.1`
- listen_port：默认从 `21001` 开始递增
- protocol：`socks`
- auth：`noauth`
- udp：`true`

### VLESS + Reality 出站

每个节点生成一个 VLESS + Reality 出站：

- tag：`reality-01`、`reality-02`、`reality-03`……
- protocol：`vless`
- server：原始链接服务器地址
- server_port：原始链接端口
- UUID：原始链接 UUID
- encryption：`none`
- flow：原始链接中的 flow

Xray Reality 客户端字段应使用以下结构：

```json
{
  "streamSettings": {
    "network": "tcp",
    "security": "reality",
    "realitySettings": {
      "serverName": "原始 sni",
      "fingerprint": "原始 fp",
      "publicKey": "原始 pbk",
      "shortId": "原始 sid",
      "spiderX": "原始 spx"
    }
  }
}
```

注意：

- Xray 使用 `publicKey`、`shortId`、`spiderX` 驼峰字段。
- 不要使用 sing-box 的 `tls.reality.public_key` 格式。
- `spx=%2F` 必须转换为 Xray 的 `spiderX: "/"`。
- `encryption=none` 必须转换为 VLESS 用户的 `encryption: "none"`。
- 必须正确处理只有一条链接的输入，不能生成 0 个节点。
- 单节点和批量节点都必须通过相同的转换流程。

## 五、路由规则

每个入站固定路由到对应出站：

```text
127.0.0.1:21001 → reality-01
127.0.0.1:21002 → reality-02
127.0.0.1:21003 → reality-03
```

Xray 路由规则示例：

```json
{
  "routing": {
    "domainStrategy": "AsIs",
    "rules": [
      {
        "type": "field",
        "inboundTag": ["ads-01"],
        "outboundTag": "reality-01"
      },
      {
        "type": "field",
        "inboundTag": ["ads-02"],
        "outboundTag": "reality-02"
      }
    ]
  }
}
```

所有未匹配流量必须进入 `blackhole`，不能自动直连：

```json
{
  "protocol": "blackhole",
  "tag": "block"
}
```

## 六、转换器功能

转换器应支持：

- 文件选择窗口。
- 选择输入 TXT。
- 选择输出 JSON。
- 自定义起始 SOCKS5 端口。
- 默认起始端口为 `21001`。
- 检查端口范围是否超过 `65535`。
- 检查重复节点和重复端口。
- 检查 UUID、服务器地址、端口、Reality 公钥、SNI、short ID。
- 显示转换成功的节点数量。
- 显示端口映射关系。
- 转换完成后自动运行 Xray 配置检查。
- 配置错误时显示 Xray 的实际错误。
- 不在日志中输出完整 UUID、公钥等敏感信息。

成功输出示例：

```text
Converted 8 nodes.

127.0.0.1:21001 -> reality-01
127.0.0.1:21002 -> reality-02
127.0.0.1:21003 -> reality-03
```

## 七、工具二：Xray 一键启动器

客户不应需要手动打开 PowerShell 或命令提示符。

### 目录结构

```text
xray.exe
config.json
start-xray.exe 或 start-xray.cmd
stop-xray.exe 或 stop-xray.cmd
status-xray.exe 或 status-xray.cmd
```

### 启动功能

- 检查同目录的 `xray.exe`。
- 检查同目录的 `config.json`。
- 执行 Xray 配置测试：

```text
xray.exe run -test -config config.json
```

- 后台隐藏启动：

```text
xray.exe run -config config.json
```

- 记录启动进程 PID。
- 显示启动成功或失败。
- 不弹出命令行窗口。
- 启动失败时显示清晰错误。

### 停止功能

- 只停止自己记录的 PID。
- 不能按进程名批量结束所有 `xray.exe`。
- 不能影响客户其他 Xray/V2Ray 进程。
- 停止后删除自己的 PID 文件。

### 状态功能

- 显示是否运行。
- 显示 PID。
- 显示配置文件路径。
- 显示本地监听端口。

## 八、客户使用方式

客户只需要把 `xray.exe`、`config.json` 和启动器放在同一文件夹，然后双击启动器。

ADS 中配置：

```text
类型：SOCKS5
地址：127.0.0.1
端口：21001
```

不同 ADS 配置使用不同端口：

```text
21001 → Reality 节点 1
21002 → Reality 节点 2
21003 → Reality 节点 3
```

## 九、测试要求

必须测试：

- 只有 1 条 Reality 链接。
- 5 条 Reality 链接。
- 8 条 Reality 链接。
- 10 条以上 Reality 链接。
- 空行和注释行。
- URL 编码的节点名称。
- `spx=%2F`。
- 不同 fingerprint。
- 不同服务器、端口、UUID、SNI、publicKey、shortId。
- 重复端口。
- 端口超过 `65535`。
- 缺少关键参数。
- 非 Reality 链接。
- 非 TCP 链接。

每个生成的配置都必须执行：

```text
xray.exe run -test -config config.json
```

然后使用 SOCKS5 测试：

```powershell
curl.exe --proxy socks5h://127.0.0.1:21001 https://api.ipify.org
```

应逐个测试每个本地端口，并确认端口对应的是正确的 Reality 出口。

## 十、故障判断

必须区分以下问题：

### JSON 格式错误

```text
xray run -test 报错
```

### 本地端口未监听

```text
127.0.0.1:21001 无法连接
```

### Reality 参数错误

```text
TLS handshake failed
connection reset
```

### 服务端 UUID 未注册

```text
invalid request user id
```

### 服务端已经接受请求

```text
received request
XtlsFilterTls found tls
tunneling request
```

这些日志表示 Reality 握手和服务器端转发已经成功，应继续检查目标网站、出口路由或本地客户端连接。

不要把所有连接失败都归因于转换器，必须分别检查：

1. JSON 是否正确生成。
2. Xray 是否成功启动。
3. 本地 SOCKS5 端口是否监听。
4. Reality 握手是否成功。
5. UUID 是否在服务端注册。
6. 服务端是否能连接目标地址。
7. ADS 是否正确使用 SOCKS5 域名解析。

