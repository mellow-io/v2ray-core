// +build !confonly

package dispatcher

import (
	"fmt"

	"v2ray.com/core/common"
	"v2ray.com/core/common/buf"
	"v2ray.com/core/common/net"
	"v2ray.com/core/features/stats"

	tdns "github.com/eycorsican/go-tun2socks/common/dns"
	tstats "github.com/eycorsican/go-tun2socks/common/stats"
)

type InboundSizeWriter struct {
	Session *tstats.Session
	Writer  buf.LinkWriter
}

func (w *InboundSizeWriter) WriteMultiBuffer(mb buf.MultiBuffer) error {
	w.Session.AddUploadBytes(int64(mb.Len()))
	return w.Writer.WriteMultiBuffer(mb)
}

func (w *InboundSizeWriter) WritePacket(b *buf.Buffer, addr *net.UDPAddr) error {
	w.Session.AddUploadBytes(int64(b.Len()))
	if addr.Port == 53 {
		qtype, domain, err := tdns.ParseDNSQuery(b.Bytes())
		if err == nil {
			w.Session.Extra = fmt.Sprintf("%s:%s", qtype, domain)
		}
	}
	return w.Writer.WritePacket(b, addr)
}

func (w *InboundSizeWriter) Close() error {
	return common.Close(w.Writer)
}

func (w *InboundSizeWriter) Interrupt() {
	common.Interrupt(w.Writer)
}

type OutboundSizeWriter struct {
	Session *tstats.Session
	Writer  buf.LinkWriter
}

func (w *OutboundSizeWriter) WriteMultiBuffer(mb buf.MultiBuffer) error {
	w.Session.AddDownloadBytes(int64(mb.Len()))
	return w.Writer.WriteMultiBuffer(mb)
}

func (w *OutboundSizeWriter) WritePacket(b *buf.Buffer, addr *net.UDPAddr) error {
	w.Session.AddDownloadBytes(int64(b.Len()))
	return w.Writer.WritePacket(b, addr)
}

func (w *OutboundSizeWriter) Close() error {
	return common.Close(w.Writer)
}

func (w *OutboundSizeWriter) Interrupt() {
	common.Interrupt(w.Writer)
}

type SizeStatWriter struct {
	Counter stats.Counter
	Writer  buf.LinkWriter
}

func (w *SizeStatWriter) WriteMultiBuffer(mb buf.MultiBuffer) error {
	w.Counter.Add(int64(mb.Len()))
	return w.Writer.WriteMultiBuffer(mb)
}

func (w *SizeStatWriter) WritePacket(b *buf.Buffer, addr *net.UDPAddr) error {
	w.Counter.Add(int64(b.Len()))
	return w.Writer.WritePacket(b, addr)
}

func (w *SizeStatWriter) Close() error {
	return common.Close(w.Writer)
}

func (w *SizeStatWriter) Interrupt() {
	common.Interrupt(w.Writer)
}
