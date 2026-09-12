package httpapi

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// limitarPorIP recorta un endpoint publico por direccion remota.
//
// RealIP ya reescribio RemoteAddr cuando hay cabecera de proxy de fiar,
// asi que aqui se lee eso y no X-Forwarded-For otra vez.
func limitarPorIP(maximo int, ventana time.Duration) func(http.Handler) http.Handler {
	var (
		mu     sync.Mutex
		golpes = map[string][]time.Time{}
	)
	return func(siguiente http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := ipDe(r)
			ahora := time.Now()

			mu.Lock()
			vigentes := golpes[ip][:0]
			for _, t := range golpes[ip] {
				if ahora.Sub(t) < ventana {
					vigentes = append(vigentes, t)
				}
			}
			if len(vigentes) >= maximo {
				golpes[ip] = vigentes
				mu.Unlock()
				escribirError(w, http.StatusTooManyRequests, "demasiadas solicitudes, reintente en un momento")
				return
			}
			golpes[ip] = append(vigentes, ahora)
			mu.Unlock()

			siguiente.ServeHTTP(w, r)
		})
	}
}

func ipDe(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
