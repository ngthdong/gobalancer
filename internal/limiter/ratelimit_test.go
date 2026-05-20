package limiter_test

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ngthdong/gobalancer/internal/limiter"
)

func TestRateLimiter_FirstRequestAllowed(t *testing.T) {
	rl := limiter.NewRateLimiter(1, 5)

	if !rl.Allow("127.0.0.1") {
		t.Fatal("expected first request to be allowed")
	}
}

func TestRateLimiter_BurstCapacity(t *testing.T) {
	rl := limiter.NewRateLimiter(1, 3)

	ip := "127.0.0.1"

	for i := 0; i < 3; i++ {
		if !rl.Allow(ip) {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}

	if rl.Allow(ip) {
		t.Fatal("expected request to be rate limited")
	}
}

func TestRateLimiter_RefillTokens(t *testing.T) {
	rl := limiter.NewRateLimiter(2, 2)

	ip := "127.0.0.1"

	if !rl.Allow(ip) {
		t.Fatal("request 1 should pass")
	}

	if !rl.Allow(ip) {
		t.Fatal("request 2 should pass")
	}

	if rl.Allow(ip) {
		t.Fatal("request 3 should be blocked")
	}

	time.Sleep(600 * time.Millisecond)

	if !rl.Allow(ip) {
		t.Fatal("expected token refill after waiting")
	}
}

func TestRateLimiter_DifferentIPs(t *testing.T) {
	rl := limiter.NewRateLimiter(1, 1)

	if !rl.Allow("1.1.1.1") {
		t.Fatal("ip1 should pass")
	}

	if !rl.Allow("2.2.2.2") {
		t.Fatal("ip2 should pass")
	}

	if rl.Allow("1.1.1.1") {
		t.Fatal("ip1 should now be limited")
	}

	if rl.Allow("2.2.2.2") {
		t.Fatal("ip2 should now be limited")
	}
}

func TestRateLimiter_ConcurrentAccess(t *testing.T) {
	rl := limiter.NewRateLimiter(1000, 1000)

	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for j := 0; j < 100; j++ {
				rl.Allow("127.0.0.1")
			}
		}()
	}

	wg.Wait()
}

func TestRateLimitMiddleware_Allow(t *testing.T) {
	rl := limiter.NewRateLimiter(1, 1)

	handler := limiter.RateLimit(rl, nil)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:12345"

	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf(
			"expected status %d got %d",
			http.StatusOK,
			rec.Code,
		)
	}
}

func TestRateLimitMiddleware_Reject(t *testing.T) {
	rl := limiter.NewRateLimiter(1, 1)

	handler := limiter.RateLimit(rl, nil)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:12345"

	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req)

	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req)

	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf(
			"expected status %d got %d",
			http.StatusTooManyRequests,
			rec2.Code,
		)
	}

	if got := rec2.Header().Get("Retry-After"); got != "1" {
		t.Fatalf(
			"expected Retry-After=1 got=%q",
			got,
		)
	}
}

func TestRateLimitMiddleware_UsesIPAddressWithoutPort(t *testing.T) {
	rl := limiter.NewRateLimiter(1, 1)

	handler := limiter.RateLimit(rl, nil)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	)

	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1.RemoteAddr = "192.168.1.10:1111"

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "192.168.1.10:2222"

	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf(
			"expected status %d got %d",
			http.StatusTooManyRequests,
			rec2.Code,
		)
	}
}
