# 014 · Claim Owner E and register freshness requirements, ADR and tests

- **提交**：见本文件 Git 历史。
- **提交说明**：`docs: claim owner E and register FR-05 freshness plan`
- **实际负责人／AI 工具／审查人**：刘若轩（GitHub @h5f8fzckp7-coder，学号 60862168）；Grok（Grok Bot，本地实现协助）；审查人未指定，建议 D 审 E。
- **关联需求、ADR、PR**：`docs/req/requirements.yaml` 中的 FR-05、NFR-05、INV-06、INV-07；[ADR-003](../docs/architecture/adr/003-citation-version-freshness.md)；[计划](../docs/plans/e-freshness-preimplementation.md)；[CI 说明](../docs/plans/e-ci-gates.md)。尚无 GitHub PR 编号。
- **改动文件与范围**：认领 README 中的 E；正式登记需求；新增 ADR、测试计划、CI 设计、待审批模板和可执行的离线新鲜度用例。不把上游摘要引用或 C 的生成时核验算作本记录的实现成果。

## 解决的问题

原版和当前 fork 会把问答历史按内容 ID 存下来，C 也会在生成时写下引用核验状态，但来源后来被更新或删除时，重新打开对话并不会把旧回答／引用标成过期或不可用。B 已提供 `content_revision` / `content_hash`，E 要在此基础上定义并验证聊天证据的新鲜度，同时补上课程要求的六项 CI 工作流（仓库内实现，不声称 GitHub 已经强制执行）。

## 本次提交实际做了什么

把 E 的负责人写成刘若轩，登记 FR-05 / NFR-05 / INV-06 / INV-07（approval 仍为 pending），写出 ADR-003、测试计划和六项 CI 的设计与工作流脚本，并加入至少 20 个固定离线案例及负面案例。需求尚未被独立组员批准。

## 对项目的可观察贡献

用户在来源变更后应能看到旧问答证据不再被当作当前有效。本记录对应的是需求、设计和可执行测试，而不是把未审查的实现当作已经验收。

## 验证与审查证据

见后续实现记录及 `docs/evidence/`。本记录本身不声称实施已被批准。

## 限制、未完成项与答辩说法

“我先把过期规则和 20 个固定案例写进仓库，新鲜度跟 C 的核验状态是分开的。重新打开时按当前 `content_revision` 和 `content_hash` 重算。六项 CI 已经写成 workflow，但分支保护还要仓库管理员打开必需检查；Issue #3 的方框不能自己勾。课程要求的 OpenCode + DeepSeek 原始会话还需要我另外补。”
