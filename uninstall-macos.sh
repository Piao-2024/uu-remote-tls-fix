#!/bin/bash
set -euo pipefail

case "${1:-}" in -h|--help) echo 'Usage: bash uninstall-macos.sh'; exit 0 ;; '') ;; *) echo 'Unexpected argument' >&2; exit 1 ;; esac
[ "$(uname -s)" = Darwin ] || { echo 'macOS is required' >&2; exit 1; }
[ "$(id -u)" -ne 0 ] || { echo 'Run without sudo' >&2; exit 1; }
base_dir="${UU_TLS_FIX_HOME:-$HOME}"
case "$base_dir" in /*) ;; *) echo 'UU_TLS_FIX_HOME must be an absolute path' >&2; exit 1 ;; esac
label='com.uu-remote-tls-fix.fragmenter'
domain="gui/$(id -u)"
install_dir="$base_dir/Library/Application Support/UURemoteTlsFix"
log_dir="$base_dir/Library/Logs/UURemoteTlsFix"
plist="$base_dir/Library/LaunchAgents/$label.plist"
echo 'Disable the SOCKS5 proxy in UU Remote before using UU without this service.'
if launchctl print "$domain/$label" >/dev/null 2>&1; then
  launchctl bootout "$domain/$label"
fi
# Remove exact owned files only; preserve unrelated files and UU preferences.
rm -f "$plist" "$install_dir/uu-tls-fragmenter" "$log_dir/service.log"
rmdir "$install_dir" "$log_dir" 2>/dev/null || true
echo 'Removed the service and login startup. System and UU proxy settings were not modified.'
