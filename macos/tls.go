package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"strings"
)

const maxTLSRecord = 18432

func readTLSRecord(r io.Reader) ([]byte, error) {
	header := make([]byte, 5)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}
	n := int(binary.BigEndian.Uint16(header[3:]))
	if header[0] != 22 || header[1] != 3 || n == 0 || n > maxTLSRecord {
		return nil, fmt.Errorf("invalid initial TLS handshake record")
	}
	record := make([]byte, 5+n)
	copy(record, header)
	_, err := io.ReadFull(r, record[5:])
	return record, err
}

// sniRange returns the byte range of the actual host_name extension. A malformed
// or multi-record ClientHello is left unchanged rather than guessed at.
func sniRange(record []byte) (int, int) {
	if len(record) < 44 || record[0] != 22 || record[1] != 3 || record[5] != 1 || int(binary.BigEndian.Uint16(record[3:5])) != len(record)-5 {
		return 0, 0
	}
	b := record[5:]
	if int(b[1])<<16|int(b[2])<<8|int(b[3]) != len(b)-4 {
		return 0, 0
	}
	p := 38
	p += 1 + int(b[p]) // session id
	if p+2 > len(b) {
		return 0, 0
	}
	p += 2 + int(binary.BigEndian.Uint16(b[p:p+2])) // cipher suites
	if p >= len(b) {
		return 0, 0
	}
	p += 1 + int(b[p]) // compression methods
	if p+2 > len(b) {
		return 0, 0
	}
	end := p + 2 + int(binary.BigEndian.Uint16(b[p:p+2]))
	p += 2
	if end != len(b) {
		return 0, 0
	}
	for p+4 <= end {
		kind := binary.BigEndian.Uint16(b[p : p+2])
		n := int(binary.BigEndian.Uint16(b[p+2 : p+4]))
		p += 4
		if p+n > end {
			return 0, 0
		}
		if kind == 0 {
			if n < 5 || int(binary.BigEndian.Uint16(b[p:p+2])) != n-2 {
				return 0, 0
			}
			limit := p + n
			for q := p + 2; q+3 <= limit; {
				nameType := b[q]
				length := int(binary.BigEndian.Uint16(b[q+1 : q+3]))
				q += 3
				if q+length > limit {
					return 0, 0
				}
				if nameType == 0 && length > 0 {
					return 5 + q, 5 + q + length
				}
				q += length
			}
			return 0, 0
		}
		p += n
	}
	return 0, 0
}

func fragmentClientHello(record []byte) []byte {
	start, end := sniRange(record)
	if end == 0 {
		return record
	}
	host := strings.ToLower(string(record[start:end]))
	if !strings.HasSuffix(host, ".nrd.nie.163.com") {
		return record
	}
	// Split five bytes into the suffix, matching the proven Windows algorithm.
	cut := start + len(host) - len("nrd.nie.163.com") + 5
	first, second := record[5:cut], record[cut:]
	result := make([]byte, 0, len(record)+5)
	for _, part := range [][]byte{first, second} {
		result = append(result, record[0], record[1], record[2], byte(len(part)>>8), byte(len(part)))
		result = append(result, part...)
	}
	return result
}
