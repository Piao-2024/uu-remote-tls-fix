# macOS 使用说明

macOS 版通过 UU 自带 SOCKS5 代理设置接入本机 TLS 分片服务。流量发往官方
服务器，不需要代理订阅、证书安装、修改 hosts 或系统 DNS。

## 支持范围

- macOS 12 及以上，Apple Silicon（arm64）和 Intel（amd64）。
- 本机实际验证环境：Apple Silicon、UU 远程 4.42.0；Mac 作为主控连接其他电脑。
- 支持 UU 官方域名的 TCP 443 流量：`*.nrd.nie.163.com`、`uuyc.163.com`、
  `uuyc.webapp.163.com`、`nrd-ugc.fp.ps.netease.com`、`fp.ps.netease.com`。
- 只提供 SOCKS5 CONNECT，要求客户端传入域名（远程 DNS）；不代理 UDP、任意
  IP、任意网站、80 端口，也不提供 Windows 版的 LAN DNS / 443 透明接入。
- 尚未验证 Mac 作为被控端、登录前无人值守、文件传输等所有功能。服务在当前
  用户登录后启动，不能替代系统级 daemon。

## 安装

### 从源码安装

安装 [Go 1.25 或更新版本](https://go.dev/dl/)，下载或克隆仓库后执行：

```bash
bash install-macos.sh
```

脚本在本机编译，不下载第三方 Go 依赖。**使用普通登录用户运行，不要加 sudo**。
默认监听 `127.0.0.1:17899`，并注册当前用户的 LaunchAgent。

### 使用预编译程序

仓库的 macOS CI 会构建 arm64 和 amd64 程序；打 `macos-v*` 标签后生成包含
安装脚本、程序和说明的 Release 草稿。正式发布之前，请从源码构建。

下载对应架构的发布包并解压，在解压目录执行：

```bash
# SHA256SUMS 与发布包一起下载；可在下载目录用 shasum -a 256 -c SHA256SUMS 校验。
bash install-macos.sh --binary ./uu-tls-fragmenter
```

此方式无需安装 Go。二进制未做 Apple Developer ID 签名或公证，系统可能要求
通过“系统设置 → 隐私与安全性”确认允许运行。

## 设置 UU 远程

打开 **设置 → 网络 → 代理设置 → 手动设置**：

| 项目 | 值 |
| --- | --- |
| 协议 | SOCKS5 |
| 地址 | `127.0.0.1` |
| 端口 | `17899` |
| 用户名、密码 | 留空 |

点击检测并启用，再测试连接远程电脑。安装脚本不读取或改写 UU 的私有配置；
保留原代理参数，方便以后手动恢复。不要将这个代理设为系统代理或浏览器代理。

如果端口已被其他程序使用，可改用：

```bash
bash install-macos.sh --port 17901
bash test-macos.sh --port 17901
```

同时将 UU 中的端口改为 `17901`。安装脚本会拒绝替换占用该端口的其他程序。

## 检测与日志

```bash
bash test-macos.sh
tail -f "$HOME/Library/Logs/UURemoteTlsFix/service.log"
launchctl print "gui/$(id -u)/com.uu-remote-tls-fix.fragmenter"
```

检测脚本忽略环境中的 HTTP/SOCKS 代理，并明确通过本机服务访问六个官方端点。
根路径返回 404 是正常的；检测成功表示服务器返回了经过证书校验的 HTTPS
响应，不代表远程桌面、信令协议或媒体传输都已经验证。

首次在这台 Mac 上验证时，普通 API TLS 握手超时，分片后握手恢复；六端点
HTTPS 检测通过，用户随后确认 Mac 主控可以连接其他电脑。

服务默认按系统路由出站，不代表强制绕过 VPN/TUN。需要指定物理网卡时，先用
`networksetup -listallhardwareports` 确认接口，再重新安装：

```bash
bash install-macos.sh --interface en0
```

指定接口会同时约束 DNS-over-HTTPS 和服务器 IPv4 连接。接口断开后连接会失败，
不会自动改走其他接口；重新安装时省略 `--interface` 可恢复系统路由。

若日志显示域名/IP/端口不受支持，说明客户端的请求超出了上述支持范围。
若 TLS 仍失败，记录失败域名、UU 版本及网络环境；官方节点和网络策略可能变化。
程序不会关闭服务器证书验证。

## 停止与卸载

**先在 UU 中停用代理，或恢复你之前的代理设置**，然后执行：

```bash
bash uninstall-macos.sh
```

卸载会停止 LaunchAgent，删除本工具的二进制、plist 和服务日志；保留目录中的
其他文件，不修改 UU 或系统配置。以后需要再次使用时重新安装即可。

安装位置：

```text
~/Library/Application Support/UURemoteTlsFix/uu-tls-fragmenter
~/Library/LaunchAgents/com.uu-remote-tls-fix.fragmenter.plist
~/Library/Logs/UURemoteTlsFix/service.log
```

高级用途：可通过 `UU_TLS_FIX_HOME=/absolute/path` 将上述 `Library` 文件写入
指定根目录；安装和卸载必须使用相同的值。LaunchAgent 仍属于当前登录用户。

## 开发验证

```bash
cd macos
go test -race ./...
go vet ./...
mkdir -p ../dist
go build -o ../dist/uu-tls-fragmenter-darwin-arm64 .
cd ..
bash tests/macos-scripts.sh
```

脚本测试在临时目录检查安装/重复安装/卸载、特殊字符路径和端口冲突，不更改
真实的 UU 配置或注册真实 LaunchAgent。测试文件中的假 launchctl 仅替代系统
服务注册；TLS 与 SOCKS 测试使用真实协议数据。
