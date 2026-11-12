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
	var info *unix.TCPConnectionInfo
	var socketErr error
	if err := raw.Control(func(fd uintptr) {
		info, socketErr = unix.GetsockoptTCPConnectionInfo(int(fd), unix.IPPROTO_TCP, unix.TCP_CONNECTION_INFO)
	}); err != nil {
		return 0, err
	}
	if socketErr != nil {
		return 0, socketErr
	}
	// Darwin reports the smoothed RTT in milliseconds, unlike Linux.
	return time.Duration(info.Srtt) * time.Millisecond, nil
}
