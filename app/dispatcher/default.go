// +build !confonly

package dispatcher

//go:generate errorgen

import (
	"context"
	"strings"
	"sync"
	"time"

	"v2ray.com/core"
	"v2ray.com/core/common"
	"v2ray.com/core/common/buf"
	"v2ray.com/core/common/log"
	"v2ray.com/core/common/net"
	"v2ray.com/core/common/protocol"
	"v2ray.com/core/common/session"
	"v2ray.com/core/features/outbound"
	"v2ray.com/core/features/policy"
	"v2ray.com/core/features/routing"
	"v2ray.com/core/features/stats"
	"v2ray.com/core/transport"
	"v2ray.com/core/transport/pipe"

	tstats "github.com/eycorsican/go-tun2socks/common/stats"
	tsession "github.com/eycorsican/go-tun2socks/common/stats/session"
)

var (
	errSniffingTimeout = newError("timeout on sniffing")
)

type cachedReader struct {
	sync.Mutex
	reader *pipe.Reader
	cache  buf.MultiBuffer
}

func (r *cachedReader) Cache(b *buf.Buffer, timeout time.Duration) {
	mb, _ := r.reader.ReadMultiBufferTimeout(timeout)
	r.Lock()
	if !mb.IsEmpty() {
		r.cache, _ = buf.MergeMulti(r.cache, mb)
	}
	b.Clear()
	rawBytes := b.Extend(buf.Size)
	n := r.cache.Copy(rawBytes)
	b.Resize(0, int32(n))
	r.Unlock()
}

func (r *cachedReader) readInternal() buf.MultiBuffer {
	r.Lock()
	defer r.Unlock()

	if r.cache != nil && !r.cache.IsEmpty() {
		mb := r.cache
		r.cache = nil
		return mb
	}

	return nil
}

func (r *cachedReader) ReadMultiBuffer() (buf.MultiBuffer, error) {
	mb := r.readInternal()
	if mb != nil {
		return mb, nil
	}

	return r.reader.ReadMultiBuffer()
}

func (r *cachedReader) ReadMultiBufferTimeout(timeout time.Duration) (buf.MultiBuffer, error) {
	mb := r.readInternal()
	if mb != nil {
		return mb, nil
	}

	return r.reader.ReadMultiBufferTimeout(timeout)
}

func (r *cachedReader) ReadPacket() (*buf.Buffer, *net.UDPAddr, error) {
	return nil, nil, nil
}

func (r *cachedReader) Interrupt() {
	r.Lock()
	if r.cache != nil {
		r.cache = buf.ReleaseMulti(r.cache)
	}
	r.Unlock()
	r.reader.Interrupt()
}

// DefaultDispatcher is a default implementation of Dispatcher.
type DefaultDispatcher struct {
	ohm    outbound.Manager
	router routing.Router
	policy policy.Manager
	stats  stats.Manager
	stater tstats.SessionStater
}

func init() {
	common.Must(common.RegisterConfig((*Config)(nil), func(ctx context.Context, config interface{}) (interface{}, error) {
		d := new(DefaultDispatcher)
		if err := core.RequireFeatures(ctx, func(om outbound.Manager, router routing.Router, pm policy.Manager, sm stats.Manager) error {
			return d.Init(config.(*Config), om, router, pm, sm)
		}); err != nil {
			return nil, err
		}
		return d, nil
	}))
}

// Init initializes DefaultDispatcher.
func (d *DefaultDispatcher) Init(config *Config, om outbound.Manager, router routing.Router, pm policy.Manager, sm stats.Manager) error {
	d.ohm = om
	d.router = router
	d.policy = pm
	d.stats = sm
	d.stater = tsession.NewSimpleSessionStater()
	return nil
}

// Type implements common.HasType.
func (*DefaultDispatcher) Type() interface{} {
	return routing.DispatcherType()
}

// Start implements common.Runnable.
func (d *DefaultDispatcher) Start() error {
	d.stater.Start()
	return nil
}

// Close implements common.Closable.
func (d *DefaultDispatcher) Close() error {
	d.stater.Stop()
	return nil
}

func (d *DefaultDispatcher) getLink(ctx context.Context, destination net.Destination) (*transport.Link, *transport.Link, *tstats.Session) {
	var uplinkReader, downlinkReader *pipe.Reader
	var uplinkWriter, downlinkWriter *pipe.Writer

	opt := pipe.OptionsFromContext(ctx)

	if destination.Network == net.Network_UDP {
		uplinkReader, uplinkWriter = pipe.New(append(opt, pipe.DiscardOverflow())...)
		downlinkReader, downlinkWriter = pipe.New(opt...)
	} else {
		uplinkReader, uplinkWriter = pipe.New(append(opt, pipe.WithSizeLimit(32*1024))...)
		downlinkReader, downlinkWriter = pipe.New(opt...)
	}

	inboundLink := &transport.Link{
		Reader: downlinkReader,
		Writer: uplinkWriter,
	}

	outboundLink := &transport.Link{
		Reader: uplinkReader,
		Writer: downlinkWriter,
	}

	sessionInbound := session.InboundFromContext(ctx)
	var user *protocol.MemoryUser
	if sessionInbound != nil {
		user = sessionInbound.User
	}

	var sess *tstats.Session
	content := session.ContentFromContext(ctx)
	if content != nil {
		sess = &tstats.Session{
			0,
			0,
			content.Application,
			content.Network,
			content.LocalAddr,
			content.RemoteAddr,
			time.Now(),
			time.Time{},
			content.Extra,
			"",
			false,
			time.Time{},
			"",
		}
		d.stater.AddSession(outboundLink, sess)
		inboundLink.Writer = &InboundSizeWriter{
			Session: sess,
			Writer:  inboundLink.Writer,
		}
		outboundLink.Writer = &OutboundSizeWriter{
			Session: sess,
			Writer:  outboundLink.Writer,
		}
	}

	if user != nil && len(user.Email) > 0 {
		p := d.policy.ForLevel(user.Level)
		if p.Stats.UserUplink {
			name := "user>>>" + user.Email + ">>>traffic>>>uplink"
			if c, _ := stats.GetOrRegisterCounter(d.stats, name); c != nil {
				inboundLink.Writer = &SizeStatWriter{
					Counter: c,
					Writer:  inboundLink.Writer,
				}
			}
		}
		if p.Stats.UserDownlink {
			name := "user>>>" + user.Email + ">>>traffic>>>downlink"
			if c, _ := stats.GetOrRegisterCounter(d.stats, name); c != nil {
				outboundLink.Writer = &SizeStatWriter{
					Counter: c,
					Writer:  outboundLink.Writer,
				}
			}
		}
	}

	return inboundLink, outboundLink, sess
}

func shouldOverride(result SniffResult, domainOverride []string) bool {
	for _, p := range domainOverride {
		if strings.HasPrefix(result.Protocol(), p) {
			return true
		}
	}
	return false
}

// Dispatch implements routing.Dispatcher.
func (d *DefaultDispatcher) Dispatch(ctx context.Context, destination net.Destination) (*transport.Link, error) {
	if !destination.IsValid() {
		panic("Dispatcher: Invalid destination.")
	}

	ob := session.OutboundFromContext(ctx)
	if ob == nil {
		ob = new(session.Outbound)
		ctx = session.ContextWithOutbound(ctx, ob)
	}
	ob.Target = destination

	inbound, outbound, sess := d.getLink(ctx, destination)
	ctx = session.ContextWithProxySession(ctx, sess)
	content := session.ContentFromContext(ctx)
	if content == nil {
		content = new(session.Content)
		ctx = session.ContextWithContent(ctx, content)
	}
	sniffingRequest := content.SniffingRequest
	if destination.Network != net.Network_TCP || !sniffingRequest.Enabled {
		go d.routedDispatch(ctx, outbound, destination)
	} else {
		go func() {
			cReader := &cachedReader{
				reader: outbound.Reader.(*pipe.Reader),
			}
			outbound.Reader = cReader
			result, err := sniffer(ctx, cReader)
			if err == nil {
				content.Protocol = result.Protocol()
			}
			if err == nil && shouldOverride(result, sniffingRequest.OverrideDestinationForProtocol) {
				domain := result.Domain()
				newError("sniffed domain: ", domain).WriteToLog(session.ExportIDToError(ctx))
				destination.Address = net.ParseAddress(domain)
				ob.Target = destination
				if record := session.ProxyRecordFromContext(ctx); record != nil {
					record.Target = destination.String()
				}
				if sess != nil {
					sess.RemoteAddr = destination.NetAddr()
				}
			}
			d.routedDispatch(ctx, outbound, destination)
		}()
	}
	return inbound, nil
}

func sniffer(ctx context.Context, cReader *cachedReader) (SniffResult, error) {
	payload := buf.New()
	defer payload.Release()

	sniffer := NewSniffer()
	totalAttempt := 0
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			totalAttempt++
			if totalAttempt > 2 {
				return nil, errSniffingTimeout
			}
			timeout := time.Millisecond * 300
			if totalAttempt > 1 {
				timeout = time.Millisecond * 5
			}
			cReader.Cache(payload, timeout)
			if !payload.IsEmpty() {
				result, err := sniffer.Sniff(payload.Bytes())
				if err != common.ErrNoClue {
					return result, err
				}
			}
			if payload.IsFull() {
				return nil, errUnknownContent
			}
		}
	}
}

func (d *DefaultDispatcher) routedDispatch(ctx context.Context, link *transport.Link, destination net.Destination) {
	var handler outbound.Handler
	if d.router != nil {
		if tag, err := d.router.PickRoute(ctx); err == nil {
			if h := d.ohm.GetHandler(tag); h != nil {
				newError("taking detour [", tag, "] for [", destination, "]").WriteToLog(session.ExportIDToError(ctx))
				handler = h
			} else {
				newError("non existing tag: ", tag).AtWarning().WriteToLog(session.ExportIDToError(ctx))
			}
		} else {
			newError("default route for ", destination).WriteToLog(session.ExportIDToError(ctx))
		}
	}

	if handler == nil {
		handler = d.ohm.GetDefaultHandler()
	}

	if handler == nil {
		newError("default outbound handler not exist").WriteToLog(session.ExportIDToError(ctx))
		common.Close(link.Writer)
		common.Interrupt(link.Reader)
		return
	}

	if record := session.ProxyRecordFromContext(ctx); record != nil {
		record.Tag = handler.Tag()
	}
	if sess := session.ProxySessionFromContext(ctx); sess != nil {
		sess.OutboundTag = handler.Tag()
	}

	accessMessage := log.AccessMessageFromContext(ctx)
	if accessMessage != nil {
		accessMessage.OutboundTag = handler.Tag()
		log.Record(accessMessage)
	}

	handler.Dispatch(ctx, link)

	d.stater.RemoveSession(link)
}
