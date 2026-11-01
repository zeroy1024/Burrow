#!/bin/sh
set -eu
cat /etc/vsftpd.conf > /run/ftp-integration.conf
printf 'pasv_min_port=%s\npasv_max_port=%s\n' "$SSHDESK_PASV_START" "$SSHDESK_PASV_END" >> /run/ftp-integration.conf
exec /usr/sbin/vsftpd /run/ftp-integration.conf
