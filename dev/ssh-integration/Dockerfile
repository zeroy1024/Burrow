FROM ubuntu:24.04

RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
       openssh-server vim htop procps \
    && useradd --create-home --shell /bin/bash burrow \
    && install -d -m 0755 /run/sshd \
    && install -d -o burrow -g burrow -m 0700 /home/burrow/.ssh \
    && ssh-keygen -A

COPY sshd_config /etc/ssh/sshd_config

EXPOSE 22
CMD ["/usr/sbin/sshd", "-D", "-e"]
