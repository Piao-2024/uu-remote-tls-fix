#!/bin/bash
set -euo pipefail

repo_dir="$(cd "$(dirname "$0")/.." && pwd)"
test_dir="$(mktemp -d "${TMPDIR:-/tmp}/uu-macos-scripts.XXXXXX")"
trap 'rm -rf "$test_dir"' EXIT
export UU_TLS_FIX_HOME="$test_dir/user & <test>"
mkdir -p "$test_dir/bin" "$UU_TLS_FIX_HOME/Library/Application Support/UURemoteTlsFix"
export LAUNCH_LOG="$test_dir/launch.log"
export PATH="$test_dir/bin:$PATH"
cat > "$test_dir/bin/launchctl" <<'EOF'
#!/bin/bash
printf '%s\n' "$*" >> "$LAUNCH_LOG"
case "$1" in
  print) exit 1 ;;
  bootout) exit 3 ;;
  *) exit 0 ;;
esac
EOF
cat > "$test_dir/bin/lsof" <<'EOF'
#!/bin/bash
exit "${LSOF_TEST_STATUS:-1}"
EOF
cat > "$test_dir/test-binary" <<'EOF'
#!/bin/bash
printf 'uu-tls-fragmenter test\n'
EOF
chmod +x "$test_dir/bin/launchctl" "$test_dir/bin/lsof" "$test_dir/test-binary"
install_dir="$UU_TLS_FIX_HOME/Library/Application Support/UURemoteTlsFix"
plist="$UU_TLS_FIX_HOME/Library/LaunchAgents/com.uu-remote-tls-fix.fragmenter.plist"
printf 'keep me\n' > "$install_dir/unrelated.txt"

# A conflicting port must fail before replacing an installed program or plist.
export LSOF_TEST_STATUS=0
if bash "$repo_dir/install-macos.sh" --binary "$test_dir/test-binary" --port 17899; then
  echo 'FAIL: accepted port conflict' >&2; exit 1
fi
test ! -e "$plist"
test ! -e "$install_dir/uu-tls-fragmenter"
export LSOF_TEST_STATUS=1

bash "$repo_dir/install-macos.sh" --binary "$test_dir/test-binary" --port 17901
test -x "$install_dir/uu-tls-fragmenter"
plutil -lint "$plist"
test "$(/usr/libexec/PlistBuddy -c 'Print :ProgramArguments:2' "$plist")" = '127.0.0.1:17901'
test "$(/usr/libexec/PlistBuddy -c 'Print :ProgramArguments:0' "$plist")" = "$install_dir/uu-tls-fragmenter"
grep -q '^bootstrap ' "$LAUNCH_LOG"

# Reinstall and uninstall are repeatable; unrelated files survive.
bash "$repo_dir/install-macos.sh" --binary "$test_dir/test-binary" --port 17901
bash "$repo_dir/uninstall-macos.sh"
bash "$repo_dir/uninstall-macos.sh"
test ! -e "$plist"
test ! -e "$install_dir/uu-tls-fragmenter"
test -f "$install_dir/unrelated.txt"
printf 'PASS: macOS script lifecycle, XML paths, port conflict, ownership\n'
