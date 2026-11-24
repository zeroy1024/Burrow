package sshclient_test

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/zeroy1024/Burrow/internal/sshclient"
	"golang.org/x/crypto/ssh"
)

func TestForwardListener(t *testing.T) {
	const hostname, port = "service.test", 8080
	config := channelServer(t, func(_ context.Context, request ssh.NewChannel, _ int) {
		var target struct {
			Host       string
			Port       uint32
			Origin     string
			OriginPort uint32
		}
		if request.ChannelType() != "direct-tcpip" || ssh.Unmarshal(request.ExtraData(), &target) != nil || target.Host != hostname || target.Port != port {
			t.Error("forwarded channel did not carry the expected destination")
			request.Reject(ssh.ConnectionFailed, "unexpected destination")
			return
		}
		channel, requests, err := request.Accept()
		if err != nil {
			t.Error(err)
			return
		}
		defer channel.Close()
		go ssh.DiscardRequests(requests)
		// Waiting for EOF before answering exposes relays that drop the return stream on a half-close.
		payload, err := io.ReadAll(channel)
		if err != nil {
			t.Error(err)
			return
		}
		if _, err := channel.Write(append([]byte("from fixture: "), payload...)); err != nil {
			t.Error(err)
		}
	})
	connection, err := sshclient.Dial(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { connection.Close(); waitConnection(t, connection) })
	release, err := connection.Retain()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	for _, kind := range []string{"fixed destination", "socks5 hostname"} {
		t.Run(kind, func(t *testing.T) {
			target := ""
			if kind == "fixed destination" {
				target, err = sshclient.ForwardTarget(hostname, port)
				if err != nil {
					t.Fatal(err)
				}
			}
			reservation, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
			if err != nil {
				t.Fatal(err)
			}
			local := reservation.Addr().(*net.TCPAddr)
			reservation.Close()
			forward, err := connection.ListenForward(t.Context(), local.AddrPort(), target, func(err error) { t.Error(err) })
			if err != nil {
				t.Fatal(err)
			}
			defer func() { forward.Close(); forward.Wait() }()
			client, err := net.DialTCP("tcp4", nil, local)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			if err := client.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
				t.Fatal(err)
			}
			if target == "" {
				if _, err := client.Write([]byte{5, 1, 0}); err != nil {
					t.Fatal(err)
				}
				var method [2]byte
				if _, err := io.ReadFull(client, method[:]); err != nil || method != [2]byte{5, 0} {
					t.Fatalf("SOCKS5 method: %v, %v", method, err)
				}
				request := append([]byte{5, 1, 0, 3, byte(len(hostname))}, hostname...)
				request = append(request, port>>8, port&255)
				if _, err := client.Write(request); err != nil {
					t.Fatal(err)
				}
				var reply [10]byte
				if _, err := io.ReadFull(client, reply[:]); err != nil || reply[0] != 5 || reply[1] != 0 {
					t.Fatalf("SOCKS5 reply: %v, %v", reply, err)
				}
			}
			if _, err := client.Write([]byte("hello")); err != nil {
				t.Fatal(err)
			}
			if err := client.CloseWrite(); err != nil {
				t.Fatal(err)
			}
			response, err := io.ReadAll(client)
			if err != nil || !bytes.Equal(response, []byte("from fixture: hello")) {
				t.Fatalf("forwarded response: %q, %v", response, err)
			}
			forward.Close()
			if err := forward.Wait(); err != nil {
				t.Fatal(err)
			}
			if unexpected, err := net.DialTCP("tcp4", nil, local); err == nil {
				unexpected.Close()
				t.Fatal("closed rule still accepts connections")
			}
		})
	}
}
