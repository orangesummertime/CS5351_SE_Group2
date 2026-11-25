# 015 · Implement chat freshness, tests, UI and in-repo CI gates

- **提交**：见本文件 Git 历史。
- **提交说明**：`feat: mark stored chat evidence expired after source change`
- **实际负责人／AI 工具／审查人**：刘若轩；Grok（Grok Bot，本地实现协助）；尚未经独立组员审查，建议 D 审 E。不得把本记录视为自批。
- **关联需求、ADR、PR**：FR-05、NFR-05、INV-06、INV-07；ADR-003；尚无合并批准。
- **改动文件与范围**：新增 `src/internal/freshness`；在 `src/internal/ai` 的 Chat / ChatHistory 中盖章并重算状态；SQLite 读取 B 的版本列；HTTP 重新打开；前端展示过期／不可用；`.github/workflows/ci.yml` 与 `scripts/ci/`。未修改已发布的上游迁移，也未改动摘要缓存路径。

## 解决的问题

存储的聊天回答在来源更新、删除、并发变更或查找失败后，仍可能被界面按“当前有效”理解。重新打开对话必须得到与当前内容一致的状态。

## 本次提交实际做了什么

- 生成回答时写入来源修订号和内容哈希；读取时重算 `versionStatus` / `freshness`。
- 客户端伪造的当前状态和引用会被清除。
- 查找错误或未接线的版本读取器失败关闭，不以当前有效展示。
- 界面为回答和引用显示“当前有效／已过期／来源不可用”。
- 增加确定性离线测试（含 NFR-05 固定集、更新、删除、重新打开、并发和负面案例）。
- 增加六项课程 CI 作业。GitHub 必需检查仍未启用，不在此声称已经阻断合并。

## 对项目的可观察贡献

来源变更后，用户重新打开对话时能看到旧回答和引用不再被标成当前证据。这与上游摘要引用缓存，以及 C 在生成时写入的核验状态都不同。

## 验证与审查证据

命令与结果见 `docs/evidence/e-verification.md`。独立审查、分支保护必需检查、Issue #3 勾选，以及课程指定的 OpenCode + DeepSeek V4 Flash 原始标注会话均未完成。

## 限制、未完成项与答辩说法

笔记和 D 的比较结果暂不纳入 v1。测试批准门在独立组员签署前会失败，这是预期行为。“我没有改 B 的版本列迁移，只是在读取聊天历史时对比快照。过期不等于 C 的核验失败，所以原来的 verified 标记还在，只是不能再显示成当前有效。”
