package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fakeHealth bool

func (f fakeHealth) Stalled(time.Duration) bool {
	return bool(f)
}

type fakeTotals struct{}

func (fakeTotals) CauseTotals(context.Context, string) ([]CauseTotal, error) {
	return []CauseTotal{{Cause: "Educação", Total: "10.56", Donations: "1"}}, nil
}

func (fakeTotals) NGOTotals(context.Context, int) ([]DayTotal, error) {
	return []DayTotal{}, nil
}

func get(h http.Handler, path string) int {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Code
}

func TestRoutes(t *testing.T) {
	tests := []struct {
		name    string
		stalled bool
		path    string
		want    int
	}{
		{"saudável", false, "/health", http.StatusOK},
		{"laço parado", true, "/health", http.StatusServiceUnavailable},
		{"causas de hoje", false, "/totais/causas", http.StatusOK},
		{"causas de um dia", false, "/totais/causas?dia=2026-11-28", http.StatusOK},
		{"dia inválido", false, "/totais/causas?dia=28-11-2026", http.StatusBadRequest},
		{"totais da ONG", false, "/totais/ongs/1", http.StatusOK},
		{"id não numérico", false, "/totais/ongs/abc", http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := get(routes(fakeHealth(tt.stalled), fakeTotals{}), tt.path); got != tt.want {
				t.Errorf("GET %s = %d, esperado %d", tt.path, got, tt.want)
			}
		})
	}
}
