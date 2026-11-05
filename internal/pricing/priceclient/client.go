// Package priceclient adapte le fournisseur HTTP au port Source (voir ADR 0011).
package priceclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"bomstudio/internal/pricing"
)

const maxResponseBytes = 64 << 20

type Client struct {
	base string
	http *http.Client
}

var _ pricing.Source = (*Client)(nil)

func New(base string, timeout time.Duration) (*Client, error) {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || timeout <= 0 {
		return nil, fmt.Errorf("invalid source URL or timeout: %w", pricing.ErrInvalidSourceRequest)
	}
	return &Client{base: strings.TrimRight(base, "/"), http: &http.Client{Timeout: timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (c *Client) Fetch(ctx context.Context, op pricing.SourceOperation, in pricing.SourceInput) (pricing.SourceOutput, error) {
	if err := in.Validate(op); err != nil {
		return pricing.SourceOutput{}, err
	}
	paths := map[pricing.SourceOperation]string{pricing.SourceOffer: "/quotes/offer", pricing.SourceSupplier: "/quotes/supplier", pricing.SourceCatalog: "/quotes/catalog", pricing.SourcePair: "/fx/pair", pricing.SourceFX: "/fx"}
	body, err := json.Marshal(in)
	if err != nil {
		return pricing.SourceOutput{}, fmt.Errorf("encode source input: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/api/v1"+paths[op], bytes.NewReader(body))
	if err != nil {
		return pricing.SourceOutput{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return pricing.SourceOutput{}, fmt.Errorf("fetch source: %w: %w", pricing.ErrSourceUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return pricing.SourceOutput{}, fmt.Errorf("source HTTP %d: %w", resp.StatusCode, pricing.ErrSourceUnavailable)
	}
	media, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return pricing.SourceOutput{}, fmt.Errorf("source content type: %w", pricing.ErrInvalidSourceData)
	}
	limited := &io.LimitedReader{R: resp.Body, N: maxResponseBytes + 1}
	decoder := json.NewDecoder(limited)
	decoder.DisallowUnknownFields()
	var out pricing.SourceOutput
	if err := decoder.Decode(&out); err != nil {
		return out, fmt.Errorf("decode source: %w: %w", pricing.ErrInvalidSourceData, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF || limited.N <= 0 {
		return pricing.SourceOutput{}, fmt.Errorf("trailing or oversized source response: %w", pricing.ErrInvalidSourceData)
	}
	if err := pricing.ValidateSourceOutput(op, in, out); err != nil {
		return pricing.SourceOutput{}, err
	}
	return out, nil
}
