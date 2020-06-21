package udp

import (
	"context"
	"io"
	"sync"
	"time"

	"v2ray.com/core/common/signal/done"

	"v2ray.com/core/common"
	"v2ray.com/core/common/buf"
	"v2ray.com/core/common/net"
	"v2ray.com/core/common/protocol/udp"
	"v2ray.com/core/common/session"
	"v2ray.com/core/common/signal"
	"v2ray.com/core/features/routing"
	"v2ray.com/core/transport"
)

type ResponseCallback func(ctx context.Context, packet *udp.Packet)

type connEntry struct {
	link   *transport.Link
	timer  signal.ActivityUpdater
	cancel context.CancelFunc
}

type Dispatcher struct {
	sync.RWMutex
	conns      map[net.Destination]*connEntry
	dispatcher routing.Dispatcher
	callback   ResponseCallback
}

func NewDispatcher(dispatcher routing.Dispatcher, callback ResponseCallback) *Dispatcher {
	return &Dispatcher{
		conns:      make(map[net.Destination]*connEntry),
		dispatcher: dispatcher,
		callback:   callback,
	}
}

func (v *Dispatcher) RemoveRay(src net.Destination) {
	v.Lock()
	defer v.Unlock()
	if conn, found := v.conns[src]; found {
		common.Close(conn.link.Reader)
		common.Close(conn.link.Writer)
		delete(v.conns, src)
	}
}

func (v *Dispatcher) getInboundRay(ctx context.Context, nosrc bool, src, dest net.Destination) *connEntry {
	v.Lock()
	defer v.Unlock()

	if !nosrc {
		if entry, found := v.conns[src]; found {
			return entry
		}

		newError("establishing new connection from ", src, " to ", dest).WriteToLog()
	} else {
		newError("establishing new connection for ", dest).WriteToLog()
	}

	ctx, cancel := context.WithCancel(ctx)
	removeRay := func() {
		cancel()
		v.RemoveRay(src)
	}
	var timer signal.ActivityUpdater
	if outbound := session.OutboundFromContext(ctx); outbound != nil && outbound.Timeout > 0 {
		timer = signal.CancelAfterInactivity(ctx, removeRay, outbound.Timeout)
	} else {
		timer = signal.CancelAfterInactivity(ctx, removeRay, time.Minute*2)
	}
	link, _ := v.dispatcher.Dispatch(ctx, dest)
	entry := &connEntry{
		link:   link,
		timer:  timer,
		cancel: removeRay,
	}
	v.conns[src] = entry
	go handleInput(ctx, entry, v.callback, dest)
	return entry
}

func (v *Dispatcher) Dispatch(ctx context.Context, destination net.Destination, payload *buf.Buffer) {
	// TODO: Add user to destString
	// newError("dispatch request to: ", destination).AtDebug().WriteToLog(session.ExportIDToError(ctx))

	inbound := session.InboundFromContext(ctx)
	if inbound == nil {
		newError("inbound not found").WriteToLog(session.ExportIDToError(ctx))
		return
	}

	content := session.ContentFromContext(ctx)
	if content != nil {
		content.RemoteAddr = destination.NetAddr()
	}

	conn := v.getInboundRay(ctx, inbound.NoSource, inbound.Source, destination)
	outputStream := conn.link.Writer
	if outputStream != nil {
		if err := outputStream.WritePacket(payload, destination.UDPAddr()); err != nil {
			newError("failed to write first UDP payload").Base(err).WriteToLog(session.ExportIDToError(ctx))
			conn.cancel()
			return
		}
	}
}

func handleInput(ctx context.Context, conn *connEntry, callback ResponseCallback, origDest net.Destination) {
	defer conn.cancel()

	input := conn.link.Reader
	timer := conn.timer

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		b, dest, err := input.ReadPacket()
		if err != nil {
			newError("failed to handle UDP input").Base(err).WriteToLog(session.ExportIDToError(ctx))
			return
		}
		timer.Update()
		if dest == nil {
			dest = origDest.UDPAddr()
		}
		callback(ctx, &udp.Packet{
			Payload: b,
			Source:  net.DestinationFromAddr(dest),
		})
	}
}

type dispatcherConn struct {
	dispatcher *Dispatcher
	cache      chan *udp.Packet
	done       *done.Instance
	ctx        context.Context
}

func DialDispatcher(ctx context.Context, dispatcher routing.Dispatcher) (net.PacketConn, error) {
	c := &dispatcherConn{
		cache: make(chan *udp.Packet, 16),
		done:  done.New(),
		ctx:   ctx,
	}

	d := NewDispatcher(dispatcher, c.callback)
	c.dispatcher = d
	return c, nil
}

func (c *dispatcherConn) callback(ctx context.Context, packet *udp.Packet) {
	select {
	case <-c.done.Wait():
		packet.Payload.Release()
		return
	case c.cache <- packet:
	default:
		packet.Payload.Release()
		return
	}
}

func (c *dispatcherConn) ReadFrom(p []byte) (int, net.Addr, error) {
	select {
	case <-c.done.Wait():
		return 0, nil, io.EOF
	case packet := <-c.cache:
		n := copy(p, packet.Payload.Bytes())
		return n, &net.UDPAddr{
			IP:   packet.Source.Address.IP(),
			Port: int(packet.Source.Port),
		}, nil
	}
}

func (c *dispatcherConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	buffer := buf.New()
	raw := buffer.Extend(buf.Size)
	n := copy(raw, p)
	buffer.Resize(0, int32(n))

	c.dispatcher.Dispatch(c.ctx, net.DestinationFromAddr(addr), buffer)
	return n, nil
}

func (c *dispatcherConn) Close() error {
	return c.done.Close()
}

func (c *dispatcherConn) LocalAddr() net.Addr {
	return &net.UDPAddr{
		IP:   []byte{0, 0, 0, 0},
		Port: 0,
	}
}

func (c *dispatcherConn) SetDeadline(t time.Time) error {
	return nil
}

func (c *dispatcherConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *dispatcherConn) SetWriteDeadline(t time.Time) error {
	return nil
}
