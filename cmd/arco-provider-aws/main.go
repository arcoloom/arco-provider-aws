package main

import (
	"log/slog"
	"os"

	"github.com/arcoloom/arco-provider-aws/internal/aws"
	"github.com/arcoloom/arco-provider-aws/internal/logging"
	"github.com/arcoloom/arco-provider-aws/internal/runtime"
)

var version = "dev"

func main() {
	level := slog.LevelInfo
	if os.Getenv("ARCO_PROVIDER_DEBUG") == "1" {
		level = slog.LevelDebug
	}

	logger := logging.NewLogger(level)
	service := aws.NewService(version)
	server, err := runtime.NewServer(logger, service)
	if err != nil {
		logger.Error("failed to initialize server", "error", err)
		os.Exit(1)
	}

	if err := server.ListenAndServe(); err != nil {
		logger.Error("server exited with error", "error", err)
		os.Exit(1)
	}
}
