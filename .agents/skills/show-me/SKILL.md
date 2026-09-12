---
name: show-me
description: Show the current implementation or state for the user to review, using real observable evidence when an existing run entrypoint is available.
---

# Show Me

只读展示当前 implementation 或状态供用户 review。不要修改源码、测试或配置；不要为了演示臆造新的入口。

## Steps

1. **Identify**：查找项目已有的运行脚本、CLI、dev server、browser 入口或现有产物，选择最接近用户可观察行为的入口。
2. **Run**：优先实际运行并捕获可见结果；UI 使用已有 browser/Playwright 入口，CLI 使用已有命令。没有安全、明确的运行入口时，回退到 `git status`、相关 diff、静态配置或已有产物，并明确未完成真实运行验证。
3. **Report**：按以下固定格式在对话中汇报，不默认写报告文件：

   ```text
   展示方式：
   实际执行的命令/入口：
   实际可见结果：
   产物或 URL：
   未验证项与限制：
   ```

需要启动临时进程或目录时使用项目已有的隔离和清理方式；展示结束后清理本次创建的资源。

## 出口

用户获得可复核的实际结果，或获得明确的静态证据与“未真实运行”的限制说明；不以“构建成功”或推测结果代替展示。
