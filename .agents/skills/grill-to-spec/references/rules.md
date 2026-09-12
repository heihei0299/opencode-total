# 守则：Grill-to-Spec 本地增量规则

本文件只保存 `grill-to-spec` 相对上游 `grill-with-docs` / `to-spec` 的**增量约束**。Spec 的章节、User Story 形状、Implementation Decisions 等格式全部以 `to-spec` 为唯一事实源，本文件不复制上游模板。

## Glossary 增量规则

- 懒创建：首个术语解析时才建 `CONTEXT.md`；多上下文时先确认归属，归属不清则询问。
- 只收本上下文特有术语；定义 WHAT 非 HOW，避免把 glossary 变成实现草稿。
- glossary 可按上游流程 inline 更新，不额外增加确认轮次。

## ADR 增量规则

- 只有同时满足“难逆转 / 无上下文费解 / 存在真实权衡”时才提议 ADR。
- ADR 草稿必须完整展示并获得用户明确确认后才落盘；这是本 skill 的独有硬门禁。
- 不把 ADR 当 glossary 一样静默 inline 更新。

## Spec 增量规则

- Spec 的结构与字段全部委托 `to-spec`，本文件不维护第二份模板。
- seam 提案并入最终 spec 草稿，不再制造独立的一次确认。
- 发布前只做一次最终 spec 明确确认；确认后写入并标记 `ready-for-agent`。
- 全文沿用已经确认的 glossary 词汇，并尊重所触区域既有 ADR。

## 反模式

- 不复制 `to-spec` 的章节清单、User Story 模板或 Implementation Decisions 细则。
- 不产出 Glossary / ADR / Spec 之外的额外设计文件。
- 不把本文件当逐条朗读的对话脚本；它只约束本地增量行为。
