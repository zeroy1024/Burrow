FROM ubuntu:24.04

RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
       microsocks wireguard-go wireguard-tools iproute2 iptables \
    && rm -rf /var/lib/apt/lists/*

COPY entrypoint.sh /usr/local/bin/gateway-entrypoint
RUN chmod 0755 /usr/local/bin/gateway-entrypoint

ENTRYPOINT ["/usr/local/bin/gateway-entrypoint"]
