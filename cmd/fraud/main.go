// Command fraud runs the scoring service used inside the synchronous
// authorization path.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/coigo/payment-processor-course/internal/fraud/adapter"
	"github.com/coigo/payment-processor-course/internal/fraud/infra"
	"github.com/coigo/payment-processor-course/internal/fraud/usecases"
	"github.com/coigo/payment-processor-course/internal/plataform/config"
	"github.com/coigo/payment-processor-course/internal/plataform/logging"
	"github.com/coigo/payment-processor-course/internal/plataform/metrics"
	fraudv1 "github.com/coigo/payment-processor-course/proto/gen/go/fraud/v1"
	"google.golang.org/grpc"
)

func main() {
	logger := logging.New("fraud")
	if err := run(logger); err != nil {
		logger.Error("service stopped", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	grpcAddr := config.String("GRPC_ADDR", ":9002")
	metricsAddr := config.String("METRICS_ADDR", ":9102")
	modelPath := config.String("MODEL_PATH", "")
	batchSize := config.Int("BATCH_SIZE", 32)
	batchWait := config.Duration("BATCH_WAIT", 5*time.Millisecond)

	metrics.Serve(ctx, metricsAddr, logger)

	model, err := infra.LoadModel(modelPath)
	if err != nil {
		return err
	}
	logger.Info("model loaded", slog.String("version", model.Version))

	var resolver usecases.GeoResolver
	if config.String("GEO_PROVIDER", "static") == "ipapi" {
		resolver = infra.NewHTTPGeoResolver()
		logger.Info("using ipapi.co geo resolver")
	} else {
		resolver = infra.NewStaticGeoResolver()
		logger.Info("using static geo resolver")
	}

	pipeline := usecases.NewPipeline(
		model,
		resolver,
		infra.NewHashingEmbedder(),
		infra.NewSemanticCache(10000, 24*time.Hour, 0.90),
		logger,
		batchSize,
		batchWait,
	)
	defer pipeline.Close()

	listener, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		return err
	}

	server := grpc.NewServer(grpc.UnaryInterceptor(metrics.UnaryServerInterceptor()))
	fraudv1.RegisterFraudServiceServer(server, adapter.NewServer(pipeline, logger))

	serveErr := make(chan error, 1)
	go func() {
		if err := server.Serve(listener); err != nil {
			serveErr <- err
		}
	}()

	select {
	case err := <-serveErr:
		return fmt.Errorf("fraud: grpc server: %w", err)
	case <-ctx.Done():
	}

	gracefulCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	graceful := make(chan struct{})
	go func() {
		server.GracefulStop()
		close(graceful)
	}()

	select {
	case <-graceful:
	case <-gracefulCtx.Done():
		server.Stop()
	}

	return nil
}
