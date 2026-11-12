# Deferred design and technical debt

This register records A's deliberate v1 scope cuts. None is implemented or approved as a permanent exclusion.

| ID | Deferred item | Why deferred for A v1 | Reopen condition | Owner |
| --- | --- | --- | --- | --- |
| A-D01 | Persistent per-request usage ledger | A v1 reports per-invocation usage without a SQLite migration; process restart loses the ledger. | Auditable history, crash recovery, cross-session reporting or course acceptance requires durable accounting. | A |
| A-D02 | Account-wide/global budget | Per-invocation isolation needs no cross-user coordinator and cannot cap total spending across simultaneous actions. | Product requires a common cost ceiling, multiple accounts/devices, or aggregate quota. | A + group architecture review |
| A-D03 | Hard token/currency ceiling | Provider usage usually arrives after completion and may be missing, so a pre-request hard ceiling cannot be guaranteed from it. | Provider offers preflight reservation/guaranteed maximum tokens or a reliable metering proxy; requirement explicitly demands this guarantee. | A + group architecture review |

Reopening any item requires an updated requirement, ADR, negative tests and independent review. This register is a planning artifact, not evidence of implementation.
