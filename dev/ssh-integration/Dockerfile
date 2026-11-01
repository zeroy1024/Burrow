FROM ubuntu:24.04

RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
       openssh-server vim htop procps \
    && useradd --create-home --shell /bin/bash sshdesk \
    && install -d -m 0755 /run/sshd \
    && install -d -o sshdesk -g sshdesk -m 0700 /home/sshdesk/.ssh \
    && ssh-keygen -A

COPY sshd_config /etc/ssh/sshd_config

EXPOSE 22
CMD ["/usr/sbin/sshd", "-D", "-e"]
