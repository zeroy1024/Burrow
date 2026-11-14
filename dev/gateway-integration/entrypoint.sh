#!/bin/sh
# Disposable gateway fixture: SOCKS5 with password authentication, or a
# WireGuard peer that forwards to the Docker bridge. Secrets arrive as files
# copied into /run before start, never through the environment or image.
set -eu

case "$BURROW_GATEWAY" in
socks)
    exec microsocks -i 0.0.0.0 -p 1080 -u burrow -P "$(cat /run/socks-password)"
    ;;
wireguard)
    # Prefer the kernel module; fall back to the userspace implementation.
    if ! ip link add wg0 type wireguard 2>/dev/null; then
        mkdir -p /dev/net
        [ -c /dev/net/tun ] || mknod /dev/net/tun c 10 200
        wireguard-go wg0
    fi
    wg setconf wg0 /run/wg0.conf
    ip address add "$BURROW_WG_ADDRESS" dev wg0
    ip link set wg0 up
    iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE
    exec sleep infinity
    ;;
*)
    echo "BURROW_GATEWAY must be socks or wireguard" >&2
    exit 2
    ;;
esac
