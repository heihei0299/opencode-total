# Red-Green

仅在 `tdd-implement` Step ② 读取。TDD 语义以 `.agents/skills/tdd/SKILL.md` 为唯一事实源；本文件只描述交付阶段编排。

## 操作

1. 加载 `tdd` 核心规则；每个 Behavior 只按需读取 `tdd/tests.md` / `tdd/mocking.md`。
2. Todo 以 Behavior 为粒度；一个 Behavior 是一个 `Red → Green → formatter/typecheck → 最小相关测试` cycle。
3. 有效 Red 必须从公共接口观察到“目标行为尚未实现”的断言失败；语法错误、fixture/helper 缺失、环境启动失败、timeout 或工具错误都不是有效 Red。
4. 只写让当前 Behavior Green 的最小实现；每次修改后即时 formatter/typecheck 和最小相关测试。
5. 每个 Behavior 完成后更新 Todo，然后立即进入下一个 Behavior；一个 Seam 全绿不是阶段出口。

## Turn Continuity / Chunking

- 每个 Behavior 的 Red → Green → 验证在一个回合内连续完成；预告下一步后立即执行。
- 所有 Behaviors 完成前持续推进，除非遇到合规交互点或明确外部阻塞。
- 单次 write 超过约 150 行时先骨架后分批；超过约 5 处 replace 时拆批验证。

## 出口

- 所有 Behaviors 都有有效 Red；
- 最小实现全部 Green；
- formatter/typecheck 与最小相关测试通过；
- Todo 全部反映真实完成状态；
- `BASE_HEAD` 祖先校验通过。
