package sshclient

import (
	"net"
	"time"

	"golang.org/x/sys/unix"
)

func tcpRTT(connection *net.TCPConn) (time.Duration, error) {
	raw, err := connection.SyscallConn()
	if err != nil {
		return 0, err
	}
	var info *unix.TCPInfo
	var socketErr error
	if err := raw.Control(func(fd uintptr) {
		info, socketErr = unix.GetsockoptTCPInfo(int(fd), unix.IPPROTO_TCP, unix.TCP_INFO)
	}); err != nil {
		return 0, err
	}
	if socketErr != nil {
		return 0, socketErr
	}
	return time.Duration(info.Rtt) * time.Microsecond, nil
}
