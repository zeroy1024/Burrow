# WireGuard gateways

[English](wireguard.md) | [简体中文](wireguard.zh.md)

## Scope

WireGuard is an application-local gateway for Burrow terminals, SFTP, passive FTP/FTPS, connection tests, and MCP remote operations. Each host or site can select it directly, or follow an app default that uses it.

The implementation uses `wireguard-go` with gVisor netstack. `CreateNetTUN` creates an in-memory interface, not a macOS utun, Windows Wintun, or Linux TUN device. Burrow does not modify OS routes, default gateways, DNS settings, firewall rules, or NAT. It does not install a driver, invoke `wg-quick`, or execute configuration hooks. WireGuard's outer packets use ordinary UDP sockets and the host's existing routes and network permissions.

## Create a gateway

1. Open **Gateways**, add a gateway, and select **WireGuard**.
2. Import a client `.conf`, or enter the endpoint, tunnel addresses, peer public key, private key, and Allowed IPs manually. **Generate private key** also displays the corresponding client public key for configuration on the peer.
3. Configure tunnel DNS if targets are named by hostname, then save. Saving stores configuration; the tunnel starts only when used by a connection or test.
4. Select the gateway for a host/site or as the app default. Use the gateway's test for the WireGuard handshake, then the host/site test for the target service.

The first version supports one peer per gateway. It does not provide server configuration, automatic peer registration, address allocation, or management of another WireGuard application.

## Configuration and import

| Section | Supported fields |
|---|---|
| Interface | `PrivateKey`, `Address`, `DNS`, `MTU`, `ListenPort` |
| Peer | `PublicKey`, `PresharedKey`, `Endpoint`, `AllowedIPs`, `PersistentKeepalive` |

Tunnel addresses require CIDR prefix lengths. Address, DNS, and Allowed IPs lists accept comma-separated entries; repeated list fields are accepted on import. IPv4 and IPv6 are supported. Keys require canonical, nonzero 32-byte Base64 values. The peer key must differ from the client key.

The default MTU is 1420, with supported values from 1280 to 65535. Keepalive defaults to 0 (disabled), with values up to 65535 seconds. Local UDP port 0 uses an ephemeral port; a configured port remains subject to normal OS binding rules.

Imports are limited to 1 MiB and exactly one Interface/Peer pair. Unsupported settings, including `PreUp`, `PostUp`, `PreDown`, `PostDown`, `Table`, `SaveConfig`, and `FwMark`, are rejected with a line number. Their values are not included in the error. Burrow never executes imported text. The original file is not changed or copied into the database as raw configuration.

## Routes and DNS

| Route | Support |
|---|---|
| WireGuard → target | Supported |
| WireGuard → SSH and/or SOCKS5 → target | Supported |
| SSH/SOCKS5 → WireGuard → target | Not supported |
| Multiple WireGuard hops | Not supported |

WireGuard must be the first hop and connects through the host's existing network. Its endpoint hostname is resolved by the system resolver before the tunnel is established. The endpoint must be reachable independently of its own tunnel DNS.

Names dialed directly through WireGuard use only its configured DNS servers. These must be IP addresses covered by Allowed IPs; DNS search domains are not supported. Without tunnel DNS, use literal target IP addresses. Downstream SSH/SOCKS5 gateways continue to resolve their own target names normally.

Allowed IPs controls traffic inside this gateway; it does not install system routes. Target addresses outside those prefixes fail. Tunnel failures do not fall back to direct connections or system DNS for target names. FTP/FTPS through WireGuard uses passive mode for both control and data connections.

## Credentials, sharing, and recovery

Private keys and optional preshared keys are encrypted in Vault under an authenticated WireGuard-specific record. Public keys, endpoints, tunnel addresses, Allowed IPs, DNS, and other configuration metadata are stored in the ordinary catalog. Workspace, transfer, and MCP snapshots retain public configuration and credential revisions, never raw private keys or PSKs.

When editing a saved gateway, blank key fields retain existing keys. **Remove preshared key** explicitly clears the PSK. Changing gateway type or deleting an unreferenced gateway removes its obsolete credential record. Import source files remain the user's responsibility.

Matching settings for a client/peer identity share one tunnel across terminal, transfer, MCP, and gateway-test scopes. Closing one consumer leaves the others connected; the final lease closes the device and in-memory stack. Tests reuse this pool so they do not start a competing device with the same identity.

If the identity is active with different settings or credential revisions, new connections fail with an instruction to close related consumers before using the new settings. Existing consumers keep their captured configuration. Independent external clients should use distinct client identities: WireGuard learns a peer's latest authenticated endpoint, so simultaneous devices using the same identity can interfere with each other.

Workspace and transfer restoration remains offline. It does not start a tunnel, log in, or resume transfers automatically. Route/configuration or credential changes participate in MCP connection identity and require affected grants to be confirmed again.

## Tests and validation boundaries

The gateway test verifies an authenticated WireGuard handshake, reusing a handshake completed within the protocol's five-second initiation limit or requesting a new one with a 20-second timeout. It does not establish that an internal target service is reachable; use the host/site connection test for that.

An isolated loopback-only probe with `-race` verified real PSK authentication, IPv4/IPv6 TCP data, tunnel DNS, Allowed IPs rejection, close/reopen, and strict import rejection. It used an in-memory WireGuard peer, not an OS TUN or system routing configuration. An additional headless desktop overlay verified encrypted Vault storage, shared terminal/MCP/test leases, rejection of conflicting settings, and final-release cleanup. Build checks and native acceptance are recorded separately in [platform support](platform-support.md). Native GUI operation and Windows/Linux runtime acceptance have not been established by this probe.

Implementation: [wggateway](../internal/wggateway/), [shared runtime](../internal/desktop/gateway_wireguard.go), and [gateway form](../internal/desktop/gateway_wireguard_ui.go). Dependency licenses are available in About and the bundled [third-party notices](../THIRD_PARTY_NOTICES.md).
