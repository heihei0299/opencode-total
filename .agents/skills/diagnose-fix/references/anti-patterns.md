# 反模式清单（Diagnose-Fix）

SKILL.md 正文各阶段规则是正面约束；本文件是负向边界（不做什么），为细节唯一出处。SKILL.md 只引用本文件，不重复内容。

## 诊断阶段

- 不跳过反馈回路直接猜根因：回路未红之前不进入假设、不写修复代码
- 功能 bug 不把最小复现永久留在 harness 里：必须转写为正确 seam 上的回归测试；性能回归可保留最小、可重复且有明确阈值的 benchmark/perf harness 作为回归证据
- 不一次改多个变量：探针一次只改一个，`[DEBUG-...]` 前缀标记

## 修复阶段

- 不绕过 red-capable regression evidence 直接改代码——功能 bug 没有正确 seam 是 finding；性能回归没有可靠测量边界同样是 finding，不是豁免
- 不为性能问题强造一个不能代表真实慢路径的普通单元测试；优先复用 diagnosis 中已经验证过的 benchmark、timing、query-count 或 profiler-derived threshold
- 不套用 tdd-implement 重流程：单个 bug/perf regression 的修复保持轻量，不引入完整 delivery orchestration
- 不重写 tdd 技能的红-绿语义：功能 bug 的 seam 定义、好测试标准、mocking 边界一律查上游技能

## 回归阶段

- 不遗留探针：`[DEBUG-...]` 前缀的临时改动在回归验证后全部清理
- 不跳过原始反馈回路的重跑：最小化场景绿 ≠ 原始症状消失
