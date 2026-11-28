package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPNGOClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ngos/1":
			_, _ = w.Write([]byte(`{"id":1,"cause":"Educação"}`))
		case "/ngos/2":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c := &HTTPNGOClient{baseURL: srv.URL, http: srv.Client()}

	if cause, err := c.Cause(context.Background(), 1); err != nil || cause != "Educação" {
		t.Errorf("ONG 1: causa = %q, erro = %v", cause, err)
	}
	if _, err := c.Cause(context.Background(), 999); !errors.Is(err, ErrNGONotFound) {
		t.Errorf("ONG 999: erro = %v, esperado ErrNGONotFound", err)
	}
	if _, err := c.Cause(context.Background(), 2); err == nil || errors.Is(err, ErrNGONotFound) {
		t.Errorf("ONG 2: erro = %v, esperado erro de servidor", err)
	}
}
