package smsguard

import (
	"errors"
	"strings"
	"sync"
)

type AccountState string

const (
	AccountActive    AccountState = "active"
	AccountSuspended AccountState = "suspended"
	AccountClosed    AccountState = "closed"
)

type Tenant struct {
	ID    string
	State AccountState
}

type SendIntent struct {
	TenantID       string
	Recipient      string
	IdempotencyKey string
	Batch          []byte
}

type Decision struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason"`
}

type Registry struct {
	mu         sync.RWMutex
	tenants    map[string]Tenant
	suppressed map[string]map[string]struct{}
}

func NewRegistry() *Registry {
	return &Registry{
		tenants:    make(map[string]Tenant),
		suppressed: make(map[string]map[string]struct{}),
	}
}

func (r *Registry) Onboard(tenantID string) error {
	if strings.TrimSpace(tenantID) == "" {
		return errors.New("tenant ID is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tenants[tenantID] = Tenant{ID: tenantID, State: AccountActive}
	return nil
}

func (r *Registry) SetAccountState(tenantID string, state AccountState) error {
	if state != AccountActive && state != AccountSuspended && state != AccountClosed {
		return errors.New("invalid account state")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tenants[tenantID]; !ok {
		return errors.New("tenant not found")
	}
	r.tenants[tenantID] = Tenant{ID: tenantID, State: state}
	return nil
}

func (r *Registry) Suppress(tenantID, recipient string) error {
	recipient = normalizeRecipient(recipient)
	if recipient == "" {
		return errors.New("recipient is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tenants[tenantID]; !ok {
		return errors.New("tenant not found")
	}
	if r.suppressed[tenantID] == nil {
		r.suppressed[tenantID] = make(map[string]struct{})
	}
	r.suppressed[tenantID][recipient] = struct{}{}
	return nil
}

func (r *Registry) Unsuppress(tenantID, recipient string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tenants[tenantID]; !ok {
		return errors.New("tenant not found")
	}
	delete(r.suppressed[tenantID], normalizeRecipient(recipient))
	return nil
}

func (r *Registry) Decide(intent SendIntent) Decision {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tenant, ok := r.tenants[intent.TenantID]
	if !ok {
		return Decision{Reason: "tenant_not_found"}
	}
	if tenant.State != AccountActive {
		return Decision{Reason: "account_" + string(tenant.State)}
	}
	if _, blocked := r.suppressed[intent.TenantID][normalizeRecipient(intent.Recipient)]; blocked {
		return Decision{Reason: "recipient_suppressed"}
	}
	return Decision{Allowed: true, Reason: "allowed"}
}

func normalizeRecipient(value string) string {
	return strings.TrimSpace(value)
}
