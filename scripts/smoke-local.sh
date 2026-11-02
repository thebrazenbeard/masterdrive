#!/usr/bin/env bash
# Local-only MasterDrive smoke test. Creates and removes only its own fixture.
set -euo pipefail

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
agent_addr="${MASTERDRIVE_SMOKE_AGENT_ADDR:-127.0.0.1:18789}"
controller_addr="${MASTERDRIVE_SMOKE_CONTROLLER_ADDR:-127.0.0.1:18788}"

if [[ ! "$agent_addr" =~ ^127\.0\.0\.1:[0-9]+$ || ! "$controller_addr" =~ ^127\.0\.0\.1:[0-9]+$ ]]; then
  echo 'Smoke listeners must use 127.0.0.1 with explicit ports.' >&2
  exit 2
fi
for command_name in go curl cmp od tr sed grep sleep; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "Missing required command: $command_name" >&2
    exit 2
  fi
done
if [[ ! -d "$repo_root/web" ]]; then
  echo 'Missing web/ UI; integrate the frontend before running this smoke test.' >&2
  exit 2
fi

tmp_parent="${TMPDIR:-/tmp}"
smoke_dir="$(mktemp -d "$tmp_parent/masterdrive-smoke.XXXXXXXX")"
chmod 700 "$smoke_dir"
agent_pid=''
controller_pid=''
cleanup() {
  if [[ -n "$controller_pid" ]]; then
    kill "$controller_pid" 2>/dev/null || true
    wait "$controller_pid" 2>/dev/null || true
  fi
  if [[ -n "$agent_pid" ]]; then
    kill "$agent_pid" 2>/dev/null || true
    wait "$agent_pid" 2>/dev/null || true
  fi
  # Never recurse outside the exact freshly generated temp prefix.
  case "$smoke_dir" in
    "$tmp_parent"/masterdrive-smoke.*) rm -rf -- "$smoke_dir" ;;
    *) echo 'Skipped cleanup: unexpected temp path.' >&2 ;;
  esac
}
trap cleanup EXIT

mkdir -m 700 "$smoke_dir/export" "$smoke_dir/state"
printf 'MasterDrive local mesh smoke fixture\n' > "$smoke_dir/export/hello.txt"
token="$(od -An -tx1 -N32 /dev/urandom | tr -d ' \n')"
binary="$smoke_dir/masterdrive"
(cd "$repo_root" && go build -o "$binary" ./cmd/masterdrive)

MASTERDRIVE_AGENT_TOKEN="$token" "$binary" agent \
  -listen "$agent_addr" -export "smoke=$smoke_dir/export" \
  >"$smoke_dir/agent.out" 2>"$smoke_dir/agent.err" &
agent_pid=$!

agent_url="http://$agent_addr"
ready=false
for _ in {1..60}; do
  if printf 'header = "Authorization: Bearer %s"\n' "$token" | \
    curl --config - --noproxy '*' --silent --fail --max-time 1 "$agent_url/v1/shares" \
    >"$smoke_dir/agent-shares.json"; then
    ready=true
    break
  fi
  if ! kill -0 "$agent_pid" 2>/dev/null; then break; fi
  sleep 0.1
done
if [[ "$ready" != true ]]; then
  echo 'Agent did not become available on loopback.' >&2
  exit 1
fi

register_output="$("$binary" add-host -data-dir "$smoke_dir/state" -name smoke-node -url "$agent_url")"
host_id="$(printf '%s\n' "$register_output" | sed -n 's/^Added .*: \([0-9a-f]*\)$/\1/p')"
if [[ ! "$host_id" =~ ^[0-9a-f]{32}$ ]]; then
  echo 'Could not parse registered host ID.' >&2
  exit 1
fi
token_map="$(printf '{"%s":"%s"}' "$host_id" "$token")"

# A coprocess pipe keeps the one-time session fragment out of log files.
coproc CONTROLLER {
  exec env MASTERDRIVE_PEER_TOKENS_JSON="$token_map" "$binary" controller \
    -listen "$controller_addr" -data-dir "$smoke_dir/state" -web-dir "$repo_root/web" \
    2>"$smoke_dir/controller.err"
}
controller_pid="$CONTROLLER_PID"
IFS= read -r launch_line <&"${CONTROLLER[0]}" || true
IFS= read -r _explanation <&"${CONTROLLER[0]}" || true
session_token="${launch_line##*#session=}"
if [[ "$launch_line" != *'#session='* || ! "$session_token" =~ ^[A-Za-z0-9_-]{40,}$ ]]; then
  echo 'Controller did not provide a valid ephemeral session.' >&2
  exit 1
fi

controller_url="http://$controller_addr"
ready=false
for _ in {1..60}; do
  status="$(curl --noproxy '*' --silent --output /dev/null --write-out '%{http_code}' --max-time 1 "$controller_url/api/v1/hosts" || true)"
  if [[ "$status" == 401 ]]; then ready=true; break; fi
  if ! kill -0 "$controller_pid" 2>/dev/null; then break; fi
  sleep 0.1
done
if [[ "$ready" != true ]]; then
  echo 'Controller did not enforce a local session.' >&2
  exit 1
fi

cookie_jar="$smoke_dir/cookies"
activation_status="$(printf 'request = "POST"\nheader = "X-MasterDrive-Session: %s"\ncookie-jar = "%s"\n' \
  "$session_token" "$cookie_jar" | \
  curl --config - --noproxy '*' --silent --output /dev/null --write-out '%{http_code}' --max-time 5 \
    "$controller_url/api/v1/session")"
if [[ "$activation_status" != 204 ]]; then
  echo 'Local browser session activation failed.' >&2
  exit 1
fi

curl --noproxy '*' --silent --show-error --fail --max-time 5 --cookie "$cookie_jar" \
  "$controller_url/api/v1/hosts" > "$smoke_dir/hosts.json"
curl --noproxy '*' --silent --show-error --fail --max-time 5 --cookie "$cookie_jar" \
  "$controller_url/api/v1/hosts/$host_id/shares" > "$smoke_dir/shares.json"
curl --noproxy '*' --silent --show-error --fail --max-time 5 --cookie "$cookie_jar" \
  "$controller_url/api/v1/hosts/$host_id/shares/smoke/entries" > "$smoke_dir/entries.json"
curl --noproxy '*' --silent --show-error --fail --max-time 5 --cookie "$cookie_jar" \
  "$controller_url/api/v1/hosts/$host_id/shares/smoke/download?path=hello.txt" \
  -o "$smoke_dir/downloaded.txt"

if ! grep -q 'smoke-node' "$smoke_dir/hosts.json" ||
   ! grep -q '"smoke"' "$smoke_dir/shares.json" ||
   ! grep -q 'hello.txt' "$smoke_dir/entries.json" ||
   ! cmp -s "$smoke_dir/export/hello.txt" "$smoke_dir/downloaded.txt"; then
  echo 'Read-only mesh smoke assertions failed.' >&2
  exit 1
fi

echo 'PASS: authenticated local agent, registry, controller, session, listing, and download.'
