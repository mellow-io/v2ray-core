// +build !confonly

package measure

//go:generate errorgen

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"time"

	"v2ray.com/core/common"
	"v2ray.com/core/common/net"
	"v2ray.com/core/common/session"
	"v2ray.com/core/features/outbound"
	"v2ray.com/core/transport"
	"v2ray.com/core/transport/pipe"
)

func MeasureLatency(handler outbound.Handler, target, content string, timeout time.Duration) time.Duration {
	var isTls bool = false
	tgtParts := strings.Split(target, ":")
	if len(tgtParts) != 3 {
		panic("invalid target in latency balancer")
	}
	proto := tgtParts[0]
	host := tgtParts[1]
	port := tgtParts[2]
	if proto == "tls" {
		isTls = true
		proto = "tcp"
	}

	uplinkReader, uplinkWriter := pipe.New(pipe.WithoutSizeLimit())
	downlinkReader, downlinkWriter := pipe.New(pipe.WithoutSizeLimit())
	inbound := net.NewConnection(
		net.ConnectionInputMulti(uplinkWriter),
		net.ConnectionOutputMulti(downlinkReader),
	)
	if isTls {
		inbound = tls.Client(inbound, &tls.Config{ServerName: host})
	}
	outbound := &transport.Link{
		Reader: uplinkReader,
		Writer: downlinkWriter,
	}
	defer func() {
		common.Close(downlinkWriter)
		common.Interrupt(uplinkReader)
		inbound.Close()
	}()

	destination, err := net.ParseDestination(fmt.Sprintf("%s:%s:%s", proto, host, port))
	if err != nil {
		panic(err)
	}

	ctx := session.ContextWithOutbound(context.Background(), &session.Outbound{Target: destination})

	go handler.Dispatch(ctx, outbound)

	errCh := make(chan struct{}, 1)
	done := make(chan struct{}, 1)
	start := time.Now()

	go readResponse(inbound, errCh, done)
	go writeRequest(inbound, content, errCh)

	select {
	case <-done:
	case <-errCh:
		return timeout
	case <-time.After(timeout):
		return timeout
	}

	elasped := time.Since(start)

	return elasped
}

func writeRequest(conn net.Conn, content string, errCh chan struct{}) {
	_, err := conn.Write([]byte(content))
	if err != nil {
		newError("failed to send probe content").Base(err).WriteToLog()
		select {
		case errCh <- struct{}{}:
		default:
		}
		return
	}
}

func readResponse(conn net.Conn, errCh, done chan struct{}) {
	buf := make([]byte, 1)
	n, err := conn.Read(buf)
	if err != nil {
		newError("failed to read probe response").Base(err).WriteToLog()
		select {
		case errCh <- struct{}{}:
		default:
		}
		return
	}
	if n == 0 {
		newError("failed to read probe response: empty response").WriteToLog()
		select {
		case errCh <- struct{}{}:
		default:
		}
		return
	}
	done <- struct{}{}
}
