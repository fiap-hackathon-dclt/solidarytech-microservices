package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

const validEvent = `{"id":7,"ngo_id":1,"amount":10.56,"donor_name":"Teste","status":"APPROVED","created_at":"2026-11-28T13:44:28Z"}`

type fakeNGO struct {
	cause string
	err   error
}

func (f fakeNGO) Cause(context.Context, int) (string, error) {
	return f.cause, f.err
}

type fakeRecorder struct {
	err    error
	calls  int
	day    string
	cause  string
	amount string
}

func (f *fakeRecorder) Record(_ context.Context, ev DonationEvent, day, cause string) error {
	f.calls++
	f.day, f.cause, f.amount = day, cause, ev.Amount.String()
	return f.err
}

func message(body string) types.Message {
	return types.Message{MessageId: aws.String("m1"), Body: aws.String(body)}
}

func TestProcess(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		ngo        fakeNGO
		recordErr  error
		wantDelete bool
		wantCalls  int
		wantCause  string
	}{
		{"ONG existe", validEvent, fakeNGO{cause: "Educação"}, nil, true, 1, "Educação"},
		{"ONG inexistente soma como desconhecida", validEvent, fakeNGO{err: ErrNGONotFound}, nil, true, 1, unknownCause},
		{"ngo-service fora do ar mantém a mensagem", validEvent, fakeNGO{err: errors.New("timeout")}, nil, false, 0, ""},
		{"duplicata é apagada sem somar de novo", validEvent, fakeNGO{cause: "Educação"}, ErrAlreadyRecorded, true, 1, "Educação"},
		{"falha no DynamoDB mantém a mensagem", validEvent, fakeNGO{cause: "Educação"}, errors.New("throttled"), false, 1, "Educação"},
		{"JSON ilegível mantém a mensagem", `{"id":`, fakeNGO{cause: "Educação"}, nil, false, 0, ""},
		{"evento sem valor mantém a mensagem", `{"id":7,"ngo_id":1,"created_at":"2026-11-28T13:44:28Z"}`, fakeNGO{cause: "Educação"}, nil, false, 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &fakeRecorder{err: tt.recordErr}
			c := NewConsumer(nil, "fila", tt.ngo, rec)

			if got := c.process(context.Background(), message(tt.body)); got != tt.wantDelete {
				t.Errorf("apagar = %v, esperado %v", got, tt.wantDelete)
			}
			if rec.calls != tt.wantCalls {
				t.Errorf("gravações = %d, esperado %d", rec.calls, tt.wantCalls)
			}
			if tt.wantCause != "" && rec.cause != tt.wantCause {
				t.Errorf("causa = %q, esperado %q", rec.cause, tt.wantCause)
			}
		})
	}
}

func TestProcessKeepsExactAmount(t *testing.T) {
	rec := &fakeRecorder{}
	c := NewConsumer(nil, "fila", fakeNGO{cause: "Educação"}, rec)
	c.process(context.Background(), message(validEvent))

	if rec.amount != "10.56" {
		t.Errorf("valor = %q, esperado o texto original 10.56", rec.amount)
	}
}

func TestDayInSaoPaulo(t *testing.T) {
	tests := map[string]string{
		"2026-11-28T02:59:00Z": "2026-11-27",
		"2026-11-28T03:00:00Z": "2026-11-28",
	}
	for utc, want := range tests {
		ts, err := time.Parse(time.RFC3339, utc)
		if err != nil {
			t.Fatal(err)
		}
		if got := DayInSaoPaulo(ts); got != want {
			t.Errorf("DayInSaoPaulo(%s) = %s, esperado %s", utc, got, want)
		}
	}
}

func TestStalled(t *testing.T) {
	c := NewConsumer(nil, "fila", nil, nil)
	if c.Stalled(heartbeatLimit) {
		t.Error("consumidor recém-criado não deveria estar parado")
	}

	c.heartbeat.Store(time.Now().Add(-2 * heartbeatLimit).Unix())
	if !c.Stalled(heartbeatLimit) {
		t.Error("consumidor sem batimento há 2 minutos deveria estar parado")
	}
}
