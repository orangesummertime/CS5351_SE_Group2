# Observable branch protection (Owner E, 2026-11-20, Hong Kong time)

This note records what could be observed **without repository admin access**.
It is not evidence that the six course gates are enforced on GitHub.

## Commands

```bash
curl -sS https://api.github.com/repos/orangesummertime/CS5351_SE_Group2/branches/main
curl -sS https://api.github.com/repos/orangesummertime/CS5351_SE_Group2/rulesets
curl -sS https://api.github.com/repos/orangesummertime/CS5351_SE_Group2/actions/workflows
curl -sS https://api.github.com/repos/orangesummertime/CS5351_SE_Group2/branches/main/protection
```

The last URL returned HTTP 401 (`Requires authentication`) for an unauthenticated client. Fine-grained review settings therefore cannot be confirmed here.

## Observed results

- `protected`: true
- `protection.required_status_checks.enforcement_level`: `off`
- `protection.required_status_checks.contexts`: `[]`
- `protection.required_status_checks.checks`: `[]`
- Rulesets: `[]` (HTTP 200)
- Actions workflows: one workflow, `Release` at `.github/workflows/release.yml`, state `active`
- Combined commit status on `main`: `pending`, `total_count` 0

Owner A's earlier review memo mentioned additional authenticated settings (required pull request, one approving review, stale-review dismissal, last-push approval, conversation resolution, administrator enforcement). Those settings were not independently re-verified in this observation and are **not** used here as evidence of course gate compliance.

## What the admin must still do

See [CI gate plan](../plans/e-ci-gates.md). In particular, required status checks must be turned on and must include the six Course gates jobs before anyone claims that merges to `main` are blocked by CI. Do not check Issue #3 boxes until that enforcement exists and a passing run is recorded.
