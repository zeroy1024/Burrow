FROM ubuntu:24.04

RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends vsftpd \
    && useradd --create-home --shell /bin/bash burrow \
    && install -d -m 0755 /var/run/vsftpd/empty

COPY vsftpd.conf /etc/vsftpd.conf
COPY entrypoint.sh /usr/local/bin/ftp-integration

EXPOSE 21
ENTRYPOINT ["/bin/sh", "/usr/local/bin/ftp-integration"]
