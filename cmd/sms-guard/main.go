package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"

	smsguard "example.com/tenant-sms-suppression"
)

type service struct {
	registry *smsguard.Registry
	sender   smsguard.BatchSender
}

type sendRequest struct {
	TenantID       string          `json:"tenant_id"`
	Recipient      string          `json:"recipient"`
	IdempotencyKey string          `json:"idempotency_key"`
	Batch          json.RawMessage `json:"batch"`
}

func main() {
	svc := &service{
		registry: smsguard.NewRegistry(),
		sender:   smsguard.BatchSender{APIKey: os.Getenv("INFRAI_API_KEY")},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /tenants/{tenant}", svc.onboard)
	mux.HandleFunc("PUT /tenants/{tenant}/state/{state}", svc.lifecycle)
	mux.HandleFunc("PUT /tenants/{tenant}/suppressions/{recipient}", svc.suppress)
	mux.HandleFunc("DELETE /tenants/{tenant}/suppressions/{recipient}", svc.unsuppress)
	mux.HandleFunc("POST /sends", svc.send)
	log.Printf("sms guard listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func (s *service) onboard(w http.ResponseWriter, r *http.Request) {
	writeMutation(w, s.registry.Onboard(r.PathValue("tenant")))
}

func (s *service) lifecycle(w http.ResponseWriter, r *http.Request) {
	err := s.registry.SetAccountState(r.PathValue("tenant"), smsguard.AccountState(r.PathValue("state")))
	writeMutation(w, err)
}

func (s *service) suppress(w http.ResponseWriter, r *http.Request) {
	writeMutation(w, s.registry.Suppress(r.PathValue("tenant"), r.PathValue("recipient")))
}

func (s *service) unsuppress(w http.ResponseWriter, r *http.Request) {
	writeMutation(w, s.registry.Unsuppress(r.PathValue("tenant"), r.PathValue("recipient")))
}

func (s *service) send(w http.ResponseWriter, r *http.Request) {
	var input sendRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	decision := s.registry.Decide(smsguard.SendIntent{
		TenantID: input.TenantID, Recipient: input.Recipient,
		IdempotencyKey: input.IdempotencyKey, Batch: input.Batch,
	})
	if !decision.Allowed {
		writeJSON(w, http.StatusConflict, decision)
		return
	}
	data, err := s.sender.Send(r.Context(), input.Batch, input.IdempotencyKey)
	if err != nil {
		var rejected *smsguard.InfraiError
		if errors.As(err, &rejected) && rejected.Status >= 400 && rejected.Status < 500 {
			writeJSON(w, rejected.Status, map[string]any{"error": rejected.Code, "decision": decision})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "delivery unavailable"})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"decision": decision, "data": data})
}

func writeMutation(w http.ResponseWriter, err error) {
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
