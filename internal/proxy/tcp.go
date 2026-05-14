package proxy

import (
	"io"
	"log"
	"net"

	"github.com/ngthdong/gobalancer/internal/balancer"
	"github.com/ngthdong/gobalancer/internal/pool"
)

type TCPProxy struct {
	pool     *pool.BackendPool
	balancer balancer.Balancer
}

func NewTCPProxy(p *pool.BackendPool, b balancer.Balancer) *TCPProxy {
	return &TCPProxy{pool: p, balancer: b}
}

func (p *TCPProxy) HandleConn(client net.Conn) {
	defer client.Close()

	backend := p.balancer.Next(p.pool.Backends())
	if backend == nil {
		log.Printf("no healthy backends available")
		return
	}

	upstream, err := net.Dial("tcp", backend.Addr)
	if err != nil {
		log.Printf("dial %s: %v", backend.Addr, err)
		return
	}
	defer upstream.Close()

	log.Printf("proxying from %s to %s", client.RemoteAddr(), backend.Addr)

	done := make(chan struct{}, 2)
	go func() {
		io.Copy(upstream, client)
		upstream.(*net.TCPConn).CloseWrite()
		done <- struct{}{}
	}()
	go func() {
		io.Copy(client, upstream)
		client.(*net.TCPConn).CloseWrite()
		done <- struct{}{}
	}()
	<-done
}
