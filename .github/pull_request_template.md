## Scope

- Requirement IDs:
- Owner:
- Tests run (commands and results):
- ADR for safety-relevant paths (or why an existing ADR still applies):

## Checklist

- [ ] Tests for this change were approved by a non-author reviewer *before* implementation, or this PR is documentation/tests only.
- [ ] `docs/ai-use-log.yaml` is updated if this PR changes code or prompts.
- [ ] Negative tests are included for new behaviour.
- [ ] No credentials, secrets or local absolute paths are committed.

The six course CI jobs must be required on `main` by the repository admin before they can block a merge. See `docs/plans/e-ci-gates.md`.
