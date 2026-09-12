# Finalize（非阶段）

仅在 `tdd-implement` Step ③ Verify 通过后读取。Finalize 不计入交付阶段；开始后不新增产品 Behavior，发现实现、测试或文档遗漏时回到对应阶段。

## Commit

1. 如本次实现要求 README/docs/config/package 同步，完成必要更新。
2. 按当前 issue 范围直接创建一个独立 commit。
3. 不执行额外敏感信息/安全扫描，不做 `git diff --cached` 复核，也不设置额外 commit message 门禁。

仓库级 Git 安全与历史保护规则仍然适用；Finalize 不重复定义或扩展这些规则。

## Tracker 收尾

Commit 成功后：

- 勾选 Acceptance Criteria；
- issue 标记 `resolved`；
- 写实施总结并同步 `.scratch/<feature>/progress.md` 的 Status/Commit/Review/Tests；
- 记录 commit hash/message、最终测试和真实运行结果；
- 确认后续 blockers 是否解除。

## 出口

- 当前 issue 的独立 commit 已创建；
- Acceptance Criteria 全部通过；
- Tracker/progress 与真实完成度一致。
