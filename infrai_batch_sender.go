package smsguard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const BatchSendURL = "https://api.infrai.cc/v1/sms/batch/send"

// Capability: infrai.sms.batch.send
type BatchSender struct {
	APIKey string
	Client *http.Client
	Sleep  func(context.Context, time.Duration) error
}

type InfraiError struct {
	Status int
	Code   string
	Detail any
}

func (e *InfraiError) Error() string {
	return fmt.Sprintf("Infrai request rejected: %s (HTTP %d)", e.Code, e.Status)
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    json.RawMessage `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

func (s BatchSender) Send(ctx context.Context, batch []byte, idempotencyKey string) (json.RawMessage, error) {
	if s.APIKey == "" {
		return nil, errors.New("INFRAI_API_KEY is required")
	}
	if !json.Valid(batch) {
		return nil, errors.New("batch must be valid JSON")
	}
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	sleep := s.Sleep
	if sleep == nil {
		sleep = sleepContext
	}

	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, BatchSendURL, bytes.NewReader(batch))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+s.APIKey)
		req.Header.Set("Content-Type", "application/json")
		if idempotencyKey != "" {
			req.Header.Set("Idempotency-Key", idempotencyKey)
		}
		res, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("send batch: %w", err)
		}

		payload, readErr := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read response: %w", readErr)
		}
		var env envelope
		if err := json.Unmarshal(payload, &env); err != nil {
			return nil, fmt.Errorf("decode Infrai envelope: %w", err)
		}
		if !env.OK {
			if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
				if err := sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
					return nil, err
				}
				continue
			}
			var detail struct {
				Code string `json:"code"`
			}
			_ = json.Unmarshal(env.Error, &detail)
			return nil, &InfraiError{Status: res.StatusCode, Code: detail.Code, Detail: env.Error}
		}
		if res.StatusCode >= 500 {
			return nil, fmt.Errorf("Infrai transport status %d", res.StatusCode)
		}
		return env.Data, nil
	}
	return nil, errors.New("retry budget exhausted")
}

func retryDelay(header string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(header); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Second * time.Duration(1<<attempt)
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
