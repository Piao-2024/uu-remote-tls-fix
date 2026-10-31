package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

var version = "dev"

func main() {
	listen := flag.String("listen", "127.0.0.1:17899", "IPv4 loopback SOCKS5 address")
	iface := flag.String("interface", "", "optional physical IPv4 interface, e.g. en0; default: system route")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("uu-tls-fragmenter", version)
		return
	}
	if err := run(*listen, *iface); err != nil {
		log.Fatal(err)
	}
}

func run(address, iface string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil || host != "127.0.0.1" || port == "0" {
		return fmt.Errorf("-listen must use 127.0.0.1 and a nonzero TCP port")
	}
	dialer, err := newDialer(iface)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp4", address)
	if err != nil {
		return err
	}
	defer listener.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() { <-ctx.Done(); listener.Close() }()
	transport := &http.Transport{Proxy: nil, DialContext: dialer.DialContext, TLSHandshakeTimeout: 4 * time.Second, MaxIdleConnsPerHost: 4}
	defer transport.CloseIdleConnections()
	resolver := newResolver(&http.Client{Transport: transport, Timeout: 5 * time.Second}, "https://223.5.5.5/resolve")
	p := &proxy{resolve: resolver.resolve, logger: log.Default(), dial: func(ctx context.Context, addr string) (net.Conn, error) { return dialer.DialContext(ctx, "tcp4", addr) }}
	log.Printf("uu-tls-fragmenter %s listening=%s interface=%q", version, address, iface)
	var workers sync.WaitGroup
	defer workers.Wait()
	// Bound the number of active connections, including idle handshakes.
	slots := make(chan struct{}, 256)
	for {
		client, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			stop()
			return err
		}
		select {
		case slots <- struct{}{}:
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer func() { <-slots }()
				if err := p.handle(ctx, client); err != nil && ctx.Err() == nil {
					log.Printf("connection: %v", err)
				}
			}()
		default:
			client.Close()
		}
	}
}
