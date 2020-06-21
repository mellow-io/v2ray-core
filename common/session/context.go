package session

import (
	"context"

	"github.com/eycorsican/go-tun2socks/common/stats"
)

type sessionKey int

const (
	idSessionKey sessionKey = iota
	inboundSessionKey
	outboundSessionKey
	proxyRecordSessionKey
	proxySessionSessionKey
	contentSessionKey
)

// ContextWithID returns a new context with the given ID.
func ContextWithID(ctx context.Context, id ID) context.Context {
	return context.WithValue(ctx, idSessionKey, id)
}

// IDFromContext returns ID in this context, or 0 if not contained.
func IDFromContext(ctx context.Context) ID {
	if id, ok := ctx.Value(idSessionKey).(ID); ok {
		return id
	}
	return 0
}

func ContextWithInbound(ctx context.Context, inbound *Inbound) context.Context {
	return context.WithValue(ctx, inboundSessionKey, inbound)
}

func InboundFromContext(ctx context.Context) *Inbound {
	if inbound, ok := ctx.Value(inboundSessionKey).(*Inbound); ok {
		return inbound
	}
	return nil
}

func ContextWithOutbound(ctx context.Context, outbound *Outbound) context.Context {
	return context.WithValue(ctx, outboundSessionKey, outbound)
}

func OutboundFromContext(ctx context.Context) *Outbound {
	if outbound, ok := ctx.Value(outboundSessionKey).(*Outbound); ok {
		return outbound
	}
	return nil
}

func ContextWithProxyRecord(ctx context.Context, record *ProxyRecord) context.Context {
	return context.WithValue(ctx, proxyRecordSessionKey, record)
}

func ProxyRecordFromContext(ctx context.Context) *ProxyRecord {
	if record, ok := ctx.Value(proxyRecordSessionKey).(*ProxyRecord); ok {
		return record
	}
	return nil
}

func ContextWithContent(ctx context.Context, content *Content) context.Context {
	return context.WithValue(ctx, contentSessionKey, content)
}

func ContentFromContext(ctx context.Context) *Content {
	if content, ok := ctx.Value(contentSessionKey).(*Content); ok {
		return content
	}
	return nil
}

func ContextWithProxySession(ctx context.Context, sess *stats.Session) context.Context {
	return context.WithValue(ctx, proxySessionSessionKey, sess)
}

func ProxySessionFromContext(ctx context.Context) *stats.Session {
	if sess, ok := ctx.Value(proxySessionSessionKey).(*stats.Session); ok {
		return sess
	}
	return nil
}
