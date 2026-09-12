---
name: grill-to-spec
description: "Router：编排 grill-with-docs → to-spec，把模糊想法打磨成可执行 Spec。Use when the user asks to grill/design/polish an idea into a spec——只产出领域文档与 spec，不写代码。"
disable-model-invocation: true
---

# Grill to Spec

只做两个上游 skill 的编排：先把想法打磨成共识，再把共识发布成 spec。本 skill 不写代码、不修改源码或测试。

## 流程

### ① 形成共识

调用 [`grill-with-docs`](.agents/skills/grill-with-docs/SKILL.md)，由 `grilling` 与 `domain-modeling` 完成采访、术语和设计决策。

- glossary 按上游规则 inline 更新；
- 只有确需 ADR 时才创建 ADR 草稿；
- ADR 必须先展示完整草稿，用户明确确认后才写入，未确认不得落盘。

出口：用户确认共识已达成，且所有 ADR 草稿都已获得单独确认或明确不写入。

### ② 发布 spec

将已确认的共识交给 [`to-spec`](.agents/skills/to-spec/SKILL.md)，完成代码库理解、seam 提案和 spec 组装。

- 将 seam 提案并入最终 spec 草稿，不单独制造一次重复确认；
- 发布前展示完整 spec 草稿，用户一次明确确认后才写入 `.scratch/<feature-slug>/spec.md`；
- 发布时使用 `ready-for-agent`，格式细则只读取 [`references/rules.md`](references/rules.md)。

出口：spec 已发布，路径、状态和未纳入范围已报告。

## 回合连续性

本 skill 是 Long-Horizon Skill。阶段 ① 达到出口后立即进入阶段 ②；正常的阶段切换、进度汇报或“接下来生成 spec”不是回合终点。仅在必须获得用户确认的 ADR/spec 草稿、明确外部阻塞、用户主动停止或整个 skill 出口时暂停。确认完成后在同一任务链继续推进到下一可验证出口，不要求用户额外回复“继续”。

## 本 skill 独有门禁

- ADR：草稿 → 用户确认 → 落盘，任何情况不例外；
- spec：共识与 seam 合并为一个最终草稿，只设置一次发布前确认；
- 全程不写代码、不修改测试、不执行实现。

## 异常

- 用户放弃或没有可形成 spec 的主题时终止；
- issue tracker 未配置时报告配置阻塞，不绕过发布；
- 用户改变已确认的设计时回到 ①，不在 ② 静默扩大范围。
