package logging

import (
	"context"
	"os"

	"github.com/blendle/zapdriver"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func newProductionEncoderConfig() zapcore.EncoderConfig {
	cfg := zap.NewProductionEncoderConfig()

	cfg.TimeKey = "time"
	cfg.LevelKey = "severity"
	cfg.MessageKey = "message"
	cfg.EncodeTime = zapcore.RFC3339NanoTimeEncoder

	return cfg
}

func newGCPConfig() zap.Config {
	cfg := zap.NewProductionConfig()
	cfg.Level.SetLevel(zap.ErrorLevel)
	cfg.EncoderConfig = newProductionEncoderConfig()

	return cfg
}

func New(ctx context.Context) (*zap.Logger, error) {
	// https://cloud.google.com/run/docs/container-contract#services-env-vars
	if os.Getenv("K_SERVICE") == "" {
		return zap.NewDevelopment()
	}
	cfg := newGCPConfig()
	trace := ForContext(ctx)
	logger, err := cfg.Build()
	if err != nil {
		return nil, err
	}
	if trace != nil {
		fields := zapdriver.TraceContext(trace.TraceID, trace.SpanID, trace.Sampled, os.Getenv("PROJECT"))
		logger = logger.With(fields...)
	}
	return logger, nil
}
