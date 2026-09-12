---
name: tdd-implement
description: "完成已确认的 spec/ticket 的 test-first/TDD 交付闭环。"
disable-model-invocation: true
---

# TDD Implement

`seam` + `red-green` 是本技能的领衔词。它把一个 spec 或 task issue 编排成三个交付阶段，并在 Verify 后执行一次非阶段的 Finalize 收尾；TDD 的红-绿语义、测试质量和 mock 边界以 [tdd 技能](.agents/skills/tdd/SKILL.md) 为唯一事实源，本技能只定义交付编排。

本技能是 **Long-Horizon Skill**：阶段按顺序连续执行，并自带 **Turn Continuity** 与 **Chunking**。术语见 `CONTEXT.md`，技能设计规则见 `docs/agents/skill-design.md`。

## 入口与分支

- **单 issue**：单个 `.scratch/<feature>/spec.md`、等价 spec 或 `Type: task` issue，按下方三个 Steps 完成验证，再执行 Finalize 收尾。
- **多 issue**：`.scratch/<feature>/issues/` 下存在多个 `Type: task` 文件时，先读取 [orchestration.md](references/orchestration.md)，按 `Blocked by` 构建 DAG、Kahn 分层，再由主代理按层串行完成各 issue。
- `Type: research`、`prototype`、`grilling` 分流到对应技能，不进入本技能。

多 issue 的 A0-A5 是编排控制活动，不是额外的产品交付阶段：依赖图、分层、串行调度、层收敛、全量收敛和回退/冲突处理的详规只在 [orchestration.md](references/orchestration.md) 中维护。

## 三阶段 Steps

按序执行；每步达到可验证出口条件后立即进入下一步。每步只读取自己的轻量 reference，避免在每个阶段重复注入完整 `stages.md`。

| Step | Reference | 做什么 | 出口条件 |
|---|---|---|---|
| ① **Contract** | [contract.md](references/contract.md) | 读取入口，提取 Acceptance Criteria，建立 Scope Ledger、Preflight、验证矩阵和 Behavior/Seam 边界 | 需求无待决歧义，验证命令已确定；知道做什么、从哪里验证、什么不做 |
| ② **Red-Green** | [red-green.md](references/red-green.md) | 以 Behavior 为粒度执行有效 Red → 最小 Green → formatter/typecheck → 最小相关测试 | 所有 Behaviors 均有有效 Red、实现全绿，formatter/typecheck 和最小相关测试通过 |
| ③ **Verify** | [verify.md](references/verify.md) | 运行当前 issue 影响范围测试、必要 build、要求的真实运行验证；在最终 diff 稳定后调用一次 [code-review](.agents/skills/code-review/SKILL.md) | 最终 diff 的相关证据通过，真实运行验证完成（如要求），code-review 已完成且无 blocking finding |

## Finalize（非阶段）

Verify 出口满足后读取 [finalize.md](references/finalize.md) 并立即收尾。Finalize 不计入交付阶段，只负责必要的 docs/README 对齐、直接创建当前 issue 的独立 commit 与 Tracker/progress 更新；不执行额外安全扫描、staged diff 复核或 commit message 门禁。若发现实现、测试或文档证据不完整，回到对应阶段修复后再 Finalize。

Finalize 出口：commit 已创建、Acceptance Criteria 全部通过，Tracker 与工作区反映真实完成状态。

## 运行时纪律

- 三个阶段都从入口连续执行到自身出口；Verify 出口满足后立即进入 Finalize：预告下一步后立即执行；进度输出并入工具调用序列，输出后继续执行。只有合规交互点、明确的外部阻塞或阶段出口条件结束当前回合。
- 一个 seam 是公共可观察边界；一个 Behavior 是一个红-绿 cycle；一个 seam 可以包含多个 Behaviors。Seam/Behavior 的细节和 Todo 粒度只在进入 Step ② 时读取 [red-green.md](references/red-green.md)。
- 每个 issue 只在 Verify 的最终 diff 稳定后调用一次 `code-review`；审查维度、reviewer 数量、提示词和输出格式全部由 `code-review` 自己定义，`tdd-implement` 不复制这些规则。`code-review` 未完成或存在 blocking finding 时 issue 不得收敛；A3 层收敛不再次调用 review。
- 当前 issue 的范围、Acceptance Criteria、Out of Scope、测试/typecheck/build/真实运行证据和最终 commit 必须可追溯。Seam 或专项测试绿色不代表 issue 完成；三个阶段出口与 Finalize 全部满足后才可标记 `resolved`。
- 多 issue 模式中，每个 issue 只提交一个独立 commit；issue 影响范围测试在 Step ③ 执行，全仓测试由 orchestration 的 A4 在全部 issue 完成后执行一次。

## 引用

- TDD 核心规则：[tdd 技能](.agents/skills/tdd/SKILL.md)
- 测试标准：[tdd/tests.md](.agents/skills/tdd/tests.md)
- Mock 指南：[tdd/mocking.md](.agents/skills/tdd/mocking.md)
- Contract：[contract.md](references/contract.md)
- Red-Green：[red-green.md](references/red-green.md)
- Verify：[verify.md](references/verify.md)
- Review 方法：[code-review](.agents/skills/code-review/SKILL.md)
- Finalize：[finalize.md](references/finalize.md)
- 完整兼容规范：[stages.md](references/stages.md)
- 多 issue 编排：[orchestration.md](references/orchestration.md)
