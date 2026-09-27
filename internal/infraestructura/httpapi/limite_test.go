package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLimitarPorIPCortaElExceso(t *testing.T) {
	h := limitarPorIP(2, time.Minute)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	pedir := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/afiliaciones", nil)
		req.RemoteAddr = "203.0.113.10:1234"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	if rec := pedir(); rec.Code != http.StatusNoContent {
		t.Fatalf("1: codigo = %d", rec.Code)
	}
	if rec := pedir(); rec.Code != http.StatusNoContent {
		t.Fatalf("2: codigo = %d", rec.Code)
	}
	if rec := pedir(); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("3: codigo = %d, se esperaba 429", rec.Code)
	}
}

func TestLimitarPorIPNoMezclaDirecciones(t *testing.T) {
	h := limitarPorIP(1, time.Minute)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	reqA := httptest.NewRequest(http.MethodPost, "/", nil)
	reqA.RemoteAddr = "203.0.113.1:1"
	recA := httptest.NewRecorder()
	h.ServeHTTP(recA, reqA)

	reqB := httptest.NewRequest(http.MethodPost, "/", nil)
	reqB.RemoteAddr = "203.0.113.2:1"
	recB := httptest.NewRecorder()
	h.ServeHTTP(recB, reqB)

	if recA.Code != http.StatusNoContent || recB.Code != http.StatusNoContent {
		t.Fatalf("A=%d B=%d, cada IP tiene su cupo", recA.Code, recB.Code)
	}
}
