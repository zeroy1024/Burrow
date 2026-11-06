package desktop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/net/proxy"
)

// gatewayTestProgress is written by the test worker and read by the window:
// the hops of route before proven are connected, and current is the hop it
// connects. The route is fixed before the worker starts.
type gatewayTestProgress struct {
	route           []gatewayHop
	proven, current atomic.Int32
}

func (a *app) prepareGatewayTest(id string, progress *gatewayTestProgress) (func(context.Context) connectionTestResult, error) {
	route, err := a.captureGatewayRoute(host{session: sessionConfig{Gateway: id}})
	if err != nil {
		return nil, err
	}
	plan, err := a.captureGatewayPlan(route)
	if err != nil {
		return nil, err
	}
	gateway := route[len(route)-1]
	plan.route = route[:len(route)-1]
	// An in-flight shared transport reports to its original caller. A probe
	// owns its transports so progress always belongs to this test's route.
	plan.gatewayScope = "gateway-test:" + newSessionID()
	progress.route = route
	plan.hopProgress = func(index int, ready bool) {
		if ready {
			progress.proven.Store(int32(index + 1))
			progress.current.Store(int32(index + 1))
		} else {
			progress.current.Store(int32(index))
		}
	}
	return func(parent context.Context) connectionTestResult {
		ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
		defer cancel()
		releaseGateway := a.gatewayPool.retainRoute(route[len(route)-1:])
		defer releaseGateway()
		result := connectionTestResult{startedAt: time.Now()}
		dial, release, err := a.gatewayDialer(ctx, plan)
		if err == nil {
			defer release()
			progress.current.Store(int32(len(route) - 1))
			if gateway.Kind == "ssh" {
				step := plan.steps[len(plan.steps)-1]
				connection, connectErr := a.openSSHStep(ctx, step, plan.trust, dial, sshSecretsOverride{}, &result)
				err = connectErr
				if err == nil {
					result.Detail = "SSH 握手与认证成功"
					err = errors.Join(connection.Close(), connection.Wait())
				}
			} else {
				var auth *proxy.Auth
				auth, err = a.socksGatewayAuth(ctx, gateway)
				if err == nil {
					if dial == nil {
						dial = (&net.Dialer{Timeout: 20 * time.Second}).DialContext
					}
					address := net.JoinHostPort(strings.Trim(gateway.SOCKS.Address, "[]"), strconv.Itoa(gateway.SOCKS.Port))
					connection, connectErr := dial(ctx, "tcp", address)
					err = connectErr
					if err == nil {
						stop := closeOnCancel(ctx, connection)
						result.Detail, err = probeSOCKS5(connection, auth)
						stop()
						err = errors.Join(err, connection.Close())
					}
				}
			}
		}
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		if err == nil {
			progress.proven.Store(int32(len(route)))
		}
		result.Elapsed, result.Err = time.Since(result.startedAt), err
		result.Summary = fmt.Sprintf("连接成功 · %dms", result.Elapsed.Milliseconds())
		if err != nil {
			result.Summary = "网关连接失败"
			if errors.Is(err, context.Canceled) || errors.Is(err, errSSHConnectCanceled) {
				result.Summary = "测试已取消"
			} else if errors.Is(err, context.DeadlineExceeded) {
				result.Summary = "连接超时"
			}
			result.Detail = err.Error()
		}
		return result
	}, nil
}

func (a *app) gatewayTestSOCKSDialer(
	hop gatewayHop,
	upstream gatewayDialFunc,
	index int,
	progress func(int, bool),
) gatewayDialFunc {
	if upstream == nil {
		upstream = (&net.Dialer{Timeout: 20 * time.Second}).DialContext
	}
	forward := gatewayDialFunc(func(ctx context.Context, network, address string) (net.Conn, error) {
		connection, err := upstream(ctx, network, address)
		if err != nil {
			return nil, err
		}
		progress(index, false)
		return &gatewayTestSOCKSConn{Conn: connection, ready: func() { progress(index, true) }}, nil
	})
	dial := a.socksGatewayDialer(hop, forward)
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		progress(index, false)
		return dial(ctx, network, address)
	}
}

// Read only the method/auth replies. Once a proxy authenticates, failures in
// the subsequent relay request belong to reaching the next hop.
type gatewayTestSOCKSConn struct {
	net.Conn
	ready      func()
	reply      [2]byte
	read       int
	auth, done bool
}

func (c *gatewayTestSOCKSConn) Read(data []byte) (int, error) {
	n, err := c.Conn.Read(data)
	for _, value := range data[:n] {
		if c.done {
			break
		}
		c.reply[c.read] = value
		c.read++
		if c.read < len(c.reply) {
			continue
		}
		c.read = 0
		c.done = true
		if c.auth {
			if c.reply == [2]byte{1, 0} {
				c.ready()
			}
		} else if c.reply == [2]byte{5, 2} {
			c.auth, c.done = true, false
		} else if c.reply == [2]byte{5, 0} {
			c.ready()
		}
	}
	return n, err
}

// Probe only RFC 1928 method negotiation and RFC 1929 authentication. Relay
// requests belong to target connection tests and require a destination.
func probeSOCKS5(connection io.ReadWriter, auth *proxy.Auth) (string, error) {
	greeting := []byte{5, 1, 0}
	if auth != nil {
		greeting = []byte{5, 2, 0, 2}
	}
	if _, err := connection.Write(greeting); err != nil {
		return "", fmt.Errorf("SOCKS5 握手：%w", err)
	}
	var reply [2]byte
	if _, err := io.ReadFull(connection, reply[:]); err != nil {
		return "", fmt.Errorf("SOCKS5 握手：%w", err)
	}
	if reply[0] != 5 {
		return "", errors.New("服务器未返回有效的 SOCKS5 协议响应")
	}
	switch reply[1] {
	case 0:
		return "SOCKS5 握手成功，无需认证", nil
	case 2:
		if auth == nil {
			return "", errors.New("SOCKS5 服务器要求用户名密码认证，请配置认证信息")
		}
	case 255:
		return "", errors.New("SOCKS5 服务器不接受当前认证方式")
	default:
		return "", fmt.Errorf("SOCKS5 服务器选择了不支持的认证方式：%d", reply[1])
	}
	request := []byte{1, byte(len(auth.User))}
	request = append(request, auth.User...)
	request = append(request, byte(len(auth.Password)))
	request = append(request, auth.Password...)
	defer clear(request)
	if _, err := connection.Write(request); err != nil {
		return "", fmt.Errorf("SOCKS5 认证：%w", err)
	}
	if _, err := io.ReadFull(connection, reply[:]); err != nil {
		return "", fmt.Errorf("SOCKS5 认证：%w", err)
	}
	if reply[0] != 1 {
		return "", errors.New("SOCKS5 服务器返回了无效的认证响应")
	}
	if reply[1] != 0 {
		return "", errors.New("SOCKS5 用户名密码认证失败")
	}
	return "SOCKS5 握手与认证成功", nil
}
