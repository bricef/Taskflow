# TaskFlow Risk Register

This document records security risks that have been identified, assessed, and either mitigated or accepted.

## Accepted Risks

### RISK-001: API key stored in browser localStorage

**Severity:** Medium
**Status:** Accepted
**Date:** 2026-03-31

**Description:** The web dashboard stores the user's API key in `localStorage`, which persists across browser sessions. This makes the key accessible to any JavaScript running on the same origin (XSS), browser extensions, and anyone with physical access to the device.

**Rationale for acceptance:** The dashboard is a convenience view, primarily read-only. Requiring re-authentication on every browser session significantly degrades usability. The risk is mitigated by:
- The dashboard serves no mutation UI — all changes go through the CLI, TUI, or API directly
- The API key is scoped to a single user in a single-user system
- The server is typically accessed over a private network or via an auth proxy (Authelia)
- Security headers (X-Frame-Options DENY, X-Content-Type-Options nosniff) reduce XSS attack surface

**Mitigation if risk profile changes:** Switch to `sessionStorage` (cleared on browser close) or implement a short-lived token exchange flow.

---

### RISK-002: SSE authentication via query parameter

**Severity:** Medium
**Status:** Accepted
**Date:** 2026-03-31

**Description:** The SSE endpoints (`/boards/{slug}/events`, `/events`) accept API keys via the `?token=` query parameter as a fallback because the browser `EventSource` API cannot set custom headers.

**Risks:**
- Query parameters appear in proxy/server logs
- Browser history retains URLs with tokens
- Referrer headers may leak tokens to external sites

**Rationale for acceptance:** The `EventSource` API has no mechanism to set `Authorization` headers. This is a known limitation shared by all SSE-based systems. The risk is mitigated by:
- The `Referrer-Policy: no-referrer` header prevents token leakage via referrer
- SSE connections are long-lived (token appears in logs once, not per request)
- The server is typically behind HTTPS (tokens encrypted in transit)
- Query parameter auth is only needed for browser-based SSE; CLI/TUI use header auth

**Mitigation if risk profile changes:** Implement a token exchange endpoint (`POST /auth/sse-token`) that issues short-lived tokens (5-10 min) for SSE connections, or switch to WebSocket transport which supports headers.

---

### RISK-003: Audit log relies on application-level immutability

**Severity:** Low
**Status:** Accepted
**Date:** 2026-03-31

**Description:** The audit log table has no database-level constraints preventing UPDATE or DELETE. Immutability is enforced only by the application (the service layer only INSERTs, never UPDATEs or DELETEs audit records). An attacker with direct database access could modify audit entries.

**Exception:** `AuditUpdateTaskRef` updates the `board_slug` and `task_num` fields during task reassignment between boards. This is the only legitimate UPDATE path.

**Rationale for acceptance:** This is a single-user system where the operator has full database access by design. Database-level triggers to enforce immutability would add complexity without practical security benefit. In a multi-user deployment, this should be revisited with database-level ACLs.

---

### RISK-004: Integer overflow in task numbering

**Severity:** Low
**Status:** Accepted
**Date:** 2026-03-31

**Description:** Task numbers auto-increment per board as Go `int` (64-bit). No validation prevents exceeding the maximum value. Creating 2^63 tasks on a single board would cause silent overflow.

**Rationale for acceptance:** Exceeding 2^63 tasks is physically impossible in any realistic scenario.

---

## Mitigated Risks

| Risk | Severity | Mitigation | Commit |
|------|----------|------------|--------|
| Batch path traversal | High | Reject paths without `/` prefix or containing `..` | 5c6b803 |
| Webhook SSRF | High | Validate URL scheme, block private IPs in production | 5c6b803 |
| Missing security headers | High | X-Frame-Options, X-Content-Type-Options, Referrer-Policy | b37b7f2 |
| Board detail memory exhaustion | Medium | Cap task expansion at 500 | b37b7f2 |
| Global query memory exhaustion | Medium | Stop iterating at 1000 results, cap query length at 500 chars | 5c6b803, b37b7f2 |
| Batch method abuse | Medium | Whitelist GET/POST/PUT/PATCH/DELETE | b37b7f2 |
| No rate limiting | Medium | Per-key 50/s (authenticated), per-IP 30/min (public) via chi/httprate | 0306722, b37b7f2 |
| Idempotency key collision | Low | Scoped per Authorization header | b37b7f2 |
| Sort field injection | Low | Validated at HTTP boundary against whitelist | 3cb97ee |
| FTS5 syntax errors | Low | Query wrapped in double quotes | 3cb97ee |
| No HTTP server timeouts | Info | Read 30s, Write 60s, Idle 120s (configurable) | 3cb97ee |
| No request body limits | Info | 1 MB default via MaxBytesReader (configurable) | 3cb97ee |

## Deferred / Not Applicable

| Risk | Assessment | Reason |
|------|-----------|--------|
| API key timing attack | Not exploitable | SHA-256 hash compared via SQL WHERE (constant-time by nature) |
| Dashboard publicly accessible | By design | Static HTML SPA pattern; API calls require auth |
| Actor names in admin stats | By design | Admin-only endpoint, single-user system |
| Health/OpenAPI endpoints public | By design | Standard practice for load balancers and API consumers |
| Sub-resources of deleted tasks return empty lists | Low | Empty list leaks no data; adding existence checks costs an extra DB round trip per request for no practical security gain |
| Error messages expose JSON parse details | Low risk | No sensitive information in parse error messages |
