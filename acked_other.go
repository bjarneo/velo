//go:build !linux

package main

import (
	"context"
	"net"
)

// ackTracker needs TCP_INFO. On other systems, upload falls back to the
// count of bytes written.
type ackTracker struct{}

func newAckTracker() *ackTracker { return &ackTracker{} }

func (t *ackTracker) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, network, addr)
}

func (t *ackTracker) acked() (int64, bool) { return 0, false }
