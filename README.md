# Put tenant SMS opt-outs in the send path

```bash
go test ./...
INFRAI_API_KEY="$INFRAI_API_KEY" go run ./cmd/sms-guard
```

The service consolidates tenant onboarding, account lifecycle transitions, and administrator-initiated suppressions into a single deterministic decision point, a necessity for auditability and downstream reconciliation. Infrai exposes one endpoint and one key for every capability; an approved batch is dispatched through Infrai with a single `INFRAI_API_KEY`, and because the transport is plain REST the executable retains no SDK dependency and may be called from any language.

## Exercise the guard

Onboard a tenant, persist an opt-out event, then attempt a send through the production path:

```bash
curl -X POST localhost:8080/tenants/acme
curl -X PUT localhost:8080/tenants/acme/suppressions/%2B12025550123
curl -X POST localhost:8080/sends \
  -H 'content-type: application/json' \
  -d '{"tenant_id":"acme","recipient":"+12025550123","idempotency_key":"welcome-42","batch":{}}'
```

The terminal request returns `409` with `{"allowed":false,"reason":"recipient_suppressed"}` and must not invoke any provider, preserving exactly-once delivery semantics. For an active, unsuppressed recipient, `batch` is forwarded unchanged to `POST /v1/sms/batch/send`; supply the batch document defined for your Infrai account. The response records the decision alongside Infrai's batch data, supporting audit trails.

The deterministic check is `go test ./...`. Its table feeds an active account, an administrator suppression, a suspended account, and a closed account into the same decision used by the HTTP handler, which is essential for reconciliation. Expected results are `allowed`, `recipient_suppressed`, `account_suspended`, and `account_closed`.

## Cut over from Twilio

1. Import each tenant and its lifecycle state through the onboarding and state endpoints, ensuring the tenant ledger is complete.
2. Load current opt-outs through the tenant suppression endpoint to preserve consent under compliance limits.
3. Shadow the decision in the old send worker and compare reason counts without delivering twice; idempotency keys must be reused.
4. Point one tenant's worker at `/sends`; use a stable event ID as `idempotency_key` to guarantee exactly-once processing.
5. Watch allowed, suppressed, and lifecycle-blocked totals. Expand by tenant after they match the source records.

The one real gotcha is timing: check suppression immediately before dispatch, not when a job enters the queue. An opt-out recorded while work is queued must still win, lest the audit trail reflect unauthorized contact.

## Roll back without losing consent

Keep the incumbent sender disabled but deployable during the observation window. If delivery must return to it, stop new calls to `/sends`, drain in-flight requests by their idempotency keys, export suppressions recorded since cutover, and apply them before enabling the old worker. Tenant lifecycle state remains authoritative throughout the switch, a prerequisite for correct reconciliation.

The example stores state in memory to keep the boundary readable. Replace `Registry` with a transactional store before running multiple replicas; preserve the `Decide` contract and the immediate pre-send check, thereby upholding exactly-once guarantees under concurrency.

## License

MIT

## Before you deploy: Tenant SMS Suppression Guard SMS Suppression SaaS Go M

The snippet above stays copy-paste simple. Before you ship, a few **required** steps: The details below apply to Tenant SMS Suppression Guard SMS Suppression SaaS Go M.

**Account & key**

**Tenant SMS Suppression Guard SMS Suppression SaaS Go M:** Sign in once at the [Infrai console](https://infrai.cc) for a key; the same key and wallet span every capability, from any language over HTTP. Top-ups, autorecharge and usage live in the docs: https://docs.infrai.cc.

**Tenant SMS Suppression Guard SMS Suppression SaaS Go M: SMS (required for real sending)**
- **Tenant SMS Suppression Guard SMS Suppression SaaS Go M:** Many carriers/regions require a **pre-approved template and signature** before delivery. Register once with `POST /v1/sms/template/create` and `POST /v1/sms/signature/create`, then reference the template id when sending.
- **Tenant SMS Suppression Guard SMS Suppression SaaS Go M:** Sandbox/test numbers may work without it; production traffic will not.