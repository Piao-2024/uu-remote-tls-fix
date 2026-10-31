package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var fallbackIPs = map[string]string{
	"api.nrd.nie.163.com":             "112.13.123.98",
	"api-ipv4.nrd.nie.163.com":        "112.13.123.98",
	"api-dcdn.nrd.nie.163.com":        "117.148.172.102",
	"api-event.nrd.nie.163.com":       "117.147.201.23",
	"sig-3303-d.nrd.nie.163.com":      "34.95.122.33",
	"relay-mg-3303-d.nrd.nie.163.com": "117.147.201.40",
	"uuyc.163.com":                    "59.111.44.89",
	"uuyc.webapp.163.com":             "117.148.172.102",
	"nrd-ugc.fp.ps.netease.com":       "111.3.79.64",
	"fp.ps.netease.com":               "111.3.79.64",
}

func isUUHost(host string) bool {
	if strings.HasSuffix(host, ".nrd.nie.163.com") {
		for _, c := range host {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '.') {
				return false
			}
		}
		return len(host) <= 253
	}
	_, ok := fallbackIPs[host]
	return ok
}

type dnsEntry struct {
	ip      string
	expires time.Time
}
type resolver struct {
	client   *http.Client
	endpoint string
	mu       sync.Mutex
	cache    map[string]dnsEntry
}

func newResolver(client *http.Client, endpoint string) *resolver {
	return &resolver{client: client, endpoint: endpoint, cache: make(map[string]dnsEntry)}
}

func usableIP(s string) bool {
	ip := net.ParseIP(s)
	if ip == nil || ip.To4() == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() {
		return false
	}
	v4 := ip.To4()
	return !(v4[0] == 198 && (v4[1] == 18 || v4[1] == 19)) && s != "123.58.163.37" && s != "223.252.194.149"
}

func (r *resolver) resolve(ctx context.Context, host string) (string, error) {
	r.mu.Lock()
	entry, ok := r.cache[host]
	r.mu.Unlock()
	if ok && time.Now().Before(entry.expires) {
		return entry.ip, nil
	}
	queryHost := host
	if host == "api-ipv4.nrd.nie.163.com" {
		queryHost = "api.nrd.nie.163.com"
	}
	q := url.Values{"name": {queryHost}, "type": {"A"}}
	if strings.HasPrefix(host, "sig-") {
		q.Set("edns_client_subnet", "8.8.8.0/24")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	resp, err := r.client.Do(req)
	if err == nil {
		defer resp.Body.Close()
		var reply struct {
			Status int
			Answer []struct {
				Type int
				TTL  int
				Data string
			}
		}
		if resp.StatusCode == http.StatusOK && json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&reply) == nil && reply.Status == 0 {
			for _, a := range reply.Answer {
				if a.Type != 1 || !usableIP(a.Data) {
					continue
				}
				ttl := min(max(a.TTL, 0), 300)
				if ttl > 0 {
					r.mu.Lock()
					if len(r.cache) >= 256 {
						clear(r.cache)
					}
					r.cache[host] = dnsEntry{a.Data, time.Now().Add(time.Duration(ttl) * time.Second)}
					r.mu.Unlock()
				}
				return a.Data, nil
			}
		}
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if ip := fallbackIPs[host]; ip != "" {
		return ip, nil
	}
	return "", fmt.Errorf("no usable IPv4 answer or fallback for %s", host)
}
