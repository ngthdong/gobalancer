package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"sync"
	"time"

	"github.com/ngthdong/gobalancer/internal/balancer"
	"github.com/ngthdong/gobalancer/internal/config"
	"github.com/ngthdong/gobalancer/internal/conntrack"
	"github.com/ngthdong/gobalancer/internal/pool"
)

type TCPProxy struct {
	pool     *pool.BackendPool
	balancer balancer.Balancer
	cfg      *config.Config
	logger   *slog.Logger
	tracker  *conntrack.Tracker
}

func NewTCPProxy(
	p *pool.BackendPool,
	b balancer.Balancer,
	cfg *config.Config,
	logger *slog.Logger,
	tracker *conntrack.Tracker,
) *TCPProxy {
	return &TCPProxy{
		pool:     p,
		balancer: b,
		cfg:      cfg,
		logger:   logger,
		tracker:  tracker,
	}
}

func (p *TCPProxy) ListenAndServe(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	return p.Serve(ln)
}

func (p *TCPProxy) Serve(ln net.Listener) error {
	defer ln.Close()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				p.logger.Info("listener closed, stopping accept loop")
				return nil
			}
			p.logger.Warn("accept error", "error", err)
			continue
		}
		go p.HandleConn(conn)
	}
}

func (p *TCPProxy) HandleConn(client net.Conn) {
	defer client.Close()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		p.cfg.Retries.TotalTimeout,
	)
	defer cancel()

	id := fmt.Sprintf("%s-%d", client.RemoteAddr(), time.Now().UnixNano())
	record := &conntrack.ConnRecord{
		ID:         id,
		ClientAddr: client.RemoteAddr().String(),
		StartTime:  time.Now(),
	}
	defer p.tracker.Track(record)()

	excluded := make(map[string]struct{})
	backends := p.pool.Backends()
	maxAttempts := p.cfg.Retries.MaxAttempts

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		var backend *pool.Backend
		if attempt == 1 {
			backend = p.balancer.Next(backends)
		} else {
			backend = p.balancer.NextExcluding(backends, excluded)
		}

		if backend == nil {
			p.logger.Warn("no available backends",
				"attempt", attempt,
				"excluded", len(excluded),
			)
			return
		}

		err := p.tryBackend(ctx, client, backend, attempt)
		if err == nil {
			record.Backend = backend.Addr
			return
		}

		excluded[backend.Addr] = struct{}{}

		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			p.logger.Warn("context done, aborting retries",
				"backend", backend.Addr,
				"attempt", attempt,
				"error", err,
			)
			return
		}

		if !isRetryable(err) {
			p.logger.Error("non-retryable error",
				"backend", backend.Addr,
				"attempt", attempt,
				"error", err,
			)
			return
		}

		p.logger.Warn("backend failed, retrying",
			"backend", backend.Addr,
			"attempt", attempt,
			"max_attempts", maxAttempts,
			"error", err,
		)

		if attempt < maxAttempts {
			p.backoff(ctx, attempt)
		}
	}

	p.logger.Error("all retry attempts exhausted",
		"excluded_backends", len(excluded),
	)
}

// tryBackend dials the backend and splices bytes bidirectionally.
func (p *TCPProxy) tryBackend(
	ctx context.Context,
	client net.Conn,
	backend *pool.Backend,
	attempt int,
) error {
	backend.TrackConn(+1)
	defer backend.TrackConn(-1)

	dialCtx, cancel := context.WithTimeout(ctx, p.cfg.Timeouts.Dial)
	defer cancel()

	var d net.Dialer
	upstream, err := d.DialContext(dialCtx, "tcp", backend.Addr)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer upstream.Close()

	p.logger.Info("proxying connection",
		"client", client.RemoteAddr(),
		"backend", backend.Addr,
		"attempt", attempt,
	)

	done := make(chan error, 2)

	go func() {
		_, err := copyWithPool(upstream, client)
		upstream.(*net.TCPConn).CloseWrite()
		done <- err
	}()
	go func() {
		_, err := copyWithPool(client, upstream)
		client.(*net.TCPConn).CloseWrite()
		done <- err
	}()

	err = <-done
	if err != nil && !isEOF(err) {
		return fmt.Errorf("copy: %w", err)
	}
	return nil
}

func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		return true
	}
	return isEOF(err)
}

func isEOF(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}

// backoff sleeps for an exponentially increasing jittered duration.
func (p *TCPProxy) backoff(ctx context.Context, attempt int) {
	base := time.Duration(10*(1<<uint(attempt-1))) * time.Millisecond
	if base > 500*time.Millisecond {
		base = 500 * time.Millisecond
	}

	jitter := time.Duration(rand.Int64N(int64(base / 2)))
	delay := base/2 + jitter

	select {
	case <-time.After(delay):
	case <-ctx.Done():
	}
}

// Every io.Copy call allocates a 32KB buffer internally.
// Under 1000 concurrent connections, that's 32MB of allocations that GC has to reclaim.
// sync.Pool reuse those buffers.
var copyBufPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 32*1024)
		return &buf
	},
}

func copyWithPool(dst io.Writer, src io.Reader) (int64, error) {
	bufPtr := copyBufPool.Get().(*[]byte)
	defer copyBufPool.Put(bufPtr)
	return io.CopyBuffer(dst, src, *bufPtr)
}
