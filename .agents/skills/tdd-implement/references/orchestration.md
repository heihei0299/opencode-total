# 多 issue 编排（按依赖分层串行）

本文件仅在 `.scratch/<feature>/issues/` 下存在多个 `Type: task` issue 时生效。单 `spec` / 单 `task` 直接按 [stages.md](stages.md) 的三个交付阶段执行，并在 Verify 后 Finalize。A0-A5 是编排控制活动，不是额外的产品交付阶段。

主代理按依赖分层、层内按编号串行执行；每个 issue 由同一个主代理完成 Contract → Red-Green → Verify，再执行非阶段 Finalize 并创建一个独立 commit。实现细节以 [stages.md](stages.md) 为准，TDD 语义以 [tdd 技能](.agents/skills/tdd/SKILL.md) 为准。

## 目录

- [A0：依赖图与编排 Preflight](#a0依赖图与编排-preflight)
- [A1：Kahn 拓扑分层](#a1kahn-拓扑分层)
- [A2：分层串行调度](#a2分层串行调度)
- [A3：层收敛](#a3层收敛)
- [A4：全量收敛](#a4全量收敛)
- [A5：回退与冲突处理](#a5回退与冲突处理)

---

## A0：依赖图与编排 Preflight

1. 扫描 `.scratch/<feature>/issues/` 下全部 `NN-<slug>.md`，逐文件解析 `Blocked by`：
   - `Blocked by: None`、`Blocked by: （无）` 或无此行：无依赖；
   - `Blocked by: 01, 02` 或 `Blocked by: 01（…）`：依赖对应编号 issue；
   - `Blocked by` 行存在但无法解析：**fail closed**。将该 issue 标记为 `blocked`，记录原始字段和值，不把它加入可调度 DAG，也不得按“无依赖”继续。只有字段修正或用户明确确认依赖后才能继续编排。
2. 以 issue 编号为节点、`Blocked by` 为有向边构建 DAG；检测到环时列出环上节点并停止调度。依赖引用了不存在的 issue 时同样 fail closed：对应 issue 保持 `blocked`，报告缺失节点，不静默忽略该依赖。
3. 读取共享 `spec.md`（若存在）、`CONTEXT.md` 和与本次改动有关的 ADR。
4. 完成编排级 Preflight：记录当前 `HEAD`、工作区状态、`BASE_HEAD=$(git rev-parse HEAD)`、测试/typecheck/build 命令、真实运行路径和敏感信息扫描脚本可用性。后续只使用已经确认的命令和路径。
5. 强制初始化 `.scratch/<feature>/progress.md`：

   ```markdown
   ## DAG
   ## Layers (Kahn L1..Ln)
   ## Progress
   | NN | Status | Commit | Review | Tests |
   |---|---|---|---|---|
   ```

   `progress.md` 是派生视图，真相源仍是 `spec.md` 与 `issues/*.md`。

### A0 出口

- 所有 `Blocked by` 字段均可解析且依赖节点存在；否则相关 issue 保持 `blocked`，A1 不开始；
- DAG 已构建且无环；
- 编排 Preflight 和 `BASE_HEAD` 已记录；
- `progress.md` 已存在并可回写；
- 依赖解析或缺失节点问题已明确报告，而不是降级成无依赖。

## A1：Kahn 拓扑分层

对 DAG 做 Kahn 分层：

```text
L1 = 全部入度为 0 的节点
L2 = 移除 L1 后入度为 0 的节点
...
Ln = 最后一层
```

每层内节点互无依赖，但仍由主代理按编号串行执行。层间必须串行。编排开始前一次性向用户展示 DAG 和 `L1..Ln`，得到确认后进入 A2；这是合规交互点，不把每个 seam 或每个 issue 的正常切换变成确认点。

### A1 出口

- Kahn 分层结果已展示并确认；
- 每个可调度 issue 都属于一个层；
- 不存在因无法解析依赖而被误放入 L1 的 issue；
- 同文件预期冲突已记录，必要时已通过依赖顺序隔离。

## A2：分层串行调度

```text
for each layer Li in L1..Ln:
  for each issue in Li（按编号顺序）:
    主代理执行三个阶段：
      ① Contract
      ② Red-Green
      ③ Verify（当前 issue 影响范围 + 当前 issue review）
    执行 Finalize（非阶段：独立 commit + Tracker 收尾）
    产出回执卡片并回写 issue
    强制更新 progress.md 的 Status/Commit/Review/Tests
  通过 A3 层收敛后进入下一层
全部层完成后进入 A4
```

每个 issue 的 Verify 只运行当前 issue 影响范围内的完整测试；全仓测试不在每个 issue 中重复执行。当前 issue 的最终 diff 稳定后只调用一次 `code-review`；review 的内部方法完全由 `code-review` 定义。修复 blocking finding 后执行受影响验证和 finding delta recheck，不重复调用完整 `code-review`。

主代理在层内和层间连续调度：一个 issue 的 Finalize 出口满足后，立即取下一个 issue，直到全部层完成或发生明确外部阻塞。进度输出并入执行序列，不在正常切换点等待用户“继续”。

进入 A2 前记录的 `BASE_HEAD` 必须在每个 issue 的三个阶段出口和 Finalize commit 前校验：

```bash
git merge-base --is-ancestor $BASE_HEAD HEAD
```

为达到工作区干净只删除本次产生的 `[DEBUG-...]` 和一次性临时产物；未经用户确认不使用 `git reset --hard`、`git checkout .`、`git clean -fd`、`git stash push --include-untracked` 或其他改写/丢弃历史的命令。

### Issue 回执卡片

每个 issue 完成后记录并回写：

```text
Issue: NN
Status: resolved
Commit: <hash> — <message>
Behaviors: <completed list>
Acceptance Criteria: <checkbox result>
Review: code-review completed once, no blocking finding
Tests: <targeted command and actual result>
Runtime: <actual request/page-visible result or not required>
Docs: <updated files or no update required>
```

### A2 出口

- 当前层每个 issue 均完成三个阶段与 Finalize 并有独立 commit；
- issue、回执卡片和 `progress.md` 一致；
- 相关测试通过，工作区卫生和历史校验通过；
- 没有未记录的跨 issue 改动。

## A3：层收敛

A3 只做编排收敛，不再次调用 `code-review`；正式 review 已在每个 issue 的 Verify 中完成。

每层全部 issue 串行完成后检查以下项目，全部通过才进入下一层：

1. 所有 issue `Status: resolved`，实施总结已落盘，`progress.md` 对应行已为 `done`；
2. 该层 issue 的相关测试通过；
3. `git status` 只显示预期改动或干净；
4. `git merge-base --is-ancestor $BASE_HEAD HEAD` 通过；
5. 不存在未分类的 scope 扩张、review blocking finding 或未清理临时产物。

任一项失败，定位到该层失败 issue，按 A5 回退并重做该 issue 的受影响阶段或 Behavior，然后重新收敛本层。

## A4：全量收敛

全部层完成且各层收敛通过后：

1. 按 A0 的验证矩阵运行一次仓库全量测试；这是多 issue 流程唯一的全量回归点。只有修复全量失败后才允许必要重跑；
2. 执行 `git merge-base --is-ancestor $BASE_HEAD HEAD`；失败时按 A5 恢复后重验；
3. 执行 `git status`，确认无 `[DEBUG-...]`、一次性脚本或未跟踪临时文件；
4. 汇总各 issue 回执卡片的 commit、Behaviors、Acceptance Criteria、测试、真实运行和文档对齐结果；汇总只在对话输出，不另写汇总文件。

### A4 出口

- 全部 issue 已有独立 commit、实施总结和 `progress.md` 派生记录；
- 全量测试通过；
- 工作区卫生、历史校验和真实运行要求均满足；
- `progress.md` 与 `issues/*.md` 一致，不一致时以 issue 真相源为准并修复派生视图。

## A5：回退与冲突处理

A5 负责所有编排级失败，不把失败静默吞掉，也不把不相关问题塞入当前 issue：

| 失败类别 | 处理 |
|---|---|
| `Blocked by` 存在但无法解析，或依赖节点不存在 | fail closed：该 issue 保持 `blocked`，保留原始依赖值并停止其调度；字段修正或用户明确确认依赖后才重新构建 DAG |
| Contract 歧义、验收缺口、范围变化 | 回到该 issue 的 Contract，补 Scope Ledger、Behavior 和验证矩阵 |
| Red-Green 的有效 Red、实现、typecheck 或 targeted test 失败 | 回到该 issue 的 Red-Green，修复当前 Behavior 并重新验证 |
| Verify 的测试、build、真实运行或 review blocking finding 失败 | 回到受影响 issue 的对应阶段；修复后只做受影响检查和 delta review |
| Finalize 的必要 docs、commit 或 Tracker 失败 | 保持 issue 未 resolved，修复 Finalize 问题后重新验证 |
| 全量测试失败 | 定位到引入失败的 issue，按上述路径修复；只在修复后重跑必要范围和全量测试 |
| `Blocked by` 依赖未完成 | 后续 issue 保持 `blocked`，前置 issue resolved 后自动解阻 |
| 多 issue 预期修改同一文件 | 记录冲突，按编号串行；无法安全归属时暂停并请求用户决定 |
| Git 历史祖先校验失败 | 立即停止写入，使用 `git reflog` 找回 `BASE_HEAD` 之后的提交，校验通过后继续 |

主代理不跨 issue 无记录改动；不通过第二次完整 `code-review` 来掩盖定向修复。外部权限、model、browser 或 tool 不可用时遵循 [stages.md](stages.md) 的 Tool Failure Budget，最多一次有依据的 fallback，仍失败则标记 `blocked/unavailable` 并报告实际状态。

### A5 出口

- 失败原因已分类并记录；
- 回退目标明确，受影响证据已重新验证；
- 冲突已按依赖顺序解决或已明确请求用户决策；
- DAG 顺序、issue 状态、commit 和 `progress.md` 保持一致。
