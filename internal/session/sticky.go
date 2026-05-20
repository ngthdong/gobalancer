package session

import (
	"hash/fnv"
	"net/http"

	"github.com/ngthdong/gobalancer/internal/constant"
	"github.com/ngthdong/gobalancer/internal/pool"
)

type Sticky struct {
	cookieName string
}

func NewSticky(cookieName string) *Sticky {
	if cookieName == "" {
		cookieName = constant.DefaultCookieName
	}
	return &Sticky{cookieName: cookieName}
}

func (s *Sticky) SelectBackend(
	r *http.Request,
	backends []*pool.Backend,
	fallback func([]*pool.Backend) *pool.Backend,
) *pool.Backend {
	if cookie, err := r.Cookie(s.cookieName); err == nil {
		if backend := s.backendFromCookie(cookie.Value, backends); backend != nil {
			return backend
		}
	}
	return fallback(backends)
}

func (s *Sticky) SetCookie(w http.ResponseWriter, backend *pool.Backend) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.cookieName,
		Value:    backend.Addr,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Sticky) CookieName() string {
	return s.cookieName
}

func (s *Sticky) backendFromCookie(value string, backends []*pool.Backend) *pool.Backend {
	for _, b := range backends {
		if b.Addr == value && b.IsAvailable() {
			return b
		}
	}

	h := fnv.New32a()
	h.Write([]byte(value))
	hash := h.Sum32()

	available := make([]*pool.Backend, 0, len(backends))
	for _, b := range backends {
		if b.IsAvailable() {
			available = append(available, b)
		}
	}
	if len(available) == 0 {
		return nil
	}

	return available[hash%uint32(len(available))]
}
