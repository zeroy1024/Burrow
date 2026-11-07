#!/bin/sh
set -eu
cat /etc/vsftpd.conf > /run/ftp-integration.conf
printf 'pasv_min_port=%s\npasv_max_port=%s\n' "$BURROW_PASV_START" "$BURROW_PASV_END" >> /run/ftp-integration.conf
case "${BURROW_FTP_TLS:-none}" in
    none) ;;
    explicit|implicit)
        cat >> /run/ftp-integration.conf <<'TLS'
ssl_enable=YES
force_local_logins_ssl=YES
force_local_data_ssl=YES
require_ssl_reuse=YES
rsa_cert_file=/run/ftps-server.pem
rsa_private_key_file=/run/ftps-server.key
TLS
        if [ "$BURROW_FTP_TLS" = implicit ]; then
            printf 'implicit_ssl=YES\nlisten_port=990\n' >> /run/ftp-integration.conf
        fi
        ;;
    *) exit 1 ;;
esac
exec /usr/sbin/vsftpd /run/ftp-integration.conf
