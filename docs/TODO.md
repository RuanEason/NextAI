# NextAI TODO

更新时间：2026-02-19 09:39:04 +0800

## 执行约定（强制）
- 每位接手 AI 开始前，必须先阅读本文件与 `/home/ruan/.codex/handoff/latest.md`（Windows：`C:\Users\Lenovo\.codex\handoff\latest.md`）。
- 执行顺序优先遵循交接文件“接手建议（按顺序）”，并与本文件未完成项对齐推进。
- 每次执行后必须更新本文件：勾选完成项、记录阻塞原因、刷新“更新时间”。

## 0. 目标范围（v1）
- 以 `nextai-local` 功能边界为准，遵循 `openclaw` 的工程方法（契约优先、分层测试、CI 分层、CLI/Gateway 分离），不扩展超出 v1 范围能力。

## 1. 当前状态（已完成）
- [x] 基础工程：Monorepo、pnpm、核心文档、`Makefile`、`.env.example` 已统一。
- [x] 核心能力：Gateway/CLI/Web v1 主链路已完成，统一错误模型与关键安全防护已落地。
- [x] 契约体系：OpenAPI + schema + contract lint/test + SDK 生成流程已可运行。
- [x] 测试与 CI：已具备 unit/integration/e2e/contract 分层测试与 `ci-fast/ci-full/nightly-live` 门禁。
- [x] 文档交付：`docs/v1-roadmap.md`、`docs/contracts.md`、开发/部署/发布文档已齐备。

## 2. 接手建议（按顺序）
1. 先处理阻塞项：补齐交接文件 `~/.codex/handoff/latest.md`，再继续后续任务。
2. 涉及 Web 改动时，优先更新契约/文档，再改实现，并至少执行 `pnpm -C apps/web test` 与 `pnpm -C apps/web build`。
3. 涉及 Gateway 改动时，至少执行 `cd apps/gateway && go test ./...`；若改动 contracts，同步执行 contract tests。
4. 每次任务收尾必须更新本文件（完成项/阻塞/更新时间），并记录本次改动摘要与验证命令。

## 3. 当前未完成项与阻塞
- [ ] 阻塞：当前环境未找到交接文件 `/home/ruan/.codex/handoff/latest.md`（Windows 下 `C:\Users\Lenovo\.codex\handoff\latest.md` 也不存在）。
- [ ] 阻塞：`pnpm -C apps/web test` 当前仍失败 1 项：`test/e2e/web-shell-tool-flow.test.ts` 用例“搜索页支持过滤会话并点击进入会话详情”中 `search-chat-input` 为 `null`。
- [ ] 阻塞：`cd apps/gateway && go test ./...` 当前失败 1 项：`TestProcessAgentViewOutOfBoundsFallsBackToEmptyFile` 断言仍使用旧文案 `[empty file fallback]`，实际输出已为 `[empty] (fallback from requested [1-100], total=0)`。

## 4. 最近关键变更（摘要）
- [x] 2026-02-19 09:39 +0800 工作区分批提交完成（1/3）：`72b0a78`，主题 `feat(gateway): protect default chat from deletion`，包含 Gateway 默认会话保护 + OpenAPI/contract 文档同步。
- [x] 2026-02-19 09:39 +0800 工作区分批提交完成（2/3）：`7e3b239`，主题 `feat(web): refactor console settings and search interactions`，包含 Web 设置分组重构、搜索弹窗交互、样式与测试同步。
- [x] 2026-02-19 09:39 +0800 本次验证：`pnpm --filter @nextai/tests-contract run test` 通过；`pnpm -C apps/web build` 通过；`pnpm -C apps/web test` 失败 1 项（见阻塞）；`cd apps/gateway && go test ./...` 失败 1 项（见阻塞）。
- [x] 2026-02-19 00:01 +0800 控制台设置右侧固定高度：`apps/web/src/styles.css` 为 `.settings-layout` 增加固定高度，右侧 `.settings-sections` 改为 `overflow-y: auto`，内容超出时仅右侧区域上下滚动。
- [x] 2026-02-19 00:01 +0800 本次验证：`pnpm -C apps/web build` 通过；`pnpm -C apps/web test` 失败 2 项（`test/smoke/shell.test.ts`、`test/e2e/web-shell-tool-flow.test.ts`，均与 `search` tab DOM 缺失相关）。
- [x] 2026-02-18 23:52 +0800 聊天输入区去灰色卡片底座：`apps/web/src/styles.css` 将 `.composer-shell` 改为透明无边框，仅保留白色输入框与下方状态行。
- [x] 2026-02-18 23:52 +0800 本次验证：`pnpm -C apps/web build` 通过；`pnpm -C apps/web test` 失败 1 项（`test/e2e/web-shell-tool-flow.test.ts` 中 `search-chat-input` 为 `null`）。
- [x] 2026-02-18 23:50 +0800 模型管理删除按钮改为会话卡片同款垃圾桶图标：`apps/web/src/main.ts`、`apps/web/src/styles.css`。
- [x] 2026-02-18 23:50 +0800 新增断言：`apps/web/test/e2e/web-active-model-chat-flow.test.ts` 校验模型删除按钮包含 `chat-delete-btn` 与垃圾桶 SVG，且 `aria-label` 为“删除提供商”。
- [x] 2026-02-18 23:50 +0800 本次验证：`pnpm --filter @nextai/web lint`、`pnpm -C apps/web build` 通过；`pnpm -C apps/web test` 失败 2 项（见阻塞项）。
- [x] 2026-02-18 23:49 +0800 控制台设置关闭按钮改为 SVG 图标：`apps/web/src/index.html` 的 `#settings-popover-close` 从文案按钮改为图标按钮，并使用 `data-i18n-aria-label="common.close"` 保留可访问名称。
- [x] 2026-02-18 23:49 +0800 本次验证记录：执行 `pnpm -C apps/web test`（失败，报错含 `missing_element: #panel-search` 与 smoke 断言 `data-tab="search"`）和 `pnpm -C apps/web build`（失败，`src/main.ts(1330,27)` TS2367）。
- [x] 2026-02-18 23:44 +0800 聊天 UI 去横线：`apps/web/src/styles.css` 移除会话区与对话区头部横线（`.sidebar > .panel-head`、`.chat > .panel-head`），并去掉输入区分隔线（`.composer`、`.composer-status-row`）。
- [x] 2026-02-18 23:44 +0800 验证通过：执行 `pnpm -C apps/web test -- test/smoke/shell.test.ts` 与 `pnpm -C apps/web build` 均通过。
- [x] 2026-02-18 23:36 +0800 Web 设置重构收敛：`models/channels/workspace` 已迁入控制台设置分组，相关 e2e/smoke/build 均通过。
- [x] 2026-02-18 23:41 +0800 `apps/web/src/index.html` 编码与损坏标签修复，聊天/搜索/配置区中文后备文案恢复。
- [x] 2026-02-18 20:29 +0800 默认会话保护落地：`chat-default` 强制保留，删除返回 `400 default_chat_protected`，契约与测试同步完成。
- [x] 2026-02-18 19:01 +0800 Windows smoke 链路修复并通过：`pnpm -C tests/smoke test`、`pnpm -r test`、`pnpm -r build`。
- [x] 2026-02-18 23:42 +0800 TODO 精简：移除冗余历史流水，保留接手必需信息与有效阻塞项。
