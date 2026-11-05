package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"bomstudio/internal/pricing"
	"bomstudio/internal/pricing/priceclient"
	"bomstudio/internal/pricing/pricingstore"
)

type pricingWorker struct {
	runtime *pricing.Runtime
	config  pricing.RunConfig
	retry   time.Duration
}

type realPricingClock struct{}

func (realPricingClock) Now() time.Time { return time.Now() }
func (realPricingClock) Wait(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func pricingConfigFromEnv() (pricing.RunConfig, time.Duration, time.Duration, error) {
	var cfg pricing.RunConfig
	cfg.GeneratorVersion = pricing.GeneratorVersion
	cfg.DatasetVersion = os.Getenv("SIM_DATASET_VERSION")
	cfg.Seed = os.Getenv("SIM_SEED")
	var err error
	cfg.InitialAsOf, err = time.Parse(time.RFC3339, os.Getenv("SIM_AS_OF"))
	if err != nil {
		return cfg, 0, 0, fmt.Errorf("SIM_AS_OF must be explicit RFC3339 midnight UTC: %w", pricing.ErrInvalidSourceRequest)
	}
	cfg.YearDuration, err = time.ParseDuration(envOr("SIM_YEAR_DURATION", "40m"))
	if err != nil {
		return cfg, 0, 0, fmt.Errorf("SIM_YEAR_DURATION: %w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(envOr("SIM_FX_PAIRS", "[]")))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg.Pairs); err != nil {
		return cfg, 0, 0, fmt.Errorf("SIM_FX_PAIRS must contain decimal strings: %w", pricing.ErrInvalidSourceRequest)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return cfg, 0, 0, fmt.Errorf("SIM_FX_PAIRS trailing data: %w", pricing.ErrInvalidSourceRequest)
	}
	timeout, err := time.ParseDuration(envOr("PRICE_API_TIMEOUT", "5s"))
	if err != nil || timeout <= 0 {
		return cfg, 0, 0, fmt.Errorf("PRICE_API_TIMEOUT must be positive: %w", pricing.ErrInvalidSourceRequest)
	}
	retry, err := time.ParseDuration(envOr("PRICE_RETRY_INTERVAL", "1s"))
	if err != nil || retry <= 0 {
		return cfg, 0, 0, fmt.Errorf("PRICE_RETRY_INTERVAL must be positive: %w", pricing.ErrInvalidSourceRequest)
	}
	cfg = cfg.Normalize()
	return cfg, timeout, retry, cfg.Validate()
}

func configurePricing(pool *pgxpool.Pool) (*pricingWorker, error) {
	base := os.Getenv("PRICE_API_URL")
	if base == "" {
		return nil, nil
	}
	cfg, timeout, retry, err := pricingConfigFromEnv()
	if err != nil {
		return nil, err
	}
	source, err := priceclient.New(base, timeout)
	if err != nil {
		return nil, err
	}
	return &pricingWorker{runtime: pricing.NewRuntime(source, pricingstore.NewRuntime(pool), nil), config: cfg, retry: retry}, nil
}

func (w *pricingWorker) run(ctx context.Context) {
	clock := realPricingClock{}
	for ctx.Err() == nil {
		err := w.runtime.Run(ctx, w.config, clock)
		if ctx.Err() != nil {
			return
		}
		if errors.Is(err, pricing.ErrRuntimeConfigConflict) || errors.Is(err, pricing.ErrInvalidSourceRequest) {
			slog.ErrorContext(ctx, "pricing worker blocked by configuration", "err", err)
			return
		}
		if !errors.Is(err, pricing.ErrRuntimeWorkerBusy) {
			slog.WarnContext(ctx, "pricing worker paused; retry without catch-up", "err", err)
		}
		if err := clock.Wait(ctx, w.retry); err != nil {
			return
		}
	}
}
