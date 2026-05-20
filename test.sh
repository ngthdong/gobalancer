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

start_identity_backend() {
    local port=$1

    python3 -c "
import http.server
import socketserver

class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b'backend-$port')

    def log_message(self, *args):
        pass

with socketserver.TCPServer(('', $port), Handler) as server:
    server.serve_forever()
" >/dev/null 2>&1 &

    echo $! > "$PID_DIR/backend-$port.pid"

    log "started identity backend :$port"
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

    sleep 5

    (
        sleep 2

        log "stopping backend :9002 during load"

        stop_backend 9002
    ) &

    hey -z 5m -c 1000 "$BALANCER_URL"
}

scenario_sticky() {
    log "running sticky-session scenario"

    stop_all

    start_identity_backend 9001
    start_identity_backend 9002
    start_identity_backend 9003

    sleep 2

    COOKIE_JAR=$(mktemp)

    echo
    log "first request"

    curl -s \
        -c "$COOKIE_JAR" \
        "$BALANCER_URL"

    echo

    log "subsequent requests"

    for i in {1..5}; do
        curl -s \
            -b "$COOKIE_JAR" \
            "$BALANCER_URL"
        echo
    done

    rm -f "$COOKIE_JAR"
}

scenario_sticky_failover() {
    log "running sticky-session failover scenario"

    stop_all

    start_identity_backend 9001
    start_identity_backend 9002
    start_identity_backend 9003

    sleep 2

    COOKIE_JAR=$(mktemp)

    FIRST=$(
        curl -s \
            -c "$COOKIE_JAR" \
            "$BALANCER_URL"
    )

    echo "initial backend: $FIRST"

    PORT=$(echo "$FIRST" | grep -o '[0-9]\+$')

    stop_backend "$PORT"

    sleep 2

    echo "backend $PORT stopped"

    curl -s \
        -b "$COOKIE_JAR" \
        "$BALANCER_URL"

    echo

    rm -f "$COOKIE_JAR"
}

scenario_ratelimit() {
    log "running rate-limit scenario"

    stop_all

    start_all

    sleep 2

    success=0
    limited=0

    for i in {1..100}; do
        code=$(
            curl \
                -o /dev/null \
                -s \
                -w "%{http_code}" \
                "$BALANCER_URL"
        )

        if [[ "$code" == "429" ]]; then
            limited=$((limited + 1))
        else
            success=$((success + 1))
        fi
    done

    echo
    echo "success=$success"
    echo "limited=$limited"
}

scenario_ratelimit_recovery() {
    log "running rate-limit recovery scenario"

    stop_all

    start_all

    sleep 2

    for i in {1..100}; do
        curl -s \
            -o /dev/null \
            "$BALANCER_URL"
    done

    echo "waiting for refill..."

    sleep 3

    curl -i "$BALANCER_URL"
}

scenario_breaker() {
    log "running circuit-breaker scenario"

    stop_all

    start_500_backend 9001
    start_backend 9002

    sleep 2

    for i in {1..20}; do
        curl -s "$BALANCER_URL" >/dev/null
    done

    echo
    log "check metrics/logs for breaker=open"
}

scenario_graceful_shutdown() {
    log "running graceful shutdown scenario"

    stop_all
    start_all

    sleep 2

    log "starting load"

    hey -n 10000 -c 50 -q 100 "$BALANCER_URL" &
    HEY_PID=$!

    sleep 2

    log "sending SIGTERM to gobalancer"

    BALANCER_PID=$(pgrep -f "gobalancer" | head -n1)

    if [[ -z "$BALANCER_PID" ]]; then
        echo "gobalancer not running"
        exit 1
    fi

    kill -SIGTERM "$BALANCER_PID"

    log "waiting for load to finish..."

    wait "$HEY_PID"

    echo
    log "graceful shutdown test completed"
    log "check hey output above — Non-2xx responses should be 0"
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
    ./test.sh sticky
    ./test.sh sticky-failover
    ./test.sh ratelimit
    ./test.sh ratelimit-recovery
    ./test.sh breaker
    ./test.sh graceful
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
sticky)
    scenario_sticky
    ;;

sticky-failover)
    scenario_sticky_failover
    ;;

ratelimit)
    scenario_ratelimit
    ;;

ratelimit-recovery)
    scenario_ratelimit_recovery
    ;;

breaker)
    scenario_breaker
    ;;
graceful)
    scenario_graceful_shutdown
    ;;
*)
    usage
    exit 1
    ;;
esac