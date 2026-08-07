# Sub2API 0.1.171 Vote AI 二开差异说明

## 1. 升级基线

| 项目 | Git 引用 | 提交 |
| --- | --- | --- |
| 官方版本 | `upstream-v0.1.171` | `f0e7a9c7a23a7d02fb159b62fa809621eb0475a6` |
| 二开长期基线 | `origin/custom` | `2fabadba18f859b026a053f4cc01c7ba4afb3c63` |
| 升级分支 | `codex/sync-upstream-v0.1.171` | 见本次 PR |
| 升级前生产标签 | `custom-v0.1.170-8` | 归属 `origin/custom` |
| 应用版本 | `backend/cmd/server/VERSION` | `0.1.171` |

官方 `v0.1.171` 标签的 `backend/cmd/server/VERSION` 仍然是 `0.1.170`。本次锁定标签提交
`f0e7a9c7` 进行合并，并在二开升级分支单独将 VERSION 修正为 `0.1.171`。不使用会继续移动的
`upstream/main` 替代发布标签。

## 2. 官方 0.1.171 变更

本次合入的官方主要能力包括：

- 腾讯天御验证码和阿里云验证码 2.0，与 Turnstile 合并为互斥的验证码配置；
- Codex OAuth 出站 `User-Agent` / `originator` / `version` 归一化和官方版本自动同步；
- composite 分组的 reasoning effort 上限和映射；
- OpenAI 账号重置额度缓存；
- `gateway.disable_codex_originator_normalization` 回滚开关；
- 退款 `require_force` 破坏性变更、Stripe 幂等退款和事务一致性修复；
- 订阅并发续期、前端 token refresh、WS 租约、调度快照和计费修复；
- 模型广场图片价格口径和 prompt audit `output_text` 解析修复。

## 3. 合并决策

合并共涉及 206 个官方变更文件。唯一文本冲突为生成文件
`backend/cmd/server/wire_gen.go`。处理方式为：

1. 不手工拼接生成代码；
2. 保留官方 `OpenAICodexVersionSyncService` 与 Vote AI `ContentModerationService`；
3. 为 Vote AI repository 增加显式的 service port 适配器，修复 Wire 对未导出具体类型的依赖绑定；
4. 使用 `go generate ./cmd/server` 重新生成 `wire_gen.go`；
5. 补齐 Wire `v0.7.0` 命令所需 `google/subcommands` 的 module 校验信息，保证新环境可重复生成。

生成结果已确认同时注入内容审核 repository/cache/service/handler、Codex 版本同步服务，并在应用
cleanup 中停止 Codex 版本同步任务。

## 4. 官方身份归一化与 Vote AI TLS 边界

- 官方代码负责 Codex OAuth 出站 HTTP 头、版本号、客户端身份和全路径归一化。
- Vote AI TLS 只负责账号级开关、入站 User-Agent 路由、规范身份到 uTLS 模板的映射和固定模板回退。
- Vote AI 不恢复与官方重复的版本号或通用 HTTP 头改写逻辑。
- TLS 失败不得绕过账号原有代理直连；HTTP、WS、探针和模型发现路径继续由旁路守卫测试覆盖。

## 5. Vote AI 二开保留范围

本次同步保留：

- Vote AI 默认首页、交互式地球、主题、站内 Markdown 文档和站点 Logo；
- `/pricing` 到官方 `/model-plaza` 的兼容跳转；
- DeepSeek 语义审核、快审/完整审核、上下文感知、用户/账号范围过滤、三级风险、邮件去重和安全解禁；
- OpenAI OAuth TLS 指纹路由、Codex 固定模板、代理保持与旁路守卫；
- OpenAI API Key GPT-5.6 提示缓存优化；
- `custom-v*` 标签的 GHCR 发布工作流。

主要隔离目录与接入点仍为：

```text
frontend/src/custom/vote-ai/
backend/internal/custom/voteai/
backend/internal/service/content_moderation*.go
backend/internal/service/tls_fingerprint_router_service.go
backend/internal/service/openai_apikey_prompt_cache.go
CUSTOM(VOTE-AI-*)
```

## 6. 迁移边界

官方 `0.1.171` 迁移仍截止到 `193`。Vote AI 已在生产应用的迁移未修改、未改名、未删除：

| 迁移 | SHA-256 |
| --- | --- |
| `194_add_tls_fingerprint_routers.sql` | `c427ca5d88e89a30cef84b8a53e9a475fadeca5cff8de0abe595f4f079533f83` |
| `195_vote_ai_content_moderation_side_effect_state.sql` | `ee3284ecefd4781b55f3433ce527ca241916e0c6edbef07f6f26766a7af58e83` |
| `196_vote_ai_content_moderation_audit_details.sql` | `16e3d3245ef1f34dfdb002a179c511bfbf72a7a6a927ccc397f3d86624fe17a9` |

生产切换前仍必须完成 PostgreSQL custom dump、globals/配置/Redis 归档和临时库实际恢复验证。

## 7. 已完成的本地关卡

| 关卡 | 结果 |
| --- | --- |
| `go generate ./cmd/server` | 通过，Wire 生成结果无冲突标记 |
| `go test -tags=unit ./...` | 通过（测试进程清空 `OPENAI_API_KEY`，避免付费外部 API） |
| `go test -tags=integration ./...` | 通过 |
| `corepack pnpm install --frozen-lockfile` | 通过，锁文件无变化 |
| `corepack pnpm test:custom` | 12 个文件、83 个测试通过 |
| `corepack pnpm lint:check` | 通过 |
| `corepack pnpm typecheck` | 通过 |
| `corepack pnpm build` | 通过，1025 个模块完成生产构建 |
| `corepack pnpm test:run` | 通过 |
| `git diff --cached --check` | 通过 |

`TestEstimateOpenAIInputTokens_CompareWithOpenAIAPI` 在本机存在 OpenAI Key 时会请求官方付费 API；因当前网络拒绝连接，
本次离线单元套件显式清空 Key 使其跳过。该外部对照测试不作为代码回归成功证据。

## 8. 后续发布关卡

在合并到 `custom` 和生产切换前还必须完成：

1. 全新 `linux/amd64` Docker 镜像构建，核对 VERSION、commit、架构和实际 image ID；
2. 本地 Docker 健康、迁移幂等、页面、审核、TLS、提示缓存、验证码、退款和混合分组专项验收；
3. GitHub PR 最终 diff 审阅和全部 CI 通过；
4. 合并后创建 `custom-v0.1.171-1` annotated tag，并验证 GHCR 镜像嵌入 merge commit；
5. 生产只读审计、备份实际恢复验证、只重建 `sub2api` 服务，PostgreSQL/Redis 不重启；
6. 生产健康、数据、迁移、公开 API、管理页面、日志和 Vote AI 高冲突模块验收。

上述未完成项在得到实际命令、CI、镜像或运行时证据前均不视为通过。
