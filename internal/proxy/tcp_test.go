package proxy_test

import (
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ngthdong/gobalancer/internal/balancer"
	"github.com/ngthdong/gobalancer/internal/config"
	"github.com/ngthdong/gobalancer/internal/pool"
	"github.com/ngthdong/gobalancer/internal/proxy"
)

func TestTCPProxy_ForwardsBytes(t *testing.T) {
	backendLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer backendLn.Close()

	go func() {
		for {
			conn, err := backendLn.Accept()
			if err != nil {
				return
			}

			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c)
			}(conn)
		}
	}()

	p := pool.NewBackendPool([]string{backendLn.Addr().String()})

	tp := proxy.NewTCPProxy(
		p,
		&balancer.RoundRobin{},
		&config.Config{
			Timeouts: config.TimeoutConfig{
				Dial: time.Second,
			},
			Retries: config.RetryConfig{
				MaxAttempts:  3,
				TotalTimeout: 5 * time.Second,
			},
		},
		slog.Default(),
	)

	proxyLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer proxyLn.Close()

	go func() {
		for {
			conn, err := proxyLn.Accept()
			if err != nil {
				return
			}
			go tp.HandleConn(conn)
		}
	}()

	conn, err := net.DialTimeout("tcp", proxyLn.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	payload := []byte("hello gobalancer")

	_, err = conn.Write(payload)
	if err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, len(payload))

	conn.SetReadDeadline(time.Now().Add(time.Second))

	_, err = io.ReadFull(conn, buf)
	if err != nil {
		t.Fatal(err)
	}

	if string(buf) != string(payload) {
		t.Fatalf("got %q want %q", buf, payload)
	}
}

func TestTCPProxy_RetriesDeadBackend(t *testing.T) {
	aliveLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer aliveLn.Close()

	go func() {
		for {
			conn, err := aliveLn.Accept()
			if err != nil {
				return
			}

			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c)
			}(conn)
		}
	}()

	p := pool.NewBackendPool([]string{
		"127.0.0.1:19999",
		aliveLn.Addr().String(),
	})

	tp := proxy.NewTCPProxy(
		p,
		&balancer.RoundRobin{},
		&config.Config{
			Timeouts: config.TimeoutConfig{
				Dial: 500 * time.Millisecond,
			},
			Retries: config.RetryConfig{
				MaxAttempts:  3,
				TotalTimeout: 5 * time.Second,
			},
		},
		slog.Default(),
	)

	proxyLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer proxyLn.Close()

	go func() {
		for {
			conn, err := proxyLn.Accept()
			if err != nil {
				return
			}
			go tp.HandleConn(conn)
		}
	}()

	conn, err := net.DialTimeout("tcp", proxyLn.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	msg := []byte("retry works")

	conn.Write(msg)

	buf := make([]byte, len(msg))

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	_, err = io.ReadFull(conn, buf)
	if err != nil {
		t.Fatal(err)
	}

	if string(buf) != string(msg) {
		t.Fatalf("got %q want %q", buf, msg)
	}
}

func TestTCPProxy_ExhaustsAllBackends(t *testing.T) {
	p := pool.NewBackendPool([]string{
		"127.0.0.1:19001",
		"127.0.0.1:19002",
	})

	tp := proxy.NewTCPProxy(
		p,
		&balancer.RoundRobin{},
		&config.Config{
			Timeouts: config.TimeoutConfig{
				Dial: 200 * time.Millisecond,
			},
			Retries: config.RetryConfig{
				MaxAttempts:  2,
				TotalTimeout: time.Second,
			},
		},
		slog.Default(),
	)

	proxyLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer proxyLn.Close()

	go func() {
		for {
			conn, err := proxyLn.Accept()
			if err != nil {
				return
			}
			go tp.HandleConn(conn)
		}
	}()

	conn, err := net.DialTimeout("tcp", proxyLn.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	buf := make([]byte, 1)

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	_, err = conn.Read(buf)

	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF got %v", err)
	}
}

func TestTCPProxy_ConcurrentConnections(t *testing.T) {
	backendLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer backendLn.Close()

	go func() {
		for {
			conn, err := backendLn.Accept()
			if err != nil {
				return
			}

			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c)
			}(conn)
		}
	}()

	p := pool.NewBackendPool([]string{backendLn.Addr().String()})

	tp := proxy.NewTCPProxy(
		p,
		&balancer.RoundRobin{},
		&config.Config{
			Timeouts: config.TimeoutConfig{
				Dial: time.Second,
			},
			Retries: config.RetryConfig{
				MaxAttempts:  3,
				TotalTimeout: 5 * time.Second,
			},
		},
		slog.Default(),
	)

	proxyLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer proxyLn.Close()

	go func() {
		for {
			conn, err := proxyLn.Accept()
			if err != nil {
				return
			}
			go tp.HandleConn(conn)
		}
	}()

	const clients = 50

	var (
		wg      sync.WaitGroup
		success atomic.Int64
	)

	for i := 0; i < clients; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			conn, err := net.DialTimeout(
				"tcp",
				proxyLn.Addr().String(),
				time.Second,
			)
			if err != nil {
				return
			}
			defer conn.Close()

			msg := []byte("ping")
			conn.Write(msg)
			buf := make([]byte, len(msg))
			conn.SetReadDeadline(time.Now().Add(time.Second))

			_, err = io.ReadFull(conn, buf)

			if err == nil {
				success.Add(1)
			}
		}()
	}

	wg.Wait()

	if success.Load() < clients*9/10 {
		t.Fatalf("too many failed connections")
	}
}