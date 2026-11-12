package sshclient

import (
	"errors"
	"net"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// tcpInfoV0 follows TCP_INFO_v0 in mstcpip.h. Version 0 is sufficient for RTT.
type tcpInfoV0 struct {
	State, Mss                       uint32
	ConnectionTimeMs                 uint64
	TimestampsEnabled                uint8
	RttUs, MinRttUs, BytesInFlight   uint32
	Cwnd, SndWnd, RcvWnd, RcvBuf     uint32
	BytesOut, BytesIn                uint64
	BytesReordered, BytesRetrans     uint32
	FastRetrans, DupAcksIn, Timeouts uint32
	SynRetrans                       uint8
}

func tcpRTT(connection *net.TCPConn) (time.Duration, error) {
	raw, err := connection.SyscallConn()
	if err != nil {
		return 0, err
	}
	const sioTCPInfo = windows.IOC_IN | windows.IOC_OUT | windows.IOC_VENDOR | 39
	var version, size uint32
	var info tcpInfoV0
	var socketErr error
	if err := raw.Control(func(fd uintptr) {
		socketErr = windows.WSAIoctl(windows.Handle(fd), sioTCPInfo,
			(*byte)(unsafe.Pointer(&version)), uint32(unsafe.Sizeof(version)),
			(*byte)(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), &size, nil, 0)
	}); err != nil {
		return 0, err
	}
	if socketErr != nil {
		return 0, socketErr
	}
	if uintptr(size) < unsafe.Offsetof(info.RttUs)+unsafe.Sizeof(info.RttUs) {
		return 0, errors.New("incomplete TCP RTT information")
	}
	return time.Duration(info.RttUs) * time.Microsecond, nil
}
