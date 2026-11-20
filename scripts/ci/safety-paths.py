#!/usr/bin/env python3
"""Contract / secret / permission checks on safety-relevant paths."""
from __future__ import annotations

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SAFETY = (
    "src/internal/ai",
    "src/internal/citation",
    "src/internal/freshness",
    "src/internal/credentials",
    "src/internal/adapters/aichat",
    "src/internal/retrieval",
    "src/internal/transport/http",
)
# High-confidence secrets, not ordinary field names like "Key" or "hasKey".
SECRET = re.compile(
    r"(-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----)|"
    r"\bsk-[A-Za-z0-9]{20,}\b|"
    r"\bAKIA[0-9A-Z]{16}\b|"
    r"(?:api[_-]?key|secret[_-]?key|access[_-]?token)\s*[:=]\s*['\"][A-Za-z0-9/+]{24,}['\"]",
    re.I,
)
BYPASS = re.compile(r"(?i)(todo:\s*skip auth|disable(?:d)? authentication|password\s*=\s*['\"][^'\"]+['\"])")


def main() -> int:
    failed = 0
    for rel in SAFETY:
        root = ROOT / rel
        if not root.exists():
            continue
        for path in root.rglob("*"):
            if not path.is_file():
                continue
            if path.suffix not in {".go", ".ts", ".tsx", ".js", ".yml", ".yaml"}:
                continue
            text = path.read_text(encoding="utf-8", errors="replace")
            rel_path = path.relative_to(ROOT)
            if SECRET.search(text):
                print(f"possible credential literal in {rel_path}")
                failed += 1
            if BYPASS.search(text):
                print(f"possible permission bypass comment or literal in {rel_path}")
                failed += 1
    if failed:
        print(f"safety-paths: {failed} finding(s)", file=sys.stderr)
        return 1
    print("safety-paths: no credential or obvious permission-bypass literals found")
    return 0


if __name__ == "__main__":
    sys.exit(main())
