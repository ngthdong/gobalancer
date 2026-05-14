// main.go
package main

import (
	"log"
	"net"

	"github.com/ngthdong/gobalancer/internal/balancer"
	"github.com/ngthdong/gobalancer/internal/config"
	"github.com/ngthdong/gobalancer/internal/pool"
	"github.com/ngthdong/gobalancer/internal/proxy"
)

func main() {
	backends := pool.NewBackendPool([]string{
		"localhost:9001",
		"localhost:9002",
		"localhost:9003",
	})

	rr := &balancer.RoundRobin{}
	p := proxy.NewTCPProxy(backends, rr)

	ln, err := net.Listen(config.Protocol, config.Port)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	log.Printf("listening on :8080 with %d backends", backends.Size())

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("accept: %v", err)
			continue
		}
		go p.HandleConn(conn)
	}
}
