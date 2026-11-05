package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"bomstudio/internal/pricing"
)

type streamStatus struct {
	mu        sync.Mutex
	state     pricing.RunState
	afterRead func()
}

func (s *streamStatus) Status(context.Context) (pricing.RunState, error) {
	s.mu.Lock()
	v := s.state
	after := s.afterRead
	s.afterRead = nil
	s.mu.Unlock()
	if after != nil {
		after()
	}
	return v, nil
}
func streamState(tick int64) pricing.RunState {
	return pricing.RunState{Tick: tick, AsOf: time.Date(2026, 11, 5+int(tick), 0, 0, 0, 0, time.UTC), Status: "ready"}
}
func readStreamEvent(t *testing.T, reader *bufio.Reader) (string, pricing.RunState) {
	t.Helper()
	event := ""
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(line, "event: ") {
			event = strings.TrimSpace(strings.TrimPrefix(line, "event: "))
		}
		if strings.HasPrefix(line, "data: ") {
			var state pricing.RunState
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &state); err != nil {
				t.Fatal(err)
			}
			return event, state
		}
	}
}
func TestPricingStreamSubscriptionGapAndReconnection(t *testing.T) {
	hub := NewPricingHub()
	defer hub.Close()
	status := &streamStatus{state: streamState(2)}
	status.afterRead = func() { hub.Publish(streamState(3)) }
	server := httptest.NewServer(pricingStream{status: status, hub: hub, writeTimeout: time.Second, heartbeat: time.Second})
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	res, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "text/event-stream" || res.Header.Get("X-Accel-Buffering") != "no" {
		t.Fatal(res.Status, res.Header)
	}
	reader := bufio.NewReader(res.Body)
	_, first := readStreamEvent(t, reader)
	_, next := readStreamEvent(t, reader)
	if first.Tick != 2 || next.Tick != 3 {
		t.Fatal("lost subscription gap", first, next)
	}
	failure := streamState(3)
	failure.Status = "unavailable"
	hub.Publish(failure)
	event, state := readStreamEvent(t, reader)
	if event != "status" || state.Tick != 3 || state.Status != "unavailable" {
		t.Fatal(event, state)
	}
	// A failed ingestion must not advance the successful event ID.
	hub.Publish(streamState(4))
	event, state = readStreamEvent(t, reader)
	if event != "tick" || state.Tick != 4 {
		t.Fatal(event, state)
	}
	cancel()
	_ = res.Body.Close()
	status.mu.Lock()
	status.state = streamState(4)
	status.mu.Unlock()
	for _, tt := range []struct {
		name, id string
		want     int
	}{{"past", "1", 200}, {"equal", "4", 200}, {"future", "5", 400}, {"negative", "-1", 400}, {"word", "oops", 400}, {"plus", "+1", 400}, {"overflow", "99999999999999999999", 400}} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
			req.Header.Set("Last-Event-ID", tt.id)
			res, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = res.Body.Close() }()
			if res.StatusCode != tt.want {
				t.Fatal(res.Status)
			}
			if tt.name == "past" {
				_, state := readStreamEvent(t, bufio.NewReader(res.Body))
				if state.Tick != 4 {
					t.Fatal("replayed old history", state)
				}
			}
			if tt.name == "equal" {
				reader := bufio.NewReader(res.Body)
				line, err := reader.ReadString('\n')
				if err != nil || line != ": connected\n" {
					t.Fatal(line, err)
				}
				hub.Publish(streamState(5))
				_, state := readStreamEvent(t, reader)
				if state.Tick != 5 {
					t.Fatal("false duplicate on reconnect", state)
				}
			}
		})
	}
}
func TestPricingHubBoundsSlowClientsAndCleansSubscriptions(t *testing.T) {
	hub := NewPricingHub()
	defer hub.Close()
	slow, unsubSlow := hub.subscribe()
	defer unsubSlow()
	fast, unsubFast := hub.subscribe()
	defer unsubFast()
	for tick := int64(0); tick < 1000; tick++ {
		hub.Publish(streamState(tick))
		if state := <-fast; state.Tick != tick {
			t.Fatal(state)
		}
	}
	if state := <-slow; state.Tick != 999 {
		t.Fatal("unbounded/stale queue", state)
	}
	unsubFast()
	unsubSlow()
	hub.mu.Lock()
	count := len(hub.clients)
	hub.mu.Unlock()
	if count != 0 {
		t.Fatal("subscriptions leaked", count)
	}
}
func TestPricingStreamRepeatedConnectionsAndShutdown(t *testing.T) {
	hub := NewPricingHub()
	status := &streamStatus{state: streamState(0)}
	server := httptest.NewServer(pricingStream{status: status, hub: hub, writeTimeout: time.Second, heartbeat: time.Second})
	defer server.Close()
	for range 40 {
		res, err := server.Client().Get(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		_, state := readStreamEvent(t, bufio.NewReader(res.Body))
		if state.Tick != 0 {
			t.Fatal(state)
		}
		_ = res.Body.Close()
	}
	res, err := server.Client().Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	hub.Close()
	if _, err := io.Copy(io.Discard, res.Body); err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		hub.mu.Lock()
		count := len(hub.clients)
		hub.mu.Unlock()
		if count == 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("subscriptions did not close")
}

type blockedStreamWriter struct {
	headers  http.Header
	deadline time.Time
}

func (w *blockedStreamWriter) Header() http.Header { return w.headers }
func (w *blockedStreamWriter) WriteHeader(int)     {}
func (w *blockedStreamWriter) SetWriteDeadline(deadline time.Time) error {
	w.deadline = deadline
	return nil
}
func (w *blockedStreamWriter) Flush() {}
func (w *blockedStreamWriter) Write([]byte) (int, error) {
	if w.deadline.IsZero() {
		return 0, errors.New("missing write deadline")
	}
	timer := time.NewTimer(time.Until(w.deadline))
	defer timer.Stop()
	<-timer.C
	return 0, os.ErrDeadlineExceeded
}
func TestPricingStreamBoundsBlockedWrite(t *testing.T) {
	hub := NewPricingHub()
	defer hub.Close()
	writer := &blockedStreamWriter{headers: make(http.Header)}
	stream := pricingStream{status: &streamStatus{state: streamState(0)}, hub: hub, writeTimeout: 10 * time.Millisecond, heartbeat: time.Second}
	done := make(chan struct{})
	go func() { defer close(done); stream.ServeHTTP(writer, httptest.NewRequest(http.MethodGet, "/", nil)) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("blocked writer was not disconnected")
	}
	hub.mu.Lock()
	count := len(hub.clients)
	hub.mu.Unlock()
	if count != 0 {
		t.Fatal("blocked client leaked")
	}
}

func TestPricingStreamUnsupportedWriterPreservesJSONErrors(t *testing.T) {
	hub := NewPricingHub()
	defer hub.Close()
	stream := pricingStream{status: &streamStatus{state: streamState(0)}, hub: hub, writeTimeout: time.Second, heartbeat: time.Second}
	rec := httptest.NewRecorder()
	stream.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	var body map[string]string
	if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("Content-Type") != "application/json" || json.Unmarshal(rec.Body.Bytes(), &body) != nil || body["error"] == "" {
		t.Fatalf("API error must remain JSON: %d %s", rec.Code, rec.Body.String())
	}
}
