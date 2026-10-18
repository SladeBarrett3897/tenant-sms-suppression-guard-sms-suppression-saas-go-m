package smsguard

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestBatchSendWithoutIdempotencyKey(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.Header.Get("Idempotency-Key"); got != "" {
			t.Fatalf("Idempotency-Key = %q, want no header", got)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"ok":true,"data":{}}`)),
			Header:     make(http.Header),
		}, nil
	})}

	if _, err := (BatchSender{APIKey: "test-key", Client: client}).Send(context.Background(), []byte(`{"messages":[]}`), ""); err != nil {
		t.Fatalf("Send() with optional idempotency key omitted: %v", err)
	}
}

func TestSendDecision(t *testing.T) {
	tests := []struct {
		name       string
		state      AccountState
		suppress   bool
		wantAllow  bool
		wantReason string
	}{
		{name: "active account sends", state: AccountActive, wantAllow: true, wantReason: "allowed"},
		{name: "admin suppression blocks", state: AccountActive, suppress: true, wantReason: "recipient_suppressed"},
		{name: "suspended account blocks", state: AccountSuspended, wantReason: "account_suspended"},
		{name: "closed account blocks", state: AccountClosed, wantReason: "account_closed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewRegistry()
			if err := registry.Onboard("tenant-a"); err != nil {
				t.Fatal(err)
			}
			if err := registry.SetAccountState("tenant-a", tt.state); err != nil {
				t.Fatal(err)
			}
			if tt.suppress {
				if err := registry.Suppress("tenant-a", "+12025550123"); err != nil {
					t.Fatal(err)
				}
			}
			got := registry.Decide(SendIntent{TenantID: "tenant-a", Recipient: "+12025550123"})
			if got.Allowed != tt.wantAllow || got.Reason != tt.wantReason {
				t.Fatalf("Decide() = %+v, want allowed=%v reason=%q", got, tt.wantAllow, tt.wantReason)
			}
		})
	}
}
