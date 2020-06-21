package log

import (
	"context"
	"strings"

	"v2ray.com/core/common/serial"
)

type logKey int

const (
	accessMessageKey logKey = iota
)

type AccessStatus string

const (
	AccessAccepted = AccessStatus("accepted")
	AccessRejected = AccessStatus("rejected")
)

type AccessMessage struct {
	From        interface{}
	To          interface{}
	Status      AccessStatus
	Reason      interface{}
	InboundTag  interface{}
	OutboundTag interface{}
}

func (m *AccessMessage) String() string {
	builder := strings.Builder{}
	builder.WriteByte('[')
	builder.WriteString(serial.ToString(m.InboundTag))
	builder.WriteByte(']')
	builder.WriteByte(' ')
	builder.WriteString(serial.ToString(m.From))
	builder.WriteByte(' ')
	builder.WriteString(string(m.Status))
	builder.WriteByte(' ')
	builder.WriteByte('[')
	builder.WriteString(serial.ToString(m.OutboundTag))
	builder.WriteByte(']')
	builder.WriteByte(' ')
	builder.WriteString(serial.ToString(m.To))
	builder.WriteByte(' ')
	builder.WriteString(serial.ToString(m.Reason))
	return builder.String()
}

func ContextWithAccessMessage(ctx context.Context, accessMessage *AccessMessage) context.Context {
	return context.WithValue(ctx, accessMessageKey, accessMessage)
}

func AccessMessageFromContext(ctx context.Context) *AccessMessage {
	if accessMessage, ok := ctx.Value(accessMessageKey).(*AccessMessage); ok {
		return accessMessage
	}
	return nil
}
