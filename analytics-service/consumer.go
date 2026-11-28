package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"
	_ "time/tzdata"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

const (
	unknownCause   = "desconhecida"
	processTimeout = 15 * time.Second
	receiveBackoff = 5 * time.Second
)

var saoPaulo = mustLoadLocation("America/Sao_Paulo")

type DonationEvent struct {
	ID        int         `json:"id"`
	NgoID     int         `json:"ngo_id"`
	Amount    json.Number `json:"amount"`
	CreatedAt time.Time   `json:"created_at"`
}

type QueueAPI interface {
	ReceiveMessage(ctx context.Context, in *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
	DeleteMessage(ctx context.Context, in *sqs.DeleteMessageInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error)
}

type NGOClient interface {
	Cause(ctx context.Context, ngoID int) (string, error)
}

type Recorder interface {
	Record(ctx context.Context, ev DonationEvent, day, cause string) error
}

type Consumer struct {
	queue     QueueAPI
	queueURL  string
	ngos      NGOClient
	recorder  Recorder
	heartbeat atomic.Int64
}

func NewConsumer(queue QueueAPI, queueURL string, ngos NGOClient, recorder Recorder) *Consumer {
	c := &Consumer{queue: queue, queueURL: queueURL, ngos: ngos, recorder: recorder}
	c.beat()
	return c
}

func (c *Consumer) beat() {
	c.heartbeat.Store(time.Now().Unix())
}

func (c *Consumer) Stalled(limit time.Duration) bool {
	return time.Since(time.Unix(c.heartbeat.Load(), 0)) > limit
}

func (c *Consumer) Run(ctx context.Context) {
	slog.Info("consumindo a fila", "fila", c.queueURL)
	for ctx.Err() == nil {
		out, err := c.queue.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:              aws.String(c.queueURL),
			MaxNumberOfMessages:   10,
			WaitTimeSeconds:       20,
			MessageAttributeNames: []string{"All"},
		})
		c.beat()
		if err != nil {
			if ctx.Err() == nil {
				slog.Error("erro ao ler a fila", "erro", err)
				sleep(ctx, receiveBackoff)
			}
			continue
		}
		for _, m := range out.Messages {
			if ctx.Err() != nil {
				break
			}
			c.handle(ctx, m)
		}
	}
	slog.Info("consumo da fila encerrado")
}

// Processa com prazo próprio: um SIGTERM no meio não interrompe a mensagem em andamento.
func (c *Consumer) handle(ctx context.Context, m types.Message) {
	work, cancel := context.WithTimeout(context.WithoutCancel(ctx), processTimeout)
	defer cancel()

	if !c.process(work, m) {
		return
	}
	if _, err := c.queue.DeleteMessage(work, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(c.queueURL),
		ReceiptHandle: m.ReceiptHandle,
	}); err != nil {
		slog.Error("erro ao apagar a mensagem", "mensagem", aws.ToString(m.MessageId), "erro", err)
	}
}

// Devolve true quando a mensagem pode ser apagada da fila.
func (c *Consumer) process(ctx context.Context, m types.Message) bool {
	var ev DonationEvent
	if err := json.Unmarshal([]byte(aws.ToString(m.Body)), &ev); err != nil || !ev.valid() {
		slog.Error("evento ilegível", "mensagem", aws.ToString(m.MessageId), "erro", err)
		return false
	}

	cause, err := c.ngos.Cause(ctx, ev.NgoID)
	switch {
	case errors.Is(err, ErrNGONotFound):
		slog.Warn("doação para ONG inexistente", "doacao", ev.ID, "ngo_id", ev.NgoID)
		cause = unknownCause
	case err != nil:
		slog.Error("erro ao consultar a ONG", "doacao", ev.ID, "ngo_id", ev.NgoID, "erro", err)
		return false
	case cause == "":
		cause = unknownCause
	}

	err = c.recorder.Record(ctx, ev, DayInSaoPaulo(ev.CreatedAt), cause)
	switch {
	case errors.Is(err, ErrAlreadyRecorded):
		slog.Info("doação já somada", "doacao", ev.ID)
	case err != nil:
		slog.Error("erro ao gravar os totais", "doacao", ev.ID, "erro", err)
		return false
	default:
		slog.Info("doação somada", "doacao", ev.ID, "causa", cause, "valor", ev.Amount.String())
	}
	return true
}

func (ev DonationEvent) valid() bool {
	_, err := ev.Amount.Float64()
	return ev.ID > 0 && err == nil && !ev.CreatedAt.IsZero()
}

func DayInSaoPaulo(t time.Time) string {
	return t.In(saoPaulo).Format(time.DateOnly)
}

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
