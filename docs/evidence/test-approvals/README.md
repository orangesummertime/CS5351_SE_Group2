# Test-approval records

The test-approval CI gate reads YAML files in this directory. A record is valid only when a **non-author** reviewer has set `decision: APPROVED` and named the commit that contained the approved plan and tests. The record file must be added or updated in the pull request diff; a pre-existing approval is not enough. Its `requirement_ids` must match a requirement whose definition changed in that diff, or a requirement whose `source` path is one of the changed implementation files. A mention in a README, comment, or `related:` list does not count. `approved_commit` must be an ancestor of HEAD, and `git ls-tree` at that commit must list the plan and test files named in the record.

Do not self-approve. The Owner E template below is pending and is not an approval.

Copy `owner-e.pending.yaml` to a dated file, fill the reviewer fields, and point `approved_commit` at the documentation/test commit that was reviewed, which must be an ancestor of the implementation commits.
