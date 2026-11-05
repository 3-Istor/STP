package main

import (
	"testing"
	"time"
)

func TestPricingConfigurationIsExplicitAndValidated(t *testing.T) {
	for _, tt := range []struct {
		name, key, value string
		valid            bool
	}{
		{"valid", "SIM_SEED", "42", true}, {"missing date", "SIM_AS_OF", "", false},
		{"not daily", "SIM_AS_OF", "2026-01-01T00:00:01Z", false},
		{"negative speed", "SIM_YEAR_DURATION", "-1s", false},
		{"zero timeout", "PRICE_API_TIMEOUT", "0s", false},
		{"numeric FX", "SIM_FX_PAIRS", `[{"series_key":"fx:USD:EUR","base_currency":"USD","quote_currency":"EUR","reference":0.92,"band_percent":"5"}]`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SIM_DATASET_VERSION", "config-test")
			t.Setenv("SIM_SEED", "42")
			t.Setenv("SIM_AS_OF", "2026-01-01T00:00:00Z")
			t.Setenv("SIM_YEAR_DURATION", "40m")
			t.Setenv("SIM_FX_PAIRS", "[]")
			t.Setenv("PRICE_API_TIMEOUT", "5s")
			t.Setenv("PRICE_RETRY_INTERVAL", "1s")
			t.Setenv(tt.key, tt.value)
			cfg, _, _, err := pricingConfigFromEnv()
			if (err == nil) != tt.valid {
				t.Fatalf("valid=%v err=%v", tt.valid, err)
			}
			if tt.valid && cfg.YearDuration != 40*time.Minute {
				t.Fatalf("wrong speed: %s", cfg.YearDuration)
			}
		})
	}
}
