package sshclient

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"slices"
)

const (
	socksVersion            = 5
	socksNoAuth             = 0
	socksNoAcceptedMethod   = 255
	socksConnect            = 1
	socksIPv4               = 1
	socksDomain             = 3
	socksIPv6               = 4
	socksGranted            = 0
	socksFailure            = 1
	socksUnsupportedCommand = 7
	socksUnsupportedAddress = 8
)

func readSOCKSTarget(stream io.ReadWriter) (string, error) {
	var greeting [2]byte
	if _, err := io.ReadFull(stream, greeting[:]); err != nil {
		return "", err
	}
	if greeting[0] != socksVersion || greeting[1] == 0 {
		return "", errors.New("invalid SOCKS5 greeting")
	}
	methods := make([]byte, int(greeting[1]))
	if _, err := io.ReadFull(stream, methods); err != nil {
		return "", err
	}
	method := byte(socksNoAcceptedMethod)
	if slices.Contains(methods, socksNoAuth) {
		method = socksNoAuth
	}
	if _, err := stream.Write([]byte{socksVersion, method}); err != nil {
		return "", err
	}
	if method != socksNoAuth {
		return "", errors.New("no compatible SOCKS5 authentication method")
	}

	var request [4]byte
	if _, err := io.ReadFull(stream, request[:]); err != nil {
		return "", err
	}
	if request[0] != socksVersion || request[2] != 0 {
		return "", errors.New("invalid SOCKS5 request")
	}
	if request[1] != socksConnect {
		writeSOCKSReply(stream, socksUnsupportedCommand)
		return "", errors.New("unsupported SOCKS5 command")
	}
	var length int
	switch request[3] {
	case socksIPv4:
		length = net.IPv4len
	case socksIPv6:
		length = net.IPv6len
	case socksDomain:
		var domainLength [1]byte
		if _, err := io.ReadFull(stream, domainLength[:]); err != nil {
			return "", err
		}
		length = int(domainLength[0])
	default:
		writeSOCKSReply(stream, socksUnsupportedAddress)
		return "", errors.New("unsupported SOCKS5 address type")
	}
	address := make([]byte, length+2)
	if _, err := io.ReadFull(stream, address); err != nil {
		return "", err
	}
	host := string(address[:length])
	if request[3] != socksDomain {
		host = net.IP(address[:length]).String()
	}
	target, err := ForwardTarget(host, int(binary.BigEndian.Uint16(address[length:])))
	if err != nil {
		writeSOCKSReply(stream, socksFailure)
	}
	return target, err
}

func writeSOCKSReply(w io.Writer, code byte) error {
	// The SSH direct-tcpip channel does not expose the far-end socket's bind address.
	_, err := w.Write([]byte{socksVersion, code, 0, socksIPv4, 0, 0, 0, 0, 0, 0})
	return err
}
