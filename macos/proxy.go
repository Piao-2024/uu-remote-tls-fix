package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"time"
)

func socksReply(w io.Writer, code byte) error {
	_, err := w.Write([]byte{5, code, 0, 1, 0, 0, 0, 0, 0, 0})
	return err
}

func readSOCKSRequest(rw io.ReadWriter) (string, error) {
	var header [2]byte
	if _, err := io.ReadFull(rw, header[:]); err != nil {
		return "", err
	}
	if header[0] != 5 {
		return "", fmt.Errorf("SOCKS5 required")
	}
	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(rw, methods); err != nil {
		return "", err
	}
	if !bytes.Contains(methods, []byte{0}) {
		rw.Write([]byte{5, 255})
		return "", fmt.Errorf("SOCKS5 anonymous authentication required")
	}
	if _, err := rw.Write([]byte{5, 0}); err != nil {
		return "", err
	}
	var request [4]byte
	if _, err := io.ReadFull(rw, request[:]); err != nil {
		return "", err
	}
	if request[0] != 5 || request[1] != 1 || request[2] != 0 {
		socksReply(rw, 7)
		return "", fmt.Errorf("only SOCKS5 CONNECT is supported")
	}
	if request[3] != 3 {
		socksReply(rw, 8)
		return "", fmt.Errorf("use SOCKS5 remote DNS / a UU hostname, not a literal IP")
	}
	var length [1]byte
	if _, err := io.ReadFull(rw, length[:]); err != nil {
		return "", err
	}
	nameAndPort := make([]byte, int(length[0])+2)
	if _, err := io.ReadFull(rw, nameAndPort); err != nil {
		return "", err
	}
	host := strings.TrimSuffix(strings.ToLower(string(nameAndPort[:length[0]])), ".")
	port := binary.BigEndian.Uint16(nameAndPort[length[0]:])
	if !isUUHost(host) || port != 443 {
		socksReply(rw, 2)
		return "", fmt.Errorf("only approved UU HTTPS hostnames on port 443 are supported")
	}
	return host, nil
}

type proxy struct {
	resolve func(context.Context, string) (string, error)
	dial    func(context.Context, string) (net.Conn, error)
	logger  *log.Logger
}

func (p *proxy) handle(ctx context.Context, client net.Conn) error {
	defer client.Close()
	client.SetDeadline(time.Now().Add(20 * time.Second))
	stopClient := context.AfterFunc(ctx, func() { client.Close() })
	defer stopClient()
	host, err := readSOCKSRequest(client)
	if err != nil {
		return err
	}
	ip, err := p.resolve(ctx, host)
	if err != nil {
		socksReply(client, 4)
		return err
	}
	upstream, err := p.dial(ctx, net.JoinHostPort(ip, "443"))
	if err != nil {
		socksReply(client, 5)
		return fmt.Errorf("connect %s: %w", host, err)
	}
	defer upstream.Close()
	stopUpstream := context.AfterFunc(ctx, func() { upstream.Close() })
	defer stopUpstream()
	upstream.SetDeadline(time.Now().Add(20 * time.Second))
	if err := socksReply(client, 0); err != nil {
		return err
	}
	record, err := readTLSRecord(client)
	if err != nil {
		return err
	}
	output := fragmentClientHello(record)
	if _, err := io.Copy(upstream, bytes.NewReader(output)); err != nil {
		return err
	}
	p.logger.Printf("host=%s upstream=%s fragmented=%t", host, ip, len(output) > len(record))
	client.SetDeadline(time.Time{})
	upstream.SetDeadline(time.Time{})
	done := make(chan struct{}, 2)
	go func() { io.Copy(upstream, client); done <- struct{}{} }()
	go func() { io.Copy(client, upstream); done <- struct{}{} }()
	<-done
	client.Close()
	upstream.Close()
	<-done
	return nil
}
