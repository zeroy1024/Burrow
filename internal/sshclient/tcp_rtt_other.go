//go:build !darwin && !linux && !windows

package sshclient

import (
	"errors"
	"net"
	"time"
)

func tcpRTT(*net.TCPConn) (time.Duration, error) {
	return 0, errors.ErrUnsupported
}
