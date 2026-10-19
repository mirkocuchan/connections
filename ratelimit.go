package main

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

//un cubo de fichas por visitante, más cuándo lo vimos por última vez
type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type rateLimiter struct {
	mu       sync.Mutex //protege el mapa: varias requests lo tocan a la vez
	visitors map[string]*visitor
	every    time.Duration //cada cuánto se devuelve una ficha
	burst    int           //fichas máximas del cubo
}

func newRateLimiter(every time.Duration, burst int) *rateLimiter {
	rl := &rateLimiter{
		visitors: make(map[string]*visitor),
		every:    every,
		burst:    burst,
	}
	go rl.cleanup()
	return rl
}

//allow devuelve true si esta clave todavía tiene fichas, y gasta una
func (rl *rateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, ok := rl.visitors[key]
	if !ok {
		v = &visitor{limiter: rate.NewLimiter(rate.Every(rl.every), rl.burst)}
		rl.visitors[key] = v
	}
	v.lastSeen = time.Now()
	return v.limiter.Allow()
}

//cleanup borra a los visitantes que no aparecen hace rato, si no el mapa crecería para siempre
func (rl *rateLimiter) cleanup() {
	for {
		time.Sleep(5 * time.Minute)
		rl.mu.Lock()
		for key, v := range rl.visitors {
			if time.Since(v.lastSeen) > 10*time.Minute {
				delete(rl.visitors, key)
			}
		}
		rl.mu.Unlock()
	}
}

//middleware envuelve un handler y si no quedan fichas, corta con 429
func (rl *rateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		if !rl.allow(ip) {
			w.Header().Set("Retry-After", "60")
			RespondWithError(w, http.StatusTooManyRequests, "Demasiados intentos, probá de nuevo en un minuto")
			return
		}
		next.ServeHTTP(w, r)
	})
}