#!/bin/bash
# macOS Bash 3.2 compatible; installs only a per-user service.
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: bash install-macos.sh [--binary PATH] [--port PORT] [--interface en0]
Without --binary, use dist/uu-tls-fragmenter-darwin-ARCH or build macos/ with Go 1.25+.
UU proxy settings must be configured manually after installation.
EOF
}
fail() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }
binary=''
port=17899
interface=''
while [ "$#" -gt 0 ]; do
  case "$1" in
    --binary|--port|--interface)
      [ "$#" -ge 2 ] || fail "$1 needs a value"
      case "$1" in --binary) binary=$2 ;; --port) port=$2 ;; --interface) interface=$2 ;; esac
      shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) fail "Unknown argument: $1" ;;
  esac
done
[ "$(uname -s)" = Darwin ] || fail 'macOS is required'
[ "$(id -u)" -ne 0 ] || fail 'Run as your login user, without sudo'
case "$port" in ''|*[!0-9]*) fail 'Port must be an integer from 1024 to 65535' ;; esac
[ "${#port}" -le 5 ] || fail 'Invalid port'
port=$((10#$port))
[ "$port" -ge 1024 ] && [ "$port" -le 65535 ] || fail 'Port must be from 1024 to 65535'
case "$interface" in *[!a-zA-Z0-9_-]*) fail 'Invalid interface name' ;; esac
if [ -n "$interface" ]; then /sbin/ifconfig "$interface" >/dev/null 2>&1 || fail "No interface named $interface"; fi
command -v lsof >/dev/null || fail 'lsof is required to check port ownership'

repo_dir="$(cd "$(dirname "$0")" && pwd)"
base_dir="${UU_TLS_FIX_HOME:-$HOME}"
case "$base_dir" in /*) ;; *) fail 'UU_TLS_FIX_HOME must be an absolute path' ;; esac
install_dir="$base_dir/Library/Application Support/UURemoteTlsFix"
log_dir="$base_dir/Library/Logs/UURemoteTlsFix"
agent_dir="$base_dir/Library/LaunchAgents"
label='com.uu-remote-tls-fix.fragmenter'
domain="gui/$(id -u)"
plist="$agent_dir/$label.plist"
agent_info="$(launchctl print "$domain/$label" 2>/dev/null || true)"
agent_pid="$(printf '%s\n' "$agent_info" | awk '/^[[:space:]]*pid = / {print $3; exit}')"
if port_pids="$(lsof -nP -t -iTCP:"$port" -sTCP:LISTEN 2>/dev/null)"; then
  [ -n "$agent_pid" ] && [ "$port_pids" = "$agent_pid" ] || fail "Port $port is already in use; stop that process or choose --port"
fi

case "$(uname -m)" in arm64) arch=arm64 ;; x86_64) arch=amd64 ;; *) fail 'Unsupported Mac architecture' ;; esac
if [ -z "$binary" ] && [ -f "$repo_dir/dist/uu-tls-fragmenter-darwin-$arch" ]; then
  binary="$repo_dir/dist/uu-tls-fragmenter-darwin-$arch"
fi
if [ -n "$binary" ]; then [ -f "$binary" ] || fail "No binary at $binary"; fi
mkdir -p "$install_dir" "$log_dir" "$agent_dir"
stage="$(mktemp "$install_dir/.uu-tls-fragmenter.XXXXXX")"
plist_stage="$(mktemp "$agent_dir/.$label.XXXXXX")"
trap 'rm -f "$stage" "$plist_stage"' EXIT
if [ -n "$binary" ]; then
  cp "$binary" "$stage"
else
  command -v go >/dev/null || fail 'Install Go 1.25+ to build from source, or supply --binary PATH'
  (cd "$repo_dir/macos" && CGO_ENABLED=0 go build -trimpath -o "$stage" .)
fi
chmod 755 "$stage"
"$stage" -version
xml() { printf '%s' "$1" | sed -e 's/\&/\&amp;/g' -e 's/</\&lt;/g' -e 's/>/\&gt;/g' -e 's/"/\&quot;/g' -e "s/'/\&apos;/g"; }
{
  cat <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>com.uu-remote-tls-fix.fragmenter</string>
<key>ProgramArguments</key><array>
EOF
  printf '<string>%s</string>\n' "$(xml "$install_dir/uu-tls-fragmenter")"
  printf '<string>-listen</string><string>127.0.0.1:%s</string>\n' "$port"
  if [ -n "$interface" ]; then printf '<string>-interface</string><string>%s</string>\n' "$(xml "$interface")"; fi
  printf '</array>\n<key>RunAtLoad</key><true/>\n<key>KeepAlive</key><true/>\n<key>ThrottleInterval</key><integer>10</integer>\n'
  printf '<key>StandardOutPath</key><string>%s</string>\n' "$(xml "$log_dir/service.log")"
  printf '<key>StandardErrorPath</key><string>%s</string>\n' "$(xml "$log_dir/service.log")"
  printf '</dict></plist>\n'
} > "$plist_stage"
plutil -lint "$plist_stage" >/dev/null
chmod 644 "$plist_stage"
# Prepare and validate everything before stopping a previous installation.
if [ -n "$agent_info" ]; then launchctl bootout "$domain/$label"; fi
mv -f "$stage" "$install_dir/uu-tls-fragmenter"
mv -f "$plist_stage" "$plist"
if ! launchctl bootstrap "$domain" "$plist"; then
  fail "launchd registration failed. Files are installed; inspect $plist and retry from a logged-in GUI session"
fi
printf '\nInstalled. UU Remote → Settings → Network → Manual proxy:\n'
printf '  Protocol: SOCKS5\n  Host: 127.0.0.1\n  Port: %s\n  Username/password: empty\n' "$port"
printf 'Check: bash "%s/test-macos.sh" --port %s\n' "$repo_dir" "$port"
printf 'Log: %s/service.log\n' "$log_dir"
printf 'System and UU proxy settings were not modified.\n'
