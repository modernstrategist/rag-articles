package sender

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/example/aea/internal/collector"
)

// HTTPSender posts batches over HTTPS with retries and jittered backoff.
type HTTPSender struct {
	client     *http.Client
	endpoint   string
	maxRetries int
	backoff    time.Duration
}

// HTTPConfig encapsulates the parameters for the HTTPSender.
type HTTPConfig struct {
	Endpoint    string
	Timeout     time.Duration
	MaxRetries  int
	Backoff     time.Duration
	InsecureTLS bool
}

// NewHTTPSender constructs an HTTPSender with tuned Transport for low overhead.
func NewHTTPSender(cfg HTTPConfig) (*HTTPSender, error) {
	if cfg.Endpoint == "" {
		return nil, errors.New("collector endpoint is required")
	}

	transport := &http.Transport{
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: cfg.InsecureTLS}, //nolint:gosec intentionally configurable for on-prem
		DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		MaxIdleConns:          8,
		IdleConnTimeout:       90 * time.Second,
		DisableCompression:    false,
		ExpectContinueTimeout: 1 * time.Second,
	}

	client := &http.Client{
		Timeout:   cfg.Timeout,
		Transport: transport,
	}

	retries := cfg.MaxRetries
	if retries <= 0 {
		retries = 3
	}

	backoff := cfg.Backoff
	if backoff <= 0 {
		backoff = 500 * time.Millisecond
	}

	return &HTTPSender{
		client:     client,
		endpoint:   cfg.Endpoint,
		maxRetries: retries,
		backoff:    backoff,
	}, nil
}

func (s *HTTPSender) Send(ctx context.Context, batch []collector.Payload) error {
	if len(batch) == 0 {
		return nil
	}

	body, err := json.Marshal(batch)
	if err != nil {
		return err
	}

	var attempt int
	for {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(body))
		if reqErr != nil {
			return reqErr
		}
		req.Header.Set("Content-Type", "application/json")

		resp, doErr := s.client.Do(req)
		if doErr == nil && resp != nil && resp.StatusCode < 500 {
			if resp.Body != nil {
				_ = resp.Body.Close()
			}
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
			return errors.New(resp.Status)
		}
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}

		attempt++
		if attempt > s.maxRetries {
			if doErr != nil {
				return doErr
			}
			return errors.New("max retries exceeded")
		}

		sleep := s.backoff + time.Duration(attempt*100)*time.Millisecond
		select {
		case <-time.After(sleep):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
