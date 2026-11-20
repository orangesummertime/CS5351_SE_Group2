#!/usr/bin/env python3
"""Self-tests for the test-approval gate.

These build temporary repositories. They do not approve the course work
and they do not write records into the project tree.
"""
from __future__ import annotations

import importlib.util
import os
import subprocess
import sys
import tempfile
import textwrap
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parent / "test-approval.py"
GIT_ENV = {
    "GIT_AUTHOR_NAME": "Test",
    "GIT_AUTHOR_EMAIL": "test@example.com",
    "GIT_COMMITTER_NAME": "Test",
    "GIT_COMMITTER_EMAIL": "test@example.com",
}


def git(repo: Path, *args: str) -> str:
    env = os.environ.copy()
    env.update(GIT_ENV)
    result = subprocess.run(
        ["git", "-C", str(repo), *args],
        check=True,
        text=True,
        capture_output=True,
        env=env,
    )
    return result.stdout.strip()


def init_repo() -> Path:
    repo = Path(tempfile.mkdtemp(prefix="approval-gate-"))
    subprocess.run(["git", "init", "-q", "-b", "main", str(repo)], check=True)
    git(repo, "config", "user.email", "test@example.com")
    git(repo, "config", "user.name", "Test")
    git(repo, "config", "commit.gpgsign", "false")
    return repo


def write(repo: Path, rel: str, text: str) -> None:
    path = repo / rel
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text)


def commit_all(repo: Path, message: str) -> str:
    git(repo, "add", "-A")
    git(repo, "commit", "-q", "-m", message)
    return git(repo, "rev-parse", "HEAD")


def approval_text(
    *,
    requirement_ids: list[str],
    plan: str,
    commit: str,
    reviewer: str = "Owner D",
    reviewer_github: str = "owner-d",
    owner: str = "Owner E",
    owner_github: str = "owner-e",
    tests: list[str] | None = None,
) -> str:
    lines = ["requirement_ids:"]
    lines.extend(f"  - {item}" for item in requirement_ids)
    lines.append(f"plan: {plan}")
    if tests:
        lines.append("tests:")
        lines.extend(f"  - {item}" for item in tests)
    lines.extend(
        [
            f"owner: {owner}",
            f"owner_github: {owner_github}",
            "decision: APPROVED",
            f"approved_commit: {commit}",
            f"reviewer: {reviewer}",
            f"reviewer_github: {reviewer_github}",
        ]
    )
    return "\n".join(lines) + "\n"


def load_gate(repo: Path):
    os.chdir(repo)
    spec = importlib.util.spec_from_file_location(f"approval_gate_{repo.name}", SCRIPT)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


def run_gate(repo: Path, base: str) -> subprocess.CompletedProcess[str]:
    env = os.environ.copy()
    env.update(GIT_ENV)
    env["GITHUB_BASE_SHA"] = base
    env.pop("GITHUB_EVENT_PATH", None)
    env.pop("GITHUB_ACTIONS", None)
    return subprocess.run(
        [sys.executable, str(SCRIPT)],
        cwd=repo,
        env=env,
        text=True,
        capture_output=True,
    )


class TestApprovalGate(unittest.TestCase):
    def test_bound_approval_of_ancestor_with_plan_passes(self) -> None:
        repo = init_repo()
        write(repo, "docs/plans/plan.md", "plan\n")
        plan_commit = commit_all(repo, "plan")
        write(repo, "src/internal/ai/chat.go", "package ai\n")
        write(
            repo,
            "docs/evidence/test-approvals/ok.yaml",
            approval_text(requirement_ids=["FR-05"], plan="docs/plans/plan.md", commit=plan_commit),
        )
        commit_all(repo, "implement")
        result = run_gate(repo, plan_commit)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_stale_unrelated_approval_fails(self) -> None:
        repo = init_repo()
        write(repo, "docs/plans/old.md", "old plan\n")
        plan_commit = commit_all(repo, "old plan")
        write(
            repo,
            "docs/evidence/test-approvals/old.yaml",
            approval_text(requirement_ids=["FR-01"], plan="docs/plans/old.md", commit=plan_commit),
        )
        base = commit_all(repo, "historical approval")
        write(repo, "src/internal/ai/chat.go", "package ai\n\nfunc Chat() {}\n")
        commit_all(repo, "unrelated implementation")
        result = run_gate(repo, base)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("not part of this diff", result.stdout + result.stderr)
        self.assertNotIn("accepted approval", result.stdout)

    def test_self_approval_fails(self) -> None:
        repo = init_repo()
        write(repo, "docs/plans/plan.md", "plan\n")
        plan_commit = commit_all(repo, "plan")
        write(repo, "src/internal/ai/chat.go", "package ai\n")
        write(
            repo,
            "docs/evidence/test-approvals/self.yaml",
            approval_text(
                requirement_ids=["FR-05"],
                plan="docs/plans/plan.md",
                commit=plan_commit,
                reviewer="Owner E",
                reviewer_github="owner-e",
                owner="Owner E",
                owner_github="owner-e",
            ),
        )
        commit_all(repo, "self approved implementation")
        result = run_gate(repo, plan_commit)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("self-approval", result.stdout + result.stderr)

    def test_non_ancestor_approved_commit_fails(self) -> None:
        repo = init_repo()
        write(repo, "README.md", "base\n")
        base = commit_all(repo, "base")
        git(repo, "checkout", "-q", "-b", "side")
        write(repo, "docs/plans/plan.md", "side plan\n")
        side = commit_all(repo, "side plan")
        git(repo, "checkout", "-q", "main")
        write(repo, "src/internal/ai/chat.go", "package ai\n")
        write(
            repo,
            "docs/evidence/test-approvals/side.yaml",
            approval_text(requirement_ids=["FR-05"], plan="docs/plans/plan.md", commit=side),
        )
        commit_all(repo, "implementation pointing at a side commit")
        result = run_gate(repo, base)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("not an ancestor", result.stdout + result.stderr)

    def test_ancestor_missing_plan_file_fails(self) -> None:
        repo = init_repo()
        write(repo, "README.md", "base\n")
        base = commit_all(repo, "base without the plan")
        write(repo, "docs/plans/plan.md", "plan added later\n")
        write(repo, "src/internal/ai/chat.go", "package ai\n")
        write(
            repo,
            "docs/evidence/test-approvals/late.yaml",
            approval_text(requirement_ids=["FR-05"], plan="docs/plans/plan.md", commit=base),
        )
        commit_all(repo, "approval of a commit that lacks the plan")
        result = run_gate(repo, base)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("does not contain", result.stdout + result.stderr)

    def test_comment_mention_does_not_bind_historical_approval(self) -> None:
        repo = init_repo()
        write(repo, "docs/plans/plan.md", "plan\n")
        plan_commit = commit_all(repo, "plan")
        write(
            repo,
            "docs/evidence/test-approvals/scoped.yaml",
            approval_text(requirement_ids=["FR-05"], plan="docs/plans/plan.md", commit=plan_commit),
        )
        base = commit_all(repo, "approval already on the base")
        write(repo, "src/internal/ai/chat.go", textwrap.dedent("""\
            package ai

            // FR-05 freshness stamp.
            func Chat() {}
            """))
        commit_all(repo, "change inside the approved requirement")
        result = run_gate(repo, base)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("not part of this diff", result.stdout + result.stderr)
        self.assertNotIn("accepted approval", result.stdout)

    def _historical_unrelated(self) -> tuple[Path, str]:
        """Base contains an unrelated approval; HEAD changes other code.

        requirements.yaml is already on the base, so this change does not
        edit requirement definitions. The changed Go file is a source of
        FR-05 only. Diff text still mentions FR-01 and FR-02.
        """
        repo = init_repo()
        write(repo, "src/internal/adapters/aichat/budget_test.go", "package aichat\n")
        baseline = commit_all(repo, "baseline test")
        write(repo, "docs/req/requirements.yaml", textwrap.dedent("""\
            requirements:
              - id: FR-01
                source:
                  - "../../src/internal/adapters/aichat/protocol.go"
                related: ["FR-02"]
              - id: FR-02
                source:
                  - "../../src/web/src/features/ai/chatContext.ts"
                related: ["FR-01"]
              - id: FR-05
                source:
                  - "../../src/internal/freshness/freshness.go"
                related: ["FR-01", "FR-02", "FR-03"]
            """))
        write(
            repo,
            "docs/evidence/test-approvals/kefan.yaml",
            approval_text(
                requirement_ids=["FR-01", "FR-02"],
                plan="src/internal/adapters/aichat/budget_test.go",
                commit=baseline,
                owner="Kefan CHEN",
                owner_github="orangesummertime",
                reviewer="Owner D",
                reviewer_github="owner-d",
            ),
        )
        base = commit_all(repo, "historical unrelated approval")
        write(repo, "src/internal/freshness/freshness.go", textwrap.dedent("""\
            package freshness

            // Fixture mentions that must not widen scope: FR-01 FR-02 FR-03.
            func Assess() {}
            """))
        write(repo, "docs/plans/e-ci-gates.md", "See FR-01, FR-02, and FR-03.\n")
        write(repo, "scripts/ci/test_approval_gate.py", 'requirement_ids = ["FR-01", "FR-02"]\n')
        commit_all(repo, "implementation with incidental requirement ids")
        return repo, base

    def test_unrelated_historical_approval_does_not_pass(self) -> None:
        repo, base = self._historical_unrelated()
        gate = load_gate(repo)
        files = gate.changed_files(base)
        implementation = [path for path in files if gate.is_implementation(path)]
        scope = gate.changed_requirement_scope(base, implementation)
        self.assertIn("FR-05", scope)
        self.assertNotIn("FR-01", scope)
        self.assertNotIn("FR-02", scope)
        self.assertTrue(
            gate.commit_has_path(
                gate.parse_record(repo / "docs/evidence/test-approvals/kefan.yaml")["approved_commit"],
                "src/internal/adapters/aichat/budget_test.go",
            )
        )
        result = run_gate(repo, base)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("not part of this diff", result.stdout + result.stderr)
        self.assertNotIn("accepted approval", result.stdout)

    def test_readme_ref_mention_does_not_accept_unrelated_approval(self) -> None:
        repo, base = self._historical_unrelated()
        write(repo, "README.md", "Refs: FR-02\n")
        commit_all(repo, "mention FR-02 in the readme")
        gate = load_gate(repo)
        files = gate.changed_files(base)
        implementation = [path for path in files if gate.is_implementation(path)]
        scope = gate.changed_requirement_scope(base, implementation)
        self.assertNotIn("FR-01", scope)
        self.assertNotIn("FR-02", scope)
        self.assertNotIn("docs/evidence/test-approvals/kefan.yaml", files)
        result = run_gate(repo, base)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertNotIn("accepted approval", result.stdout)

    def test_in_diff_approval_must_match_owned_requirement(self) -> None:
        repo = init_repo()
        write(repo, "src/internal/freshness/freshness_test.go", "package freshness\n")
        baseline = commit_all(repo, "baseline test")
        write(repo, "docs/req/requirements.yaml", textwrap.dedent("""\
            requirements:
              - id: FR-01
                source:
                  - "../../src/internal/adapters/aichat/protocol.go"
              - id: FR-05
                source:
                  - "../../src/internal/freshness/freshness.go"
            """))
        base = commit_all(repo, "registry")
        write(repo, "src/internal/freshness/freshness.go", "package freshness\n\nfunc Assess() {}\n")
        write(
            repo,
            "docs/evidence/test-approvals/wrong.yaml",
            approval_text(
                requirement_ids=["FR-01", "FR-02"],
                plan="src/internal/freshness/freshness_test.go",
                commit=baseline,
                owner="Kefan CHEN",
                owner_github="orangesummertime",
                reviewer="Owner D",
                reviewer_github="owner-d",
            ),
        )
        commit_all(repo, "unrelated approval added beside the change")
        wrong = run_gate(repo, base)
        self.assertNotEqual(wrong.returncode, 0, wrong.stdout + wrong.stderr)
        self.assertIn("do not intersect", wrong.stdout + wrong.stderr)

        write(
            repo,
            "docs/evidence/test-approvals/wrong.yaml",
            approval_text(
                requirement_ids=["FR-05"],
                plan="src/internal/freshness/freshness_test.go",
                commit=baseline,
                owner="Owner E",
                owner_github="owner-e",
                reviewer="Owner D",
                reviewer_github="owner-d",
            ),
        )
        commit_all(repo, "relevant approval")
        right = run_gate(repo, base)
        self.assertEqual(right.returncode, 0, right.stdout + right.stderr)

    def test_this_pr_rejects_unrelated_baseline_approval(self) -> None:
        project = SCRIPT.parents[1]
        gate = load_gate(project)
        base = gate.git("merge-base", "main", "HEAD")
        files = gate.changed_files(base)
        implementation = [path for path in files if gate.is_implementation(path)]
        scope = gate.changed_requirement_scope(base, implementation)
        self.assertNotIn("FR-01", scope, sorted(scope))
        self.assertNotIn("FR-02", scope, sorted(scope))
        self.assertIn("FR-05", scope, sorted(scope))
        baseline = "8fd03bf8c1c881520d239962732deb3e94ec5061"
        test_file = "src/internal/adapters/aichat/budget_test.go"
        self.assertTrue(gate.commit_has_path(baseline, test_file))
        self.assertFalse(
            gate.approval_bound(
                "docs/evidence/test-approvals/unrelated.yaml",
                {"requirement_ids": ["FR-01", "FR-02"], "plan": test_file},
                files,
                scope,
                True,
            )
        )


if __name__ == "__main__":
    unittest.main()
