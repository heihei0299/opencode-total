# Verify

仅在 `tdd-implement` Step ③ 读取。验证必须对应当前最终 diff；产品代码或测试再次变化时，测试/typecheck/build 等受影响证据失效。

## 固定顺序

`影响范围测试 → 必要 build → 必要真实运行验证 → code-review 一次 → blocking 修复后的定向复核`

## 规则

- 单 issue / 单 spec：按 Contract 验证矩阵运行完整相关测试；多 issue：只跑当前 issue 影响范围，全仓测试留给 orchestration A4。
- ticket 要求真实运行时，优先专用 browser，其次项目已有 Playwright；HTTP/CLI 不能替代 WebUI 可见验证。
- 临时进程必须使用隔离配置/端口，记录 PID 与实际结果，结束后清理。
- 当前 issue 的最终 diff 稳定后，调用一次 [code-review](.agents/skills/code-review/SKILL.md)。`tdd-implement` 只规定调用时机；审查维度、reviewer 数量、提示词、上下文与输出格式以 `code-review` 为唯一事实源。
- `code-review` 未完成或返回 blocking finding 时，issue 保持未完成；只修当前 issue 的 blocking finding，其余按 review 结果记录。
- 修复 blocking finding 后，只重跑受影响测试/typecheck 并对该 finding 做 delta recheck；不再次调用完整 `code-review`。若修复引入新的 Behavior、改变 Scope 或使原 Review 对象不再成立，则回到 Contract/Red-Green，重新形成稳定最终 diff 后再进入 Verify。

## 出口

- 最终 diff 的相关测试/typecheck/build 通过；
- 要求的真实运行验证有实际证据；
- 当前稳定 diff 已完成一次 `code-review`；
- 无 blocking finding；
- post-review 修复（如有）的受影响验证与 finding delta recheck 已完成。
