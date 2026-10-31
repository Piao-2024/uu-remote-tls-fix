#!/bin/bash
set -euo pipefail
port=17899
case "${1:-}" in
  --port) [ "$#" -eq 2 ] || { echo 'Usage: bash test-macos.sh [--port PORT]' >&2; exit 1; }; port=$2 ;;
  -h|--help) echo 'Usage: bash test-macos.sh [--port PORT]'; exit 0 ;;
  '') ;;
  *) echo 'Usage: bash test-macos.sh [--port PORT]' >&2; exit 1 ;;
esac
case "$port" in ''|*[!0-9]*) echo 'Invalid port' >&2; exit 1 ;; esac
[ "${#port}" -le 5 ] || { echo 'Invalid port' >&2; exit 1; }
port=$((10#$port))
[ "$port" -gt 0 ] && [ "$port" -le 65535 ] || { echo 'Invalid port' >&2; exit 1; }
failed=0
for host in api.nrd.nie.163.com api-dcdn.nrd.nie.163.com api-event.nrd.nie.163.com api-ipv4.nrd.nie.163.com sig-3303-d.nrd.nie.163.com relay-mg-3303-d.nrd.nie.163.com; do
  if result="$(/usr/bin/curl --noproxy '' --socks5-hostname "127.0.0.1:$port" --silent --show-error --output /dev/null --connect-timeout 8 --max-time 12 --write-out 'HTTP %{http_code}; TLS %{time_appconnect}s; total %{time_total}s' "https://$host/" 2>&1)"; then
    printf '[OK]   %-36s %s\n' "$host" "$result"
  else
    printf '[FAIL] %-36s %s\n' "$host" "$result"
    failed=$((failed+1))
  fi
done
echo 'Any HTTP response (including 404) proves HTTPS reachability, not remote-control functionality.'
[ "$failed" -eq 0 ]
