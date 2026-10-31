package main

import (
	"fmt"
	"net"
	"syscall"
	"time"
)

func newDialer(name string) (*net.Dialer, error) {
	d := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	if name == "" {
		return d, nil
	}
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return nil, fmt.Errorf("interface %s: %w", name, err)
	}
	d.Control = func(network, address string, raw syscall.RawConn) error {
		var bindErr error
		if err := raw.Control(func(fd uintptr) {
			bindErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, syscall.IP_BOUND_IF, iface.Index)
		}); err != nil {
			return err
		}
		return bindErr
	}
	return d, nil
}
