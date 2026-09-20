# 官方 main 同步记录：0.2.7

日期：2026-09-20 开始，2026-09-21 完成本地验证。

## 来源与状态

- 定制版基线：`zhoucheng0508/sub2api` 的 `custom`，`5d24d82127fd9b8a408f74b8951bb7acf75b4202`。
- 官方来源：`https://github.com/Wei-Shaw/sub2api.git`。
- 官方目标：本次拉取的 `upstream/main`，`7c700729c23187d31ed320f6b19c790e2f194826`，源码 VERSION 为 `0.2.7`。
- 使用 main 的固定提交，不使用版本标签代替提交：本次读取的 `v0.2.7` 标签内 VERSION 仍是 `0.2.5`。
- 共同祖先：`5de5e2bed035d43591a2e10e51f420ef6a84eb98`。
- 备份分支：`backup/custom-before-v0.2.7-20260920`，指向定制版基线。
- 同步分支：`sync/upstream-v0.2.7-20260920`。
- 工作区：`C:/Users/Administrator/Documents/AI_project/.worktrees/sync-upstream-v0.2.7-20260920`。
- 使用 `merge --no-ff --no-commit`。文本冲突已解决，并于 2026-09-21 经用户明确确认后提交：`430e43e6e0f8fd8719793aab8d8f2e9ebadc8ee8`。
- 未推送分支、创建发布标签或部署服务器。

原 `shared-subscription-20260914` 工作区及其暂存的共享订阅设计文档未改动。文档 SHA-256：`03592e27858fa31c4f011b4acdf4f8983a2a6a89f1afd55c73ab58223c0e0774a`。

## 范围与合并决策

相对共同祖先，官方侧有 598 个变更文件，定制侧有 405 个变更文件，交集为 98 个文件；合并首次报告 29 个文本冲突文件。官方侧有 296 个基线尚未包含的提交。

保留 Vote AI 首页、站内文档及管理接口、站点 Logo、主题、`/pricing` 到 `/model-plaza` 的跳转、TLS 指纹路由、提示缓存、AI 多轮风险审计和异步视频计费。

主要处理：

1. **风控引擎兼容**：接入官方 OpenAI/TypeSafe 引擎配置、密钥状态、阈值及 engine_meta 日志，同时保留定制 AI Chat 配置、会话风险、审核细节、预算和成本统计。AI Chat、OpenAI、TypeSafe 的地址与密钥独立保存；增加前后端切换回归测试。
2. **输入提取边界**：定制的有界多轮提取及来源归一化继续用于 AI Chat。官方 OpenAI/TypeSafe 路径使用当前用户输入提取器；本地关键词检查仍检查用户提交的 reminder 原文。官方提取器放入 `content_moderation_current_user.go`，现有多轮提取接口继续保留，相关官方测试明确调用当前用户提取器。
3. **审核缓存与日志**：OpenAI/TypeSafe 采用官方当前输入哈希，AI Chat 保持定制审核目标哈希；旧版不同输入边界生成的缓存不会自动转换。关键词命中的日志记录实际匹配文本，即使语义审核过滤了 reminder 也保留诊断信息。日志 SQL 同时保留定制 audit_details 与官方 engine_meta，插入参数和读取顺序均有测试。
4. **平台和视频**：分组校验、前端平台类型和调度平台集合同时容纳 `laogou` 与官方新增 `opencode_go`；保留视频按次计费字段。官方 Seedance 解析先完成，再生成审核请求体。
5. **网关与依赖注入**：采用官方 XML 心跳校验函数并移除旧同名实现；采用官方插件 Provider 并保留定制 TLS Provider；Wire 已实际重新生成，结果通过编译。
6. **前端兼容**：保留定制一键配置和官方批量 Key 编辑；修复测试桩重复渲染操作栏的问题。补回账号编辑页上游请求 ID 和图片 URL 转 base64 配置，并补齐中英文翻译。官方隐藏外链按钮与定制用户上下文开关均保留。
7. **测试兼容**：一键配置测试中的 `base_instructions` 在当前安装的 Codex 解析器中已不是必填字段；改用非法 `context_window` 验证解析失败及配置不落盘，未移除失败保护断言。
8. **依赖**：保留二开版较新的 `golang.org/x/image v0.45.0` 及 grpc 修复版本；保留官方合并后的其他依赖。Wire 运行补充其工具依赖 `github.com/google/subcommands v1.2.0` 的 go.sum 校验。

## 数据库迁移检查

官方新增：

- `238_opencode_go_platform.sql`：扩展平台检查约束。
- `238_purge_unlimited_user_platform_quotas.sql`：清理三档限额均为 NULL 的不限额记录。
- `238b_content_moderation_engine_meta.sql`：新增可空的审核引擎元数据字段。

迁移执行器按完整文件名排序并记录校验和，以上文件没有与现有二开文件同名冲突。迁移静态测试通过；本次没有对数据库执行迁移或进行数据库恢复演练。

## 本地验证

- 前端定向测试：361 项通过（原 27 个测试文件共 360 项，加 1 项风控切换回归；失败项修正后已复测）。范围包含品牌首页/文档、Logo、账号/分组、设置、风险控制、Key 页面、一键配置及翻译完整性。
- 前端 `vue-tsc --noEmit`、`vue-tsc -b`：通过。
- 冲突相关前端文件 ESLint：通过。
- Vite 生产构建：通过，输出到 `backend/internal/web/dist`。仅有现存大块体积、Browserslist 数据龄期和 Node 弃用提示。
- 后端定向验证累计去重：2301 项通过（包含子测试），21 个包通过，5 项按测试自身条件跳过。
- 跳过项：TypeSafe 实际服务、插件实际进程、3 项 TLS 外网测试；它们需要显式测试开关、凭据或外部测试包。
- 定向范围：AI 审计及缓存/配置、审核存储、视频/Seedance、模型清单、插件、调度、网关路由、接口契约、Wire、迁移及定制辅助包。
- `go run -mod=mod github.com/google/wire/cmd/wire ./cmd/server`：通过。
- 后端普通构建及嵌入新前端资源的 `go build -tags embed ./cmd/server`：通过，构建产物置于工作区外部 output 目录。
- `git diff --check`、暂存区差异检查、未合并文件检查：通过。

遵照仓库升级流程，本次未运行全量项目测试。没有进行浏览器端到端验证、PostgreSQL/Redis 实例集成或生产部署，不能把本地构建结果视为生产验收。

本地验证日志保存在 `C:/Users/Administrator/Documents/AI_project/output/sub2api-v027-*`。pnpm 使用现有 9.15.9 工具链及冻结锁文件安装；Go 依赖直连超时后使用 goproxy.cn 下载，未改变仓库的依赖代理配置。

## 原始文本冲突清单

以下 29 个文件均已处理，供最终审核定位：

```text
.gitignore
backend/cmd/server/VERSION
backend/cmd/server/wire_gen.go
backend/go.mod
backend/internal/handler/admin/content_moderation_handler.go
backend/internal/handler/admin/group_handler.go
backend/internal/handler/dto/settings.go
backend/internal/handler/grok_media.go
backend/internal/handler/openai_gateway_handler.go
backend/internal/repository/content_moderation_repo.go
backend/internal/repository/content_moderation_repo_test.go
backend/internal/service/account_test_models_test.go
backend/internal/service/channel_monitor_checker.go
backend/internal/service/composite_platform_test.go
backend/internal/service/content_moderation.go
backend/internal/service/content_moderation_input.go
backend/internal/service/openai_models_list_test.go
backend/internal/service/scheduler_snapshot_service.go
backend/internal/service/wire.go
frontend/src/App.vue
frontend/src/api/admin/riskControl.ts
frontend/src/i18n/locales/en/admin/channels.ts
frontend/src/i18n/locales/zh/admin/channels.ts
frontend/src/types/index.ts
frontend/src/views/admin/RiskControlView.vue
frontend/src/views/admin/SettingsView.vue
frontend/src/views/admin/__tests__/RiskControlView.spec.ts
frontend/src/views/user/KeysView.vue
frontend/src/views/user/__tests__/KeysView.spec.ts
```

审核重点是风控输入边界及配置隔离、平台并集、视频审核顺序。用户已确认并完成本地合并提交；远程发布和部署另行执行。
