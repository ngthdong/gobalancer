set -e

PID_DIR="/tmp/gobalancer-demo"

mkdir -p "$PID_DIR"

start_backend() {
    local port=$1

    python3 -m http.server "$port" >/dev/null 2>&1 &

    echo $! >"$PID_DIR/backend-$port.pid"

    echo "started backend :$port"
}

stop_backend() {
    local port=$1

    if [ -f "$PID_DIR/backend-$port.pid" ]; then
        kill "$(cat "$PID_DIR/backend-$port.pid")" 2>/dev/null || true
        rm -f "$PID_DIR/backend-$port.pid"

        echo "stopped backend :$port"
    fi
}

stop_all() {
    stop_backend 9001
    stop_backend 9002
    stop_backend 9003
}

start_all() {
    start_backend 9001
    start_backend 9002
    start_backend 9003
}

scenario_basic() {
    echo "=== basic retry scenario ==="

    stop_all

    start_backend 9001
    start_backend 9002

    echo "backend :9003 intentionally down"

    sleep 1

    curl -v http://localhost:8008/
}

scenario_all_down() {
    echo "=== all backends down ==="

    stop_all

    curl -v http://localhost:8008/
}

scenario_timeout() {
    echo "=== timeout retry scenario ==="

    stop_all

    socat TCP-LISTEN:9001,fork PIPE >/dev/null 2>&1 &
    echo $! >"$PID_DIR/backend-9001.pid"

    echo "started hung backend :9001"

    start_backend 9002

    sleep 1

    time curl -v http://localhost:8008/
}

scenario_5xx() {
    echo "=== retry on 5xx scenario ==="

    stop_all

    python3 -c "
import http.server
import socketserver

class H(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(500)
        self.end_headers()
        self.wfile.write(b'error')

    def log_message(self, *a):
        pass

with socketserver.TCPServer(('', 9001), H) as s:
    s.serve_forever()
" >/dev/null 2>&1 &

    echo $! >"$PID_DIR/backend-9001.pid"

    echo "started always-500 backend :9001"

    start_backend 9002

    sleep 1

    curl -v http://localhost:8008/
}

scenario_load() {
    echo "=== load test with backend failure ==="

    stop_all

    start_all

    sleep 1

    (
        sleep 2

        echo
        echo ">>> killing backend :9002 during load"

        stop_backend 9002
    ) &

    hey -n 300 -c 30 http://localhost:8008/
}

usage() {
    echo
    echo "usage:"
    echo "  ./scripts/retry_demo.sh basic"
    echo "  ./scripts/retry_demo.sh all-down"
    echo "  ./scripts/retry_demo.sh timeout"
    echo "  ./scripts/retry_demo.sh 5xx"
    echo "  ./scripts/retry_demo.sh load"
    echo "  ./scripts/retry_demo.sh start"
    echo "  ./scripts/retry_demo.sh stop"
    echo
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