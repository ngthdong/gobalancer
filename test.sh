#!/usr/bin/env bash

set -euo pipefail

PID_DIR="/tmp/gobalancer-demo"
BALANCER_URL="http://localhost:8008"

mkdir -p "$PID_DIR"

log() {
    echo "$@"
}

start_backend() {
    local port=$1

    python3 -m http.server "$port" >/dev/null 2>&1 &

    echo $! >"$PID_DIR/backend-$port.pid"

    log "started backend :$port"
}

start_hung_backend() {
    local port=$1

    socat TCP-LISTEN:"$port",fork PIPE >/dev/null 2>&1 &

    echo $! >"$PID_DIR/backend-$port.pid"

    log "started hung backend :$port"
}

start_500_backend() {
    local port=$1

    python3 -c "
import http.server
import socketserver

class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(500)
        self.end_headers()
        self.wfile.write(b'error')

    def log_message(self, *args):
        pass

with socketserver.TCPServer(('', $port), Handler) as server:
    server.serve_forever()
" >/dev/null 2>&1 &

    echo $! >"$PID_DIR/backend-$port.pid"

    log "started always-500 backend :$port"
}

stop_backend() {
    local port=$1
    local pid_file="$PID_DIR/backend-$port.pid"

    if [[ -f "$pid_file" ]]; then
        kill "$(cat "$pid_file")" 2>/dev/null || true
        rm -f "$pid_file"

        log "stopped backend :$port"
    fi
}

start_all() {
    start_backend 9001
    start_backend 9002
    start_backend 9003
}

stop_all() {
    stop_backend 9001
    stop_backend 9002
    stop_backend 9003
}

request() {
    curl -v "$BALANCER_URL"
}

scenario_basic() {
    log "running basic retry scenario"

    stop_all

    start_backend 9001
    start_backend 9002

    log "backend :9003 is down"

    sleep 1

    request
}

scenario_all_down() {
    log "running all-backends-down scenario"

    stop_all

    request
}

scenario_timeout() {
    log "running timeout retry scenario"

    stop_all

    start_hung_backend 9001
    start_backend 9002

    sleep 1

    time request
}

scenario_5xx() {
    log "running retry-on-5xx scenario"

    stop_all

    start_500_backend 9001
    start_backend 9002

    sleep 1

    request
}

scenario_load() {
    log "running load test scenario"

    stop_all

    start_all

    sleep 1

    (
        sleep 2

        log "stopping backend :9002 during load"

        stop_backend 9002
    ) &

    hey -n 5000 -c 500 "$BALANCER_URL"
}

usage() {
    cat <<EOF
usage:
  ./test.sh basic
  ./test.sh all-down
  ./test.sh timeout
  ./test.sh 5xx
  ./test.sh load
  ./test.sh start
  ./test.sh stop
EOF
}

case "${1:-}" in
basic)
    scenario_basic
    ;;
all-down)
    scenario_all_down
    ;;
timeout)
    scenario_timeout
    ;;
5xx)
    scenario_5xx
    ;;
load)
    scenario_load
    ;;
start)
    start_all
    ;;
stop)
    stop_all
    ;;
*)
    usage
    exit 1
    ;;
esac