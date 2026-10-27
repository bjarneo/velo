//go:build linux

package main

import (
	"context"
	"net"
	"sync"

	"golang.org/x/sys/unix"
)

// ackTracker sums the bytes that the server acknowledged on every TCP
// connection it dialed. An upload counter that counts writes also counts
// data that only sits in socket buffers, which shows a false peak when the
// streams open.
type ackTracker struct {
	mu    sync.Mutex
	conns []*ackConn
}

type ackConn struct {
	raw  *net.TCPConn
	last uint64
}

func newAckTracker() *ackTracker { return &ackTracker{} }

func (t *ackTracker) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	c, err := (&net.Dialer{}).DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}
	if tc, ok := c.(*net.TCPConn); ok {
		t.mu.Lock()
		t.conns = append(t.conns, &ackConn{raw: tc})
		t.mu.Unlock()
	}
	return c, nil
}

// acked returns the acknowledged bytes, or false when TCP_INFO is not
// readable on an open connection. A closed connection keeps its last reading.
func (t *ackTracker) acked() (int64, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	var sum uint64
	ok := true
	for _, c := range t.conns {
		if rc, err := c.raw.SyscallConn(); err == nil {
			rc.Control(func(fd uintptr) {
				if info, err := unix.GetsockoptTCPInfo(int(fd), unix.IPPROTO_TCP, unix.TCP_INFO); err == nil {
					c.last = info.Bytes_acked
				} else {
					ok = false
				}
			})
		}
		sum += c.last
	}
	return int64(sum), ok
}
