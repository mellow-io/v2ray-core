package transport

import "v2ray.com/core/common/buf"

// Link is a utility for connecting between an inbound and an outbound proxy handler.
type Link struct {
	Reader buf.LinkReader
	Writer buf.LinkWriter
}
