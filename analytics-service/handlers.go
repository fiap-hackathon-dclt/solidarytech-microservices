package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

const heartbeatLimit = 60 * time.Second

type HealthChecker interface {
	Stalled(limit time.Duration) bool
}

type Totals interface {
	CauseTotals(ctx context.Context, day string) ([]CauseTotal, error)
	NGOTotals(ctx context.Context, ngoID int) ([]DayTotal, error)
}

func routes(health HealthChecker, totals Totals) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		if health.Stalled(heartbeatLimit) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "parado", "service": "analytics-service"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "analytics-service"})
	})

	mux.HandleFunc("GET /totais/causas", func(w http.ResponseWriter, r *http.Request) {
		day := r.URL.Query().Get("dia")
		if day == "" {
			day = DayInSaoPaulo(time.Now())
		} else if _, err := time.Parse(time.DateOnly, day); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "dia deve estar no formato AAAA-MM-DD"})
			return
		}
		causes, err := totals.CauseTotals(r.Context(), day)
		if err != nil {
			internalError(w, "erro ao ler os totais por causa", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"dia": day, "causas": causes})
	})

	mux.HandleFunc("GET /totais/ongs/{id}", func(w http.ResponseWriter, r *http.Request) {
		ngoID, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id da ONG deve ser numérico"})
			return
		}
		days, err := totals.NGOTotals(r.Context(), ngoID)
		if err != nil {
			internalError(w, "erro ao ler os totais da ONG", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ngo_id": ngoID, "dias": days})
	})

	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("erro ao escrever a resposta", "erro", err)
	}
}

func internalError(w http.ResponseWriter, msg string, err error) {
	slog.Error(msg, "erro", err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Erro interno"})
}
