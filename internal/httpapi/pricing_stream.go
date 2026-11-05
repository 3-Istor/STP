package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"bomstudio/internal/pricing"
)

// PricingHub carries a bounded latest-state signal, never the quote ledger.
// ADR 0012: slow clients may skip signals; REST always reads committed facts.
type PricingHub struct {
	mu      sync.Mutex
	clients map[chan pricing.RunState]struct{}
	closed  bool
	done    chan struct{}
	latest  int64
}

func NewPricingHub() *PricingHub {
	return &PricingHub{clients: map[chan pricing.RunState]struct{}{}, done: make(chan struct{}), latest: -1}
}
func (h *PricingHub) Publish(state pricing.RunState) {
	if state.Tick < 0 || state.AsOf.IsZero() {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || state.Tick < h.latest {
		return
	}
	h.latest = state.Tick
	for ch := range h.clients {
		select {
		case ch <- state:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- state:
			default:
			}
		}
	}
}
func (h *PricingHub) subscribe() (<-chan pricing.RunState, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan pricing.RunState, 1)
	if h.closed {
		close(ch)
		return ch, func() {}
	}
	h.clients[ch] = struct{}{}
	return ch, func() { h.mu.Lock(); defer h.mu.Unlock(); delete(h.clients, ch) }
}
func (h *PricingHub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	close(h.done)
	h.clients = map[chan pricing.RunState]struct{}{}
}

type pricingStatusReader interface {
	Status(context.Context) (pricing.RunState, error)
}
type pricingStream struct {
	status       pricingStatusReader
	hub          *PricingHub
	writeTimeout time.Duration
	heartbeat    time.Duration
}

func (s pricingStream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Subscribe before reading the persisted cursor, then discard older queued signals.
	states, unsubscribe := s.hub.subscribe()
	defer unsubscribe()
	last := int64(-1)
	if values := r.Header.Values("Last-Event-ID"); len(values) > 0 {
		if len(values) != 1 || len(values[0]) == 0 || len(values[0]) > 19 {
			badRequest(w, pricing.ErrInvalidReadRequest)
			return
		}
		for _, c := range values[0] {
			if c < '0' || c > '9' {
				badRequest(w, pricing.ErrInvalidReadRequest)
				return
			}
		}
		var err error
		last, err = strconv.ParseInt(values[0], 10, 64)
		if err != nil {
			badRequest(w, pricing.ErrInvalidReadRequest)
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	current, err := s.status.Status(ctx)
	cancel()
	if err != nil {
		pricingError(w, r, err)
		return
	}
	if last > current.Tick {
		badRequest(w, pricing.ErrInvalidReadRequest)
		return
	}
	controller := http.NewResponseController(w)
	if err := controller.SetWriteDeadline(time.Now().Add(s.writeTimeout)); err != nil {
		http.Error(w, "stream unavailable", http.StatusServiceUnavailable)
		return
	}
	defer func() { _ = controller.SetWriteDeadline(time.Time{}) }()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	lastMode := ""
	send := func(state pricing.RunState) error {
		if state.Tick < last {
			return nil
		}
		event := "tick"
		if state.Status != "ready" {
			event = "status"
			if state.Tick == last && lastMode == "unavailable" {
				return nil
			}
		} else if state.Tick <= last {
			return nil
		}
		data, err := json.Marshal(struct {
			Tick   int64     `json:"tick"`
			AsOf   time.Time `json:"as_of"`
			Status string    `json:"status"`
		}{state.Tick, state.AsOf, state.Status})
		if err != nil {
			return err
		}
		if err := controller.SetWriteDeadline(time.Now().Add(s.writeTimeout)); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", state.Tick, event, data); err != nil {
			return err
		}
		if err := controller.Flush(); err != nil {
			return err
		}
		last = state.Tick
		lastMode = state.Status
		return nil
	}
	if _, err := fmt.Fprint(w, ": connected\nretry: 1000\n\n"); err != nil {
		return
	}
	if err := send(current); err != nil {
		return
	}
	if err := controller.Flush(); err != nil {
		return
	}
	heartbeat := time.NewTicker(s.heartbeat)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.hub.done:
			return
		case state, ok := <-states:
			if !ok {
				return
			}
			if err := send(state); err != nil {
				return
			}
		case <-heartbeat.C:
			if err := controller.SetWriteDeadline(time.Now().Add(s.writeTimeout)); err != nil {
				return
			}
			if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return
			}
			if err := controller.Flush(); err != nil {
				return
			}
		}
	}
}
