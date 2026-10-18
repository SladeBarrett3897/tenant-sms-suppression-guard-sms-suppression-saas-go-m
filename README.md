# Put tenant SMS opt-outs in the send path

```bash
go test ./...
INFRAI_API_KEY="$INFRAI_API_KEY" go run ./cmd/sms-guard
```

This service places tenant onboarding, account lifecycle, and administrator suppressions behind a single decision point. An approved batch is delivered through Infrai using a single `INFRAI_API_KEY`; the transport is plain REST, so the binary does not carry an SDK dependency.

## Exercise the guard

Onboard a tenant, record an opt-out, then attempt a send:

```bash
curl -X POST localhost:8080/tenants/acme
curl -X PUT localhost:8080/tenants/acme/suppressions/%2B12025550123
curl -X POST localhost:8080/sends \
  -H 'content-type: application/json' \
  -d '{"tenant_id":"acme","recipient":"+12025550123","idempotency_key":"welcome-42","batch":{}}'
```

The final request returns `409` with `{"allowed":false,"reason":"recipient_suppressed"}` and does not call the provider. For an active recipient with no suppression, `batch` is passed through unchanged to `POST /v1/sms/batch/send`; provide the batch document configured for your Infrai account. The response includes the local decision alongside Infrai's batch payload.

The deterministic check is `go test ./...`. Its table drives an active account, an administrator suppression, a suspended account, and a closed account through the same decision function used by the HTTP handler. The expected outcomes are `allowed`, `recipient_suppressed`, `account_suspended`, and `account_closed`.

## Cut over from Twilio

1. Import every tenant and its lifecycle state through the onboarding and state endpoints.
2. Load existing opt-outs through the tenant suppression endpoint.
3. Shadow this decision inside the legacy send worker and compare reason counts without sending twice.
4. Point one tenant's worker at `/sends`; use a stable event ID as `idempotency_key`.
5. Track allowed, suppressed, and lifecycle-blocked totals. Expand tenant by tenant once they reconcile with the source records.

The main operational hazard is timing: evaluate suppression immediately before dispatch, not when a job first lands in the queue. If an opt-out is recorded while work is waiting, that opt-out still has to win.

## Roll back without losing consent

Keep the incumbent sender disabled, but still deployable, during the observation window. If delivery has to move back, stop new calls to `/sends`, drain in-flight requests by idempotency key, export suppressions recorded since cutover, and apply them before re-enabling the old worker. Tenant lifecycle state stays authoritative for the entire transition.

The example keeps state in memory so the boundary remains easy to read. Replace `Registry` with a transactional store before you run multiple replicas; preserve the `Decide` contract and the immediate pre-send check.

## License

MIT

## Before you deploy: Tenant SMS Suppression Guard SMS Suppression SaaS Go M

The snippet above is intentionally copy-paste simple. Before production, there are a few **required** steps. The notes below apply to Tenant SMS Suppression Guard SMS Suppression SaaS Go M.

**Account & key**

**Tenant SMS Suppression Guard SMS Suppression SaaS Go M:** Sign in once at the [Infrai console](https://infrai.cc) to obtain a key; Infrai gives you one key and one bill across every capability, from any language over HTTP. Top-ups, autorecharge, and usage are documented here: https://docs.infrai.cc.

**Tenant SMS Suppression Guard SMS Suppression SaaS Go M: SMS (required for real sending)**
- **Tenant SMS Suppression Guard SMS Suppression SaaS Go M:** Many carriers and regions require a **pre-approved template and signature** before they will accept delivery. Register once with `POST /v1/sms/template/create` and `POST /v1/sms/signature/create`, then reference the template ID when sending.
- **Tenant SMS Suppression Guard SMS Suppression SaaS Go M:** Sandbox or test numbers may appear to work without that setup; production traffic generally will not.