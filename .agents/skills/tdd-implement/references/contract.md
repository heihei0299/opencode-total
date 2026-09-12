# Contract

仅在 `tdd-implement` Step ① 读取。完整跨阶段规则仍以 `stages.md` 为兼容事实源；本文件只提供 Contract 阶段运行所需内容，避免加载其它阶段。

## 操作

1. 读取 spec/task、相关 `CONTEXT.md` 与必要 ADR，并使用仓库规定的代码探索入口理解当前实现。
2. 提取每条 Acceptance Criterion，建立 Scope Ledger：`必须实现 / 明确不做 / 允许触及`。
3. 将新发现分类为：当前 Behavior 必须修复、当前 issue 新增 Behavior、后续 ticket、无关项；只有前两类进入本次实现。
4. 做一次 Preflight：记录 `HEAD`、工作区、`BASE_HEAD=$(git rev-parse HEAD)`、test/typecheck/build、可用 subagent/browser、敏感扫描、真实运行验证路径。
5. 建立一次验证矩阵，后续复用，不重复探测等价命令。
6. 定义公共 Seam 与 Behaviors：Behavior 必须映射到 Acceptance Criterion，并明确输入、可观察输出和验证层级。
7. 已确认且未变化的 seam 直接复用；只有歧义、验收缺口、范围变化、破坏性操作或互斥方案才请求用户确认。

## 出口

- Acceptance Criteria、Scope Ledger 与 Out of Scope 明确；
- 无待决需求歧义；
- 验证矩阵和真实运行路径已确定，或明确标为 `blocked/unavailable`；
- Behaviors/Seams 可追溯；
- `BASE_HEAD` 已记录。
