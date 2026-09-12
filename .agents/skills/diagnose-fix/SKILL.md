---
name: diagnose-fix
description: "Complete diagnosis→fix→regression channel for bugs and performance regressions: diagnose, then fix behind observed red regression evidence. Functional bugs require a failing regression test; performance regressions may use a benchmark/perf harness with an explicit failing threshold. Use when the user says diagnose/debug/fix this, or reports something broken/throwing/failing/slow — prefer this over diagnosing-bugs when a fix is wanted, not just a diagnosis."
---

# Diagnose Fix

本 skill 只连接诊断、受保护的最小修复和回归三个阶段。诊断细节以 [`diagnosing-bugs`](.agents/skills/diagnosing-bugs/SKILL.md) 为事实源，普通功能 bug 的红绿语义以 [`tdd`](.agents/skills/tdd/SKILL.md) 为事实源；不复制两个上游的完整步骤。

## 流程

### ① 诊断

委托 `diagnosing-bugs` 完成修复前的诊断活动：建立能捕捉用户症状的 tight feedback loop，实际复现并最小化，再形成可证伪的假设并按单变量探针验证。不要依赖固定 Phase 编号；以上游当前“进入修复前必须完成的诊断出口”为事实源。

出口：反馈回路已实际变红，最小复现已确认，且导致症状的假设已有证据；此阶段不写修复代码。

### ② 受保护的最小修复

先判断 Red 证据类型：

- **功能 bug**：在正确的公共 seam 上提出回归测试边界，遵循 `tdd` 要求获得用户确认；把最小复现转成失败回归测试并实际看到 Red，再写最小修复直到 Green。
- **性能回归**：复用 `diagnosing-bugs` 的 performance branch，建立可重复的 benchmark、timing harness、query-count 或 profiler-derived threshold；必须先实际观察到该阈值失败，再做最小修复，并用同一测量方式确认转绿。不为满足形式强造一个无法代表真实性能症状的普通单元测试。

**硬门槛：**不存在已实际观察到的 **red-capable regression evidence** 时，不得写修复代码。功能 bug 的证据必须是正确 seam 上的失败回归测试；性能回归可使用带明确阈值且可重复的 benchmark/perf harness。不存在正确 seam 或可靠性能测量边界时，本身就是 finding；记录阻塞，不绕过证据直接修改。

出口：功能 bug 为 regression test Red → 最小修复 → Green；性能回归为 benchmark/perf threshold Red → 最小修复 → Green。两者都不进入 `tdd-implement` 的长流程。

### ③ 回归收尾

按 `diagnosing-bugs` 的收尾要求重跑阶段 ① 的原始、未最小化反馈回路，确认用户症状消失；功能 bug 重跑回归测试，性能回归重跑原始基准/测量；清理 `[DEBUG-...]` 探针和一次性 harness，并记录最终验证的假设。

出口：原始症状消失、对应 regression evidence 为 Green、临时诊断产物已清理。

## 回合连续性

诊断 → Red 证据 → 修复 → Green → 原始回路 → 清理在一个回合内连续推进。只有功能 bug 的 seam 确认、发现无正确 seam/测量边界、外部环境阻塞或整个阶段出口可以暂停；预告下一步后立即执行。

## 引用

- 诊断与性能分支：[`diagnosing-bugs`](.agents/skills/diagnosing-bugs/SKILL.md)
- 功能 bug 修复：[`tdd`](.agents/skills/tdd/SKILL.md)
- 负向边界：[`references/anti-patterns.md`](references/anti-patterns.md)
