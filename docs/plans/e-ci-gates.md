# Owner E · six course blocking CI gates

- **Owner:** 刘若轩 (E).
- **Issue:** [Issue #3](https://github.com/orangesummertime/CS5351_SE_Group2/issues/3) tracks these gates. This document does **not** check those boxes.
- **In-repo workflow:** [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml), run from [`scripts/ci/`](../../scripts/ci/).

The six gates are implemented as GitHub Actions jobs that fail the workflow. That is not the same as GitHub *enforcing* them on `main`. Unauthenticated observation of this repository on 2026-11-20 (Hong Kong time) found: `main` is protected; required status checks are off with an empty context list; the only Actions workflow previously present was `Release` (tag / manual). Rulesets were empty. Finer review settings are not visible without admin access and are not asserted here.

Until the repository owner (`orangesummertime` / the admin who can edit branch protection) turns the jobs into required checks, a passing or failing workflow cannot by itself block a merge.

## Gate implementation (in-repo)

1. **Test approval.** `scripts/ci/test-approval.py` inspects the diff against the PR base (or `origin/main`). If application code under `src/` (excluding tests) or runtime prompts change, an `APPROVED` record under `docs/evidence/test-approvals/` must be added or updated in that diff. A file that already existed and is not part of this change does not count. When `docs/req/requirements.yaml` is present, the record's `requirement_ids` must also intersect requirement blocks edited in this diff or requirements whose `source` paths name a changed implementation file. Mentions in README, comments, fixtures, or `related:` lists do not create that scope. The record needs a non-author `reviewer` and an `approved_commit` that is an ancestor of HEAD and that `git ls-tree` shows already contained the referenced plan and test files. A historical approval for a different requirement, a self-approval, a non-ancestor commit, or a commit that does not contain those files fails the job. This branch itself has only a pending template; the job is expected to fail until a non-author records a real approval.
2. **Static analysis.** `scripts/ci/static-analysis.sh` runs `gofmt` on the changed Go files, builds `src/web` so the untracked `src/web/dist` tree exists for `go:embed`, then runs `go vet`, `go mod verify`, and `tsc --noEmit`. The job checks out full history. A missing pull-request base, a diff failure, or a Go diff that checks zero files fails the job. On safety-relevant paths (`src/internal/ai`, `citation`, `freshness`, `credentials`, `adapters/aichat`, `retrieval`, `transport/http`) it also runs `scripts/ci/safety-paths.py` for credential-like literals and obvious permission bypass comments.
3. **Schema.** `scripts/ci/schema.sh` converts `docs/req/requirements.yaml` and validates it against `docs/req/req-schema.json` with `ajv-cli@5 --spec=draft2020`.
4. **Semantic evaluation.** `scripts/ci/semantic-eval.sh` inspects the diff for prompt or model-configuration paths (`src/internal/ai/prompts.go`, `src/web/src/features/ai/promptTemplates.ts`, `prompts/`, `test/offline/eval-b/eval-configs.yaml`). The frozen Owner B retrieval replay does not read those files, so a change in that scope is reported unmet and fails the job instead of being passed by `go test ./replay`. If those paths are unchanged, the job records a skip and passes. This is not a substitute for NFR-02 acceptance by B.
5. **Regression.** `scripts/ci/regression.sh` runs `go test ./... -count=1` and the frontend unit tests plus `npm run build`.
6. **AI-use log.** `scripts/ci/ai-use-log.sh` fails when code or prompt paths change unless `docs/ai-use-log.yaml` is also in the same diff.

## Branch protection the admin must enable

The repository owner should set, on `main`, at least:

- Require a pull request before merging.
- Require at least one approving review from a non-author.
- Dismiss stale approvals when new commits are pushed.
- Require approval of the most recent push.
- Require conversation resolution before merging.
- Do not allow administrators to bypass these rules (or enable “do not allow bypass”).
- **Require status checks to pass** and add every job in `.github/workflows/ci.yml`: `Test approval`, `Static analysis`, `Requirements schema`, `Semantic evaluation`, `Regression`, `AI-use log`.
- Require the branch to be up to date before merging, if the host supports it.
- Optionally require review from Code Owners for safety-relevant paths, after a `CODEOWNERS` file is agreed by the group (this branch does not invent GitHub handles for every member).

Safety-critical path changes should also include an architecture decision or an explicit statement that an existing ADR still applies. The pull request template reminds authors; it is not a substitute for required reviews.

## Honest current state

| Item | State as of this branch |
| --- | --- |
| In-repo workflow files for the six gates | Added |
| Gates enforced by GitHub required checks | **No** (required status checks observed off) |
| Issue #3 boxes | Still unchecked; only check them after required checks are on and a passing run exists |
| This branch's test-approval record | Pending independent review; the test-approval job should fail until then |
