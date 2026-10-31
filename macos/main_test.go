package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/iotest"
	"time"
)

// Real crypto/tls output catches accidental changes to handshake bytes and SNI offsets.
func clientHello(t *testing.T, host string, protocols ...string) []byte {
	t.Helper()
	client, peer := net.Pipe()
	t.Cleanup(func() { client.Close(); peer.Close() })
	peer.SetDeadline(time.Now().Add(2 * time.Second))
	go func() { _ = tls.Client(client, &tls.Config{ServerName: host, NextProtos: protocols}).Handshake() }()
	h := make([]byte, 5)
	if _, err := io.ReadFull(peer, h); err != nil {
		t.Fatal(err)
	}
	b := make([]byte, int(binary.BigEndian.Uint16(h[3:])))
	if _, err := io.ReadFull(peer, b); err != nil {
		t.Fatal(err)
	}
	return append(h, b...)
}

func TestFragmentPreservesHandshakeAndSplitsInsideSNI(t *testing.T) {
	original := clientHello(t, "api.nrd.nie.163.com")
	got := fragmentClientHello(original)
	firstLength := int(binary.BigEndian.Uint16(got[3:5]))
	boundary := 5 + firstLength
	if boundary+5 >= len(got) || len(got) != len(original)+5 {
		t.Fatal("expected two nonempty TLS records")
	}
	if !bytes.Equal(got[:3], original[:3]) || !bytes.Equal(got[boundary:boundary+3], original[:3]) {
		t.Fatal("record type/version changed")
	}
	joined := append(bytes.Clone(got[5:boundary]), got[boundary+5:]...)
	if !bytes.Equal(joined, original[5:]) {
		t.Fatal("handshake bytes changed")
	}
	if bytes.Contains(got[5:boundary], []byte("nrd.nie.163.com")) || bytes.Contains(got[boundary+5:], []byte("nrd.nie.163.com")) {
		t.Fatal("SNI suffix remains in one record")
	}
	if int(binary.BigEndian.Uint16(got[boundary+3:boundary+5])) != len(got)-boundary-5 {
		t.Fatal("invalid second record length")
	}
}

func TestFragmentOnlyMatchesRealUUSNI(t *testing.T) {
	for _, record := range [][]byte{
		clientHello(t, "example.com", "nrd.nie.163.com"),
		clientHello(t, "evil-nrd.nie.163.com.example.org"),
		{22, 3, 3, 0, 10, 1},
		{23, 3, 3, 0, 1, 1},
		{},
	} {
		if !bytes.Equal(fragmentClientHello(record), record) {
			t.Fatal("changed unrelated or incomplete record")
		}
	}
}

func TestReadTLSRecordWithSingleByteReads(t *testing.T) {
	hello := clientHello(t, "api.nrd.nie.163.com")
	reader := bytes.NewReader(append(bytes.Clone(hello), 99))
	got, err := readTLSRecord(iotest.OneByteReader(reader))
	if err != nil || !bytes.Equal(got, hello) || reader.Len() != 1 {
		t.Fatalf("record framing: %v, remaining=%d", err, reader.Len())
	}
}

func TestReadTLSRecordRejectsInvalidInput(t *testing.T) {
	for _, data := range [][]byte{{22, 3}, {22, 3, 3, 0, 2, 1}, {22, 3, 3, 255, 255}, {1, 3, 3, 0, 1, 1}, {22, 3, 3, 0, 0}} {
		if _, err := readTLSRecord(bytes.NewReader(data)); err == nil {
			t.Fatalf("accepted %x", data)
		}
	}
}

type memoryRW struct {
	io.Reader
	bytes.Buffer
}

func (r *memoryRW) Read(p []byte) (int, error) { return r.Reader.Read(p) }

func socksRequest(host string, port uint16) []byte {
	b := []byte{5, 1, 0, 5, 1, 0, 3, byte(len(host))}
	b = append(b, host...)
	return append(b, byte(port>>8), byte(port))
}

func TestSOCKSReadsChunkedDomainRequest(t *testing.T) {
	r := &memoryRW{Reader: iotest.OneByteReader(bytes.NewReader(socksRequest("API.NRD.NIE.163.COM", 443)))}
	host, err := readSOCKSRequest(r)
	if err != nil || host != "api.nrd.nie.163.com" || !bytes.Equal(r.Bytes(), []byte{5, 0}) {
		t.Fatalf("host=%s reply=%x error=%v", host, r.Bytes(), err)
	}
}

func TestSOCKSRejectsUnsupportedRequests(t *testing.T) {
	for _, tc := range []struct {
		name           string
		request, reply []byte
	}{
		{"auth", []byte{5, 1, 2}, []byte{5, 255}},
		{"udp", []byte{5, 1, 0, 5, 3, 0, 1}, []byte{5, 0, 5, 7, 0, 1, 0, 0, 0, 0, 0, 0}},
		{"ip", []byte{5, 1, 0, 5, 1, 0, 1, 127, 0, 0, 1, 1, 187}, []byte{5, 0, 5, 8, 0, 1, 0, 0, 0, 0, 0, 0}},
		{"other-host", socksRequest("example.org", 443), []byte{5, 0, 5, 2, 0, 1, 0, 0, 0, 0, 0, 0}},
		{"wrong-port", socksRequest("api.nrd.nie.163.com", 80), []byte{5, 0, 5, 2, 0, 1, 0, 0, 0, 0, 0, 0}},
		{"suffix-trick", socksRequest("evilnrd.nie.163.com", 443), []byte{5, 0, 5, 2, 0, 1, 0, 0, 0, 0, 0, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &memoryRW{Reader: bytes.NewReader(tc.request)}
			if _, err := readSOCKSRequest(r); err == nil {
				t.Fatal("accepted unsupported request")
			}
			if !bytes.Equal(r.Bytes(), tc.reply) {
				t.Fatalf("reply=%x want=%x", r.Bytes(), tc.reply)
			}
		})
	}
}

func TestResolverAppliesPoliciesAndCaches(t *testing.T) {
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		q := r.URL.Query()
		if q.Get("type") != "A" {
			t.Error("missing A query")
		}
		switch q.Get("name") {
		case "api.nrd.nie.163.com":
			fmt.Fprint(w, `{"Status":0,"Answer":[{"type":1,"TTL":60,"data":"112.13.123.98"}]}`)
		case "sig-3303-d.nrd.nie.163.com":
			if q.Get("edns_client_subnet") != "8.8.8.0/24" {
				t.Error("missing signal ECS")
			}
			fmt.Fprint(w, `{"Status":0,"Answer":[{"type":1,"TTL":60,"data":"34.95.122.33"}]}`)
		default:
			t.Error("wrong query name")
		}
	}))
	defer s.Close()
	r := newResolver(s.Client(), s.URL)
	for _, tc := range []struct{ host, want string }{{"api-ipv4.nrd.nie.163.com", "112.13.123.98"}, {"api-ipv4.nrd.nie.163.com", "112.13.123.98"}, {"sig-3303-d.nrd.nie.163.com", "34.95.122.33"}} {
		got, err := r.resolve(context.Background(), tc.host)
		if err != nil || got != tc.want {
			t.Fatalf("got=%s err=%v", got, err)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("cache missed: calls=%d", calls.Load())
	}
}

func TestResolverRejectsBadAnswersAndFallsBack(t *testing.T) {
	for _, answer := range []string{"123.58.163.37", "223.252.194.149", "127.0.0.1", "10.0.0.1", "198.18.0.2", "::1", "not-an-ip"} {
		t.Run(answer, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, `{"Status":0,"Answer":[{"type":1,"TTL":60,"data":%q}]}`, answer)
			}))
			defer s.Close()
			r := newResolver(s.Client(), s.URL)
			got, err := r.resolve(context.Background(), "api.nrd.nie.163.com")
			if err != nil || got != "112.13.123.98" {
				t.Fatalf("fallback=%s err=%v", got, err)
			}
			if _, err = r.resolve(context.Background(), "unknown.nrd.nie.163.com"); err == nil {
				t.Fatal("accepted bad answer without fallback")
			}
		})
	}
}

func TestResolverHandlesHTTPFailure(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
		io.WriteString(w, strings.Repeat("x", 10))
	}))
	defer s.Close()
	got, err := newResolver(s.Client(), s.URL).resolve(context.Background(), "api.nrd.nie.163.com")
	if err != nil || got != "112.13.123.98" {
		t.Fatalf("fallback=%s err=%v", got, err)
	}
}
