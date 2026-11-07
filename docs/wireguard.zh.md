# WireGuard 网关

[English](wireguard.md) | [简体中文](wireguard.zh.md)

## 功能范围

WireGuard 为 Burrow 的终端、SFTP、被动模式 FTP/FTPS、连接测试及 MCP 远端操作提供应用内网关。主机或站点可直接选择它，也可跟随使用该网关的应用默认设置。

实现使用 `wireguard-go` 和 gVisor netstack。`CreateNetTUN` 创建内存接口，不创建 macOS utun、Windows Wintun 或 Linux TUN 设备。Burrow 不修改系统路由、默认网关、DNS 设置、防火墙规则或 NAT，不安装驱动，不调用 `wg-quick`，不执行配置中的钩子脚本。WireGuard 外层数据通过普通 UDP socket 收发，遵循系统现有路由和联网权限。

## 创建网关

1. 打开「网关」，新增网关并选择「WireGuard」。
2. 导入客户端 `.conf`，或手动填写 Peer 地址、隧道地址、Peer 公钥、私钥和 Allowed IPs。「生成私钥」会同时显示对应客户端公钥，供配置对端使用。
3. 目标使用域名时配置隧道 DNS，然后保存。保存只写入配置，连接或测试使用时才建立隧道。
4. 为主机/站点或应用默认出口选择该网关。先用网关测试验证 WG 握手，再用主机/站点测试验证目标服务。

首版每个网关支持一个 Peer，不提供服务端配置、自动注册 Peer、地址分配或外部 WireGuard 应用管理。

## 配置与导入

| 配置段 | 支持字段 |
|---|---|
| Interface | `PrivateKey`、`Address`、`DNS`、`MTU`、`ListenPort` |
| Peer | `PublicKey`、`PresharedKey`、`Endpoint`、`AllowedIPs`、`PersistentKeepalive` |

隧道地址需要 CIDR 前缀长度。Address、DNS 和 Allowed IPs 列表可用逗号分隔，导入时也支持重复填写列表字段。支持 IPv4 和 IPv6。密钥必须是规范编码、非零的 32 字节 Base64 值；Peer 公钥不能与客户端公钥相同。

MTU 默认为 1420，支持 1280 至 65535。保活间隔默认为 0（关闭），最大为 65535 秒。本地 UDP 端口为 0 时使用临时端口；指定端口仍遵循系统正常的绑定规则。

导入限制为 1 MiB，且恰好包含一个 Interface 和一个 Peer。不支持的设置，包括 `PreUp`、`PostUp`、`PreDown`、`PostDown`、`Table`、`SaveConfig` 和 `FwMark`，会按行号明确拒绝，错误不包含对应值。Burrow 不执行导入文本，不修改源文件，也不将原始配置全文复制进数据库。

## 路由与 DNS

| 路由 | 支持情况 |
|---|---|
| WireGuard → 目标 | 支持 |
| WireGuard → SSH 和/或 SOCKS5 → 目标 | 支持 |
| SSH/SOCKS5 → WireGuard → 目标 | 不支持 |
| 多个 WireGuard 跳点 | 不支持 |

WireGuard 必须位于第一跳，通过本机现有网络连接。其 Endpoint 域名在隧道建立前使用系统解析器解析，Endpoint 不能依赖该隧道自身的 DNS 才可到达。

直接经 WireGuard 拨号的域名仅使用其配置的 DNS 服务器。DNS 必须填写 Allowed IPs 覆盖的 IP 地址，不支持搜索域。未配置隧道 DNS 时应使用目标 IP 地址。后续 SSH/SOCKS5 网关仍按自身方式解析目标域名。

Allowed IPs 控制该网关内部的流量，不向系统安装路由。目标地址不在网段内时会失败；隧道失败不自动直连，目标域名也不回退到系统 DNS。FTP/FTPS 经 WireGuard 时使用被动模式，控制和数据连接均经过网关。

## 凭据、共享与恢复

私钥及可选预共享密钥使用独立、受认证的 WireGuard 凭据记录加密保存在 Vault。公钥、Endpoint、隧道地址、Allowed IPs、DNS 等配置元数据保存在普通目录记录中。工作区、传输和 MCP 快照只保留公开配置及凭据修订号，不包含原始私钥或 PSK。

编辑保存的网关时，密钥字段留空会保留原值；「移除预共享密钥」明确清除 PSK。改变网关类型或删除无引用网关时，会移除废弃的凭据记录。导入源文件仍由用户管理。

同一客户端/Peer 身份的相同配置在终端、传输、MCP 和网关测试之间共享一个隧道。关闭单个使用者不影响其他连接，最后一个引用释放时关闭设备和内存网络栈。测试复用该池，不会为同一身份额外启动相互竞争的设备。

同一身份已有不同配置或凭据修订号的活动隧道时，新连接会明确提示先关闭相关使用者，再使用新设置。原有使用者继续保留其配置快照。独立的外部客户端应使用不同客户端身份：WireGuard 会记录最近经过身份验证的对端地址，同一身份的多个设备并行运行可能相互干扰。

工作区及传输恢复仍保持离线，不自动建立隧道、登录或继续任务。路由配置及凭据变更参与 MCP 连接身份计算，受影响的授权需重新确认。

## 测试与验证边界

网关测试验证经过身份认证的 WireGuard 握手：复用协议发起限制内最近五秒完成的握手，或发起新的握手并等待最多 20 秒。它不证明内网目标服务可达，目标需使用主机/站点的连接测试验证。

隔离的本机回环探针已在 `-race` 下验证真实 PSK 认证、IPv4/IPv6 TCP 数据、隧道 DNS、Allowed IPs 拒绝、关闭后重建及严格导入拒绝。对端使用内存 WireGuard，不使用系统 TUN 或系统路由配置。额外的无界面 desktop overlay 已验证 Vault 加密存储、终端/MCP/测试共享引用、冲突配置拒绝及最后引用释放后的清理。构建检查与原生验收分别记录在[平台支持](platform-support.zh.md)中；该探针尚不能证明原生 GUI 操作及 Windows/Linux 运行验收通过。

实现入口：[wggateway](../internal/wggateway/)、[共享运行时](../internal/desktop/gateway_wireguard.go)和[网关表单](../internal/desktop/gateway_wireguard_ui.go)。依赖许可可在关于页及随包分发的[第三方声明](../THIRD_PARTY_NOTICES.md)中查看。
