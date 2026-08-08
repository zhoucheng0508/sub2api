# Sub2API 0.1.172 Vote AI 二开升级差异说明

## 1. 升级基线

| 项目 | Git 引用 |
| --- | --- |
| 升级前二开基线 | `origin/custom@5f39fbbf5ecbab78e8f6b15527a4c78cb4666573` |
| 官方 `v0.1.172` 标签对象 | `61ba94d2e85a00ba639fc870b91946b1bd2f990d` |
| 官方标签指向提交 | `155c494964c3ea6ecc31f52679525c1034bf0f16` |
| 本次锁定的官方目标 | `68d8f122e47fe50e9edda3beba95c7bd98b19e56` |
| 目标二开发布标签 | `custom-v0.1.172-1` |

官方标签中的 `backend/cmd/server/VERSION` 仍为 `0.1.171`。标签之后的
`68d8f122` 仅将该文件同步为 `0.1.172`，因此本次按升级规范锁定这个明确提交，避免发布镜像显示错误版本。

## 2. 官方更新摘要

本次合入的主要官方能力和修复包括：

- 修复 OAuth 登录补全流程中的高危账号接管漏洞；
- 新增上游响应模型审计以及模型不一致筛选；
- Codex OAuth 默认客户端身份从 `codex_cli_rs` 切换为 `codex-tui`；
- 修复 Codex 容量降载恢复后的故障切换；
- 修复订阅日额度午夜刷新和计费精度量化问题；
- 为上游 DNS、TLS 和代理建连增加明确的 10 秒超时；
- 清理 Responses 转 Anthropic 时的无效 content block；
- 修复 Codex Desktop 工具 schema 中的 `type: null`；
- 改进模型广场的组合分组与跨平台同名模型展示；
- 增加 Gemini 3.6 Flash 支持，并包含腾讯验证码、易支付和日志退避等修复。

## 3. 合并冲突与决策

本次共有两个文本冲突：

1. `backend/cmd/server/VERSION`：采用 `0.1.172`。
2. `backend/internal/service/openai_codex_models_service.go`：接纳官方
   `openai.CodexDefaultOriginator`，同时保留 Vote AI 的统一 `s.httpUpstream` 调用路径。

模型清单请求继续经过账号绑定代理、TLS 指纹模板和身份联动策略。没有引入旁路客户端，也不允许 TLS
失败后绕过账号代理直接连接。守卫测试改为引用 `openai.CodexDefaultOriginator`，以持续跟随官方默认身份。

## 4. 数据库迁移兼容性

官方新增：

- `194_add_usage_log_upstream_response_model.sql`
- `195_add_usage_log_upstream_model_mismatch_index_notx.sql`

Vote AI 已有：

- `194_add_tls_fingerprint_routers.sql`
- `195_vote_ai_content_moderation_side_effect_state.sql`
- `196_vote_ai_content_moderation_audit_details.sql`

迁移执行器按完整文件名记录迁移，数字前缀重复不会相互覆盖。两组迁移操作的表、字段和索引也不冲突；
官方 usage log 字段迁移先执行，`_notx` 并发索引由专用阶段执行。生产升级前仍必须完成 PostgreSQL
逻辑备份、临时库实际恢复和关键计数核对。

## 5. 二开边界

本次继续保留 Vote AI 的品牌首页、站内文档、模型广场调整、OpenAI OAuth TLS 指纹路由、账号代理守卫、
内容安全审核、会话风险状态、通知去重、审核成本与延时优化。官方账号调度、计费、鉴权和网关主流程的
更新均按 0.1.172 接纳；二开接入点仍由现有守卫测试覆盖。

## 6. 验证结果

- 后端：`go test -tags=unit ./...` 全部通过；
- 后端静态检查：`go vet ./...` 通过；
- 前端：220 个测试文件、1615 项测试全部通过；
- 前端：`lint:check`、`typecheck`、生产构建全部通过；
- `git diff --check` 通过；
- TLS 身份专项测试和 `internal/service` 全量测试通过。
- 本地 Docker 镜像构建通过，`/health` 返回 `{"status":"ok"}`；
- 镜像内置版本为 `0.1.172`，内置提交与本次合并提交一致；
- 隔离 PostgreSQL 实测同时记录官方 `194/195` 和 Vote AI `194/195/196` 五个完整迁移文件名；
- `usage_logs.upstream_response_model` 字段和 `tls_fingerprint_routers` 表均已创建。

本机未安装 `golangci-lint` 和 `govulncheck`，对应检查交由 GitHub CI 和安全扫描工作流执行。前端测试
存在既有的 Vue 组件解析警告和 Browserslist 数据提示，但无失败用例。

## 7. 发布与回滚要求

发布前必须等待 GitHub CI 全绿，并核验 GHCR 镜像为 `linux/amd64`、内置版本为 `0.1.172`、内置提交
等于本次合并提交。生产环境只重建应用容器，不重启 PostgreSQL 或 Redis，不删除卷，不覆盖 `.env`、
`config.yaml` 或数据目录。保留旧镜像、备份、Compose override、校验清单和 `ROLLBACK.txt`。
