# 012 · Correct streamed completion and summary failure reports

- **Commit:** `fix: preserve AI stream and summary terminal states`; see this file's Git history.
- **Task owner:** Kefan CHEN.
- **Scope:** three concrete PR #6 follow-up findings within A-T08/A-T12/A-T18.
- **Review:** technical corrections verified; final implementation approval remains pending.

## What changed and why

A provider can emit text and usage and then disconnect before confirming completion. Previously that partial stream could become a successful service result. Limited Chat and Responses streams now require their completion marker; premature EOF returns a truncation error, retains forwarded text and cannot trigger a non-stream retry or downstream success `done`. The unbudgeted EOF compatibility path is retained.

The Chat parser previously rejected `choices: []` before reading its usage block. It now accepts a usage-only chunk after content, so a valid prompt/output/total triple is reconciled. Malformed usage remains unknown and stops further starts.

Summary generation copied its budget report before a deferred error classifier ran. A provider failure could therefore appear active, or exhausted when the last slot had been consumed. The terminal error is now classified before the returned snapshot, preserving provider-error or cancellation status and partial progress.

## Contribution and verification

The new transport cases reproduce content plus usage followed by EOF, valid usage-only chunks, malformed usage-only chunks and unbudgeted compatibility. Service and HTTP tests cover summary failures at limits 1 and 5 and cancellation/timeout classification. Focused Go tests, race tests, `go vet` and the otherwise-full regression passed; only the previously recorded upstream FFmpeg exact-duration test is excluded. Frontend source did not change, so its existing 251-test/build result is retained rather than rerun.

The fixed-input current workflow comparison was also replayed successfully; the [recorded JSON](../docs/evidence/a-budget-comparison.json) retains the original unbounded control and refreshes the six current rows, including elapsed times. The full original follow-up session is retained owner-locally and indexed in the AI-use log. No browser or live application-provider evaluation was used.

## Presentation explanation

“Receiving some text is not proof that a stream completed. I require a completion marker for a limited task and test an EOF after text and usage. I also accept the provider's separate usage-only frame. Finally, the error state must be set before copying the response report, so an exhausted slot does not hide a provider failure.”
