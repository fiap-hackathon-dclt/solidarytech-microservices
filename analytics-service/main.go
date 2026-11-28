package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

type Config struct {
	Port     string
	Region   string
	QueueURL string
	Table    string
	NGOURL   string
}

func main() {
	os.Exit(run())
}

func run() int {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg, err := loadConfig()
	if err != nil {
		slog.Error("configuração inválida", "erro", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()

	awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(cfg.Region))
	if err != nil {
		slog.Error("erro ao carregar configuração AWS", "erro", err)
		return 1
	}

	store := &DynamoStore{db: dynamodb.NewFromConfig(awsCfg), table: cfg.Table}
	consumer := NewConsumer(
		sqs.NewFromConfig(awsCfg),
		cfg.QueueURL,
		&HTTPNGOClient{baseURL: cfg.NGOURL, http: &http.Client{Timeout: 3 * time.Second}},
		store,
	)

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           routes(consumer, store),
		ReadHeaderTimeout: 5 * time.Second,
	}

	consumerDone := make(chan struct{})
	go func() {
		consumer.Run(ctx)
		close(consumerDone)
	}()

	serverErr := make(chan error, 1)
	go func() {
		slog.Info("analytics-service rodando", "porta", cfg.Port)
		serverErr <- server.ListenAndServe()
	}()

	exitCode := 0
	select {
	case <-ctx.Done():
		slog.Info("sinal de parada recebido")
	case err := <-serverErr:
		slog.Error("servidor HTTP parou", "erro", err)
		exitCode = 1
		stop()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("erro ao desligar o servidor HTTP", "erro", err)
	}
	<-consumerDone
	slog.Info("analytics-service encerrado")
	return exitCode
}

func loadConfig() (Config, error) {
	cfg := Config{
		Port:     os.Getenv("PORT"),
		Region:   os.Getenv("AWS_REGION"),
		QueueURL: os.Getenv("AWS_SQS_URL"),
		Table:    os.Getenv("AWS_DYNAMODB_TABLE"),
		NGOURL:   os.Getenv("NGO_SERVICE_URL"),
	}
	if cfg.Port == "" {
		cfg.Port = "8084"
	}

	required := map[string]string{
		"AWS_REGION":         cfg.Region,
		"AWS_SQS_URL":        cfg.QueueURL,
		"AWS_DYNAMODB_TABLE": cfg.Table,
		"NGO_SERVICE_URL":    cfg.NGOURL,
	}
	var missing []string
	for name, value := range required {
		if value == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return cfg, fmt.Errorf("variáveis obrigatórias ausentes: %s", strings.Join(missing, ", "))
	}
	return cfg, nil
}
