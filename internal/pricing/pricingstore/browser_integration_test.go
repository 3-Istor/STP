package pricingstore

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"bomstudio/internal/catalog"
	"bomstudio/internal/catalog/catalogstore"
	"bomstudio/internal/httpapi"
	"bomstudio/internal/priceapi"
	"bomstudio/internal/pricing"
	"bomstudio/internal/pricing/priceclient"
)

// Explicit opt-in: starts real HTTP source, ingestion, PostgreSQL and SSE. The
// control routes exist only in this test binary, never in the production server.
func TestPricingLiveBrowsers(t *testing.T) {
	mode := os.Getenv("NLPF68_BROWSER_MODE")
	if mode == "" {
		t.Skip("set NLPF68_BROWSER_MODE=native or external for real browser integration")
	}
	if mode != "native" && mode != "external" {
		t.Fatal("invalid browser mode")
	}
	pool := runtimeDB(t)
	first, _ := runtimeOffer(t, pool)
	second, _ := runtimeOffer(t, pool)
	if _, err := pool.Exec(t.Context(), "UPDATE suppliers SET name=CASE WHEN id=(SELECT supplier_id FROM offers WHERE id=$1) THEN 'Flux Atlas' ELSE 'Flux Boréal' END", first); err != nil {
		t.Fatal(err)
	}
	var unavailable atomic.Bool
	sourceRouter := priceapi.NewRouter()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if unavailable.Load() {
			http.Error(w, "source stopped for outage test", http.StatusServiceUnavailable)
			return
		}
		sourceRouter.ServeHTTP(w, r)
	}))
	defer source.Close()
	client, err := priceclient.New(source.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	cfg := runtimeConfig()
	cfg.DatasetVersion = "browser-live-tests"
	hub := httpapi.NewPricingHub()
	defer hub.Close()
	reads := pricing.NewReadService(NewReadStore(pool), cfg)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	clock := &browserClock{now: time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC), steps: make(chan struct{})}
	results := make(chan error, 1)
	started := make(chan struct{}, 1)
	runtime := pricing.NewRuntime(client, &RuntimeStore{pool: pool}, func(state pricing.RunState) {
		hub.Publish(state)
		if state.Tick == 0 {
			started <- struct{}{}
		} else {
			select {
			case results <- nil:
			case <-ctx.Done():
			}
		}
	})
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		for ctx.Err() == nil {
			err := runtime.Run(ctx, cfg, clock)
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				if state, readErr := reads.Status(ctx); readErr == nil {
					hub.Publish(state)
				}
				select {
				case results <- err:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	defer func() { cancel(); <-workerDone }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("bootstrap did not complete")
	}
	router := httpapi.NewRouter(httpapi.Services{Catalog: catalog.NewService(catalogstore.New(pool), time.Now), Supplier: supplierService(pool), History: pricing.NewHistoryService(New(pool)), Pricing: reads, PriceSnapshots: pricing.NewSnapshotService(NewSnapshotStore(pool), cfg), PricingSignals: hub})
	mux := http.NewServeMux()
	mux.Handle("/", router)
	mux.HandleFunc("GET /api/v1/__test/info", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"first":%d,"second":%d}`, first, second)
	})
	mux.HandleFunc("POST /api/v1/__test/tick", func(w http.ResponseWriter, r *http.Request) {
		select {
		case clock.steps <- struct{}{}:
		case <-r.Context().Done():
			return
		case <-ctx.Done():
			return
		}
		select {
		case err := <-results:
			if err != nil {
				http.Error(w, "source unavailable; no committed tick", http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		case <-r.Context().Done():
		case <-ctx.Done():
		}
	})
	mux.HandleFunc("POST /api/v1/__test/source", func(w http.ResponseWriter, r *http.Request) {
		unavailable.Store(r.URL.Query().Get("available") == "false")
		w.WriteHeader(http.StatusNoContent)
	})
	var streamsMu sync.Mutex
	streams := map[uint64]context.CancelFunc{}
	var serial atomic.Uint64
	mux.HandleFunc("GET /api/v1/quotes/stream", func(w http.ResponseWriter, r *http.Request) {
		requestCtx, requestCancel := context.WithCancel(r.Context())
		id := serial.Add(1)
		streamsMu.Lock()
		streams[id] = requestCancel
		streamsMu.Unlock()
		defer func() { requestCancel(); streamsMu.Lock(); delete(streams, id); streamsMu.Unlock() }()
		router.ServeHTTP(w, r.WithContext(requestCtx))
	})
	mux.HandleFunc("POST /api/v1/__test/disconnect", func(w http.ResponseWriter, _ *http.Request) {
		streamsMu.Lock()
		for _, stop := range streams {
			stop()
		}
		streamsMu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	backend := httptest.NewServer(mux)
	defer backend.Close()
	project, err := filepath.Abs("../../../web")
	if err != nil {
		t.Fatal(err)
	}
	if mode == "native" {
		command := exec.CommandContext(ctx, "npm", "run", "test:e2e", "--", "--config=playwright.live.config.ts")
		command.Dir = project
		command.Env = append(os.Environ(), "STP_API_PROXY="+backend.URL)
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		if err := command.Run(); err != nil {
			t.Fatal("real browser pipeline:", err)
		}
		return
	}
	// An external browser runner (e.g. a preinstalled container image) reads this
	// ephemeral local endpoint. No shell command or container input is built in Go.
	finished := make(chan bool, 1)
	mux.HandleFunc("POST /api/v1/__test/finished", func(w http.ResponseWriter, r *http.Request) {
		select {
		case finished <- r.URL.Query().Get("success") == "true":
		default:
		}
		w.WriteHeader(http.StatusNoContent)
	})
	endpoint, err := os.CreateTemp("/tmp", "nlpf68-browser-endpoint-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(endpoint.Name()) }()
	if _, err := fmt.Fprint(endpoint, backend.URL); err != nil {
		t.Fatal(err)
	}
	if err := endpoint.Close(); err != nil {
		t.Fatal(err)
	}
	t.Log("external browser endpoint:", endpoint.Name())
	select {
	case passed := <-finished:
		if !passed {
			t.Fatal("external browser pipeline failed")
		}
	case <-ctx.Done():
		t.Fatal("external browser pipeline timed out")
	}

}

type browserClock struct {
	mu    sync.Mutex
	now   time.Time
	steps chan struct{}
}

func (c *browserClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.now }
func (c *browserClock) Wait(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.steps:
		c.mu.Lock()
		c.now = c.now.Add(d)
		c.mu.Unlock()
		return nil
	}
}
