// +build !confonly

package router

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"v2ray.com/core/app/measure"
	"v2ray.com/core/common/dice"
	"v2ray.com/core/features/outbound"
)

var ShouldPerformLatencyTest = true

type Server struct {
	latency time.Duration
	tag     string
}

type By func(p1, p2 *Server) bool

func (by By) Sort(servers []Server) {
	ss := &serverSorter{
		servers: servers,
		by:      by,
	}
	sort.Sort(ss)
}

type serverSorter struct {
	servers []Server
	by      By
}

func (s *serverSorter) Len() int {
	return len(s.servers)
}

func (s *serverSorter) Swap(i, j int) {
	s.servers[i], s.servers[j] = s.servers[j], s.servers[i]
}

func (s *serverSorter) Less(i, j int) bool {
	return s.by(&s.servers[i], &s.servers[j])
}

type LatencyStrategy struct {
	sync.Mutex

	ohm       outbound.Manager
	selectors []string

	totalMeasures int           // 2
	interval      time.Duration // 120 * time.Second
	delay         time.Duration // 0 * time.Second
	timeout       time.Duration // 6 * time.Second
	tolerance     time.Duration // 300 * time.Millisecond
	target        string        // "tls:www.google.com:443"
	content       string        // "HEAD / HTTP/1.1\r\n\r\n"

	lastMeasure time.Time

	servers        []Server
	selectedServer *Server
}

func NewLatencyStrategy(ohm outbound.Manager, selectors []string, totalMeasures int, interval, delay, timeout, tolerance time.Duration, target, content string) BalancingStrategy {
	s := &LatencyStrategy{
		ohm:            ohm,
		selectors:      selectors,
		totalMeasures:  totalMeasures,
		interval:       interval,
		delay:          delay,
		timeout:        timeout,
		tolerance:      tolerance,
		target:         target,
		content:        content,
		servers:        make([]Server, 0),
		selectedServer: nil,
	}

	go func() {
		time.Sleep(4 * time.Second)
		newError(fmt.Sprintf("new latency balancer with totalMeasures %v, interval %v, delay %v, timeout %v, tolerance: %v, probeTarget %v, probeContent %v", totalMeasures, interval, delay, timeout, tolerance, target, content)).WriteToLog()
		s.measureOnce()
	}()
	return s
}

func (s *LatencyStrategy) PickOutbound(tags []string) string {
	s.Lock()
	defer s.Unlock()

	n := len(tags)
	if n == 0 {
		panic("0 tags")
	}
	if s.selectedServer == nil {
		return tags[dice.Roll(n)]
	}

	now := time.Now()
	if now.Sub(s.lastMeasure) > s.interval {
		s.lastMeasure = now
		go s.measureOnce()
	}

	return s.selectedServer.tag
}

func (s *LatencyStrategy) measureOnce() {
	servers := make([]Server, 0)

	hs, ok := s.ohm.(outbound.HandlerSelector)
	if !ok {
		panic("not selecter")
	}
	tags := hs.Select(s.selectors)
	if len(tags) == 0 {
		panic("no tags")
	}
	for _, tag := range tags {
		h := s.ohm.GetHandler(tag)
		var totalLatency int64 = 0
		for i := 0; i < s.totalMeasures; i++ {
			newError(fmt.Sprintf("measuring %v, target: %v", tag, s.target)).AtDebug().WriteToLog()
			latency := measure.MeasureLatency(h, s.target, s.content, s.timeout)
			totalLatency += latency.Nanoseconds()
			// Waits 1 second between each measure.
			time.Sleep(s.delay)
		}
		avgLatency := time.Duration(int64(float64(totalLatency) / float64(s.totalMeasures)))
		server := Server{
			latency: avgLatency,
			tag:     tag,
		}
		servers = append(servers, server)
	}
	latency := func(p1, p2 *Server) bool {
		return p1.latency.Nanoseconds() < p2.latency.Nanoseconds()
	}
	By(latency).Sort(servers)
	for _, server := range servers {
		newError(fmt.Sprintf("outbound: %v, target: %v, latency: %v", server.tag, s.target, server.latency.String())).WriteToLog()
	}

	s.Lock()
	s.selectedServer = s.selectServer(s.servers, servers)
	newError(fmt.Sprintf("selected outbound: %v, latency: %v", s.selectedServer.tag, s.selectedServer.latency.String())).WriteToLog()
	s.servers = servers
	s.Unlock()
}

func (s *LatencyStrategy) selectServer(oldMeasures, newMeasures []Server) *Server {
	if len(oldMeasures) == 0 {
		if len(newMeasures) > 0 {
			return &newMeasures[0]
		}
		return nil
	}
	if len(newMeasures) == 0 {
		return s.selectedServer
	}

	newBest := newMeasures[0]
	for _, m := range newMeasures {
		if m.tag == s.selectedServer.tag {
			if newBest.latency.Nanoseconds() < (m.latency.Nanoseconds() - s.tolerance.Nanoseconds()) {
				return &newBest
			} else {
				return &m
			}
		}
	}
	return nil
}
