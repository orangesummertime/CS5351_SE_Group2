# test · course-facing test entry points

This directory holds the course-facing test layout:

- [`offline/`](offline/README.md) — deterministic, no-network suites and the
  B-Txx mapping.
- [`live/`](live/README.md) — gated provider/embedding tests that require
  explicit authorization and must stay separate from offline evidence.

The actual tests remain colocated with the code (`src/internal/**/*_test.go`,
`src/web/src/**/*.test.ts`) until the implementation plan is approved. The
`test/` directories document how the course-facing commands map to those suites
and how live evidence is redacted and gated.

No credentials or provider calls are used by the default test commands.