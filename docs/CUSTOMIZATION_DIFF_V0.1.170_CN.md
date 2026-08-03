# Sub2API 0.1.170 Vote AI 二开差异说明

## 1. 对比基线

| 项目 | Git 引用 | 提交 |
| --- | --- | --- |
| 官方版本 | `upstream/main` | `7e2e9ba05026b7126318aa0754c1afa0ac00bc58` |
| 二开长期基线 | `origin/custom` | `a1c503671cc01b346cc9498d9353f69a806119b0` |
| 0.1.170 升级分支 | `codex/sync-upstream-v0.1.170` | 见本次升级合并提交 |
| 升级前生产标签 | `custom-v0.1.169-1` | `d228cd0f838069efcd00833d8f8a76e687a8b1eb` |
| 应用版本 | `backend/cmd/server/VERSION` | `0.1.170` |

官方 `v0.1.170` 标签指向发布合并提交 `c043c247`，随后官方提交
`7e2e9ba0` 将 `VERSION` 同步为 `0.1.170`。因此本次使用
`upstream/main@7e2e9ba0` 作为目标引用，而不是直接使用标签树。

在新增本文档前，合并结果相对官方树的 Git 统计为：

- 44 个差异文件；
- 新增 3438 行；
- 删除 666 行；
- 未追踪的 `assets/branding/` 本地素材不在 Git 差异和发布镜像中。

## 2. 总体结论

0.1.170 官方代码已经完整合入，合并过程无文本冲突。Vote AI 二开仍集中在：

1. Vote AI 品牌默认首页和交互式地球；
2. 管理员可维护的站内 Markdown 文档；
3. 后台站点 Logo 驱动的首页、登录页和控制台品牌展示；
4. Vote AI 主题、构建接入和专项测试；
5. `custom-v*` 标签触发的自定义镜像发布工作流；
6. `/pricing` 到官方 `/model-plaza` 的兼容跳转；
7. OpenAI OAuth 账号的 TLS 指纹、客户端身份路由和代理保持能力。

二开没有重写官方账号调度、利润控制、计费、渠道、鉴权或网关转发核心。

## 3. 官方 0.1.170 更新

本次保留的主要官方能力包括：

- 分组级利润控制和请求级定价时刻；
- OpenAI、Anthropic、Gemini、Grok、Antigravity 上游倍率探测；
- 可选的账号倍率自动同步；
- 管理端账号筛选结果全选和批量删除并发限制；
- 模型广场筛选、排序和价格表布局优化；
- Anthropic 流式中断用量、OpenAI WebSocket、SSE 429、Responses 工具图片等修复；
- 内容审核请求通过配置代理转发；
- 提示词安全审计的“仅审计最新输入”范围。

官方新增迁移：

| 文件 | 作用 | 兼容性 |
| --- | --- | --- |
| `192_group_profit_control.sql` | 为 `groups` 增加利润控制开关、最低利润率和安全缓冲字段 | 字段均有默认值，功能默认关闭 |
| `193_group_profit_control_auth_cache_invalidation.sql` | 扩展分组认证缓存失效函数 | `CREATE OR REPLACE`，不删除业务数据 |
| `194_add_tls_fingerprint_routers.sql` | 新增按入站 User-Agent 匹配的 TLS 指纹路由表及 Codex CLI 固定路由 | 新表和默认路由，不修改现有账号数据；账号开关默认关闭 |

二开没有修改官方的 `192`、`193` 迁移。本次新增 `194` 迁移；生产升级前仍必须完成
PostgreSQL 备份和实际恢复验证，应用启动后会自动执行尚未执行的迁移。

## 4. Vote AI 二开边界

### 4.1 独立目录

前端二开主要位于：

```text
frontend/src/custom/vote-ai/
```

目录包含：

- `views/VoteAiHome.vue`：Vote AI 默认首页；
- `views/DocsView.vue`：公开文档和管理员文档维护页面；
- `components/InteractiveGlobe.vue`：交互式地球；
- `components/MarkdownContent.vue`：清理后的 Markdown 渲染；
- `api/docs.ts`：站内文档 API；
- `branding.ts`：默认品牌资源和品牌路由判断；
- `__tests__/`：二开专项测试。

官方文件中的接入点使用以下可搜索标记：

```text
CUSTOM(VOTE-AI-HOME)
CUSTOM(VOTE-AI-DOCS)
CUSTOM(VOTE-AI-THEME)
CUSTOM(VOTE-AI-BUILD)
CUSTOM(VOTE-AI-BRANDING)
```

### 4.2 首页优先级

首页显示优先级保持为：

1. 管理员配置的 `home_content`；
2. 官方 compact 首页；
3. Vote AI 默认首页。

Vote AI 首页通过小型接入点从 `frontend/src/views/HomeView.vue` 加载，官方首页逻辑没有
复制进二开目录。

### 4.3 站内文档

公开路由：

```text
/docs
/docs/:slug
GET /api/v1/docs
```

管理员接口：

```text
GET /api/v1/admin/docs
PUT /api/v1/admin/docs
```

文档序列化后保存在现有系统设置键 `docs_content` 中，不新增数据表。普通用户只能读取
已发布文档；管理员可以新增、编辑、排序、发布和删除文档。

### 4.4 品牌 Logo

后台配置的 `site_logo` 优先于 Vote AI 默认 Logo，并在以下位置一致生效：

- Vote AI 首页；
- 登录和注册布局；
- 用户及管理员控制台侧栏；
- 浏览器 Favicon。

源码不再硬编码替换管理员上传的 Logo。

### 4.5 模型价格页面

二开不再维护独立静态模型价格数据：

- `PricingView.vue` 不存在；
- `pricing-data.ts` 不存在；
- `/pricing` 只做兼容跳转到 `/model-plaza`；
- 模型和分组价格由官方模型广场读取后台真实配置。

页面展示不能替代计费验证。`gpt-5.6-luna`、`gpt-5.6-terra` 和 fast 模式仍需通过
测试账号的实际账单差额验证。

## 5. 后端二开文件

站内文档功能主要涉及：

```text
backend/internal/service/setting_docs.go
backend/internal/handler/setting_handler.go
backend/internal/handler/admin/setting_handler.go
backend/internal/server/routes/auth.go
backend/internal/server/routes/admin.go
```

对应测试：

```text
backend/internal/service/setting_docs_test.go
backend/internal/handler/admin/setting_docs_handler_test.go
```

0.1.170 官方与二开同时修改的主要接入文件是
`backend/internal/server/routes/admin.go`。自动合并同时保留了官方账号批量删除路由和
Vote AI 文档管理路由，后续升级应继续重点检查该文件。

## 6. 前端主题与构建

二开继续使用 Vote AI 主题和构建接入：

```text
frontend/tailwind.config.js
frontend/postcss.config.js
frontend/vite.config.ts
frontend/src/style.css
```

`cobe` 用于首页地球渲染。主题色影响公开页和控制台，因此每次升级都要同时进行桌面端、
移动端、亮色和暗色视觉回归。

## 6.1 OpenAI OAuth TLS 指纹

本次同版本二开发布新增以下能力：

- OpenAI OAuth 账号复用账号级 `enable_tls_fingerprint` 开关；
- 内置 Codex CLI 固定 TLS 身份和默认 User-Agent 路由；
- 可按入站 User-Agent 选择 TLS 指纹模板、上游 User-Agent 和 Originator；
- 路由身份缺失、路由模板无效或未命中时回退到固定 Codex CLI 身份；
- TLS 请求和 WebSocket 请求始终保留账号原有代理绑定，TLS 失败不会绕过代理直连；
- TLS 未启用或无法解析模板时继续使用官方普通 HTTP 路径，保持原有行为；
- 所有 OpenAI 上游入口统一经过 `doOpenAIUpstream`，AST 守卫测试防止后续同步新增直连旁路。

主要接入文件：

```text
backend/internal/service/openai_gateway_service.go
backend/internal/service/openai_ws_client.go
backend/internal/service/tls_fingerprint_router_service.go
backend/internal/service/openai_tls_router_test.go
frontend/src/components/admin/TLSFingerprintRoutersModal.vue
frontend/src/components/account/CreateAccountModal.vue
frontend/src/components/account/EditAccountModal.vue
```

该能力以 OpenAI OAuth 账号开关为边界，OpenAI API Key 账号不启用 TLS 指纹模拟。

## 7. 发布工作流

`.github/workflows/custom-image.yml` 在推送 `custom-v<版本>-<序号>` 标签时构建
`linux/amd64` GHCR 镜像。工作流要求：

- 标签版本与 `backend/cmd/server/VERSION` 一致；
- 标签提交属于远程 `custom` 分支历史；
- 只有完成本地测试和 PR 合并后才能创建发布标签。

## 8. 0.1.170 验收重点

### 自动化测试

```powershell
cd frontend
corepack pnpm install --frozen-lockfile
corepack pnpm test:custom
corepack pnpm lint:check
corepack pnpm typecheck
corepack pnpm build
corepack pnpm test:run

cd ../backend
go test -tags=unit ./...
```

集成环境可用时追加：

```powershell
go test -tags=integration ./...
```

### 页面和功能

- 首页三层优先级和 Vote AI 默认首页；
- `/docs` 公开读取和管理员维护；
- 后台上传 Logo 后各页面同步更新；
- `/pricing` 跳转 `/model-plaza`；
- 模型广场显示真实模型与分组价格；
- 管理员账号、分组和利润控制页面；
- 内容审核代理配置和“仅审计最新输入”；
- TLS 指纹路由器默认 Codex CLI 路由和 OpenAI OAuth 账号 TLS 开关；
- 桌面端、移动端、亮色和暗色布局；
- 浏览器控制台无应用错误。

### 计费

- `gpt-5.6-luna` 和 `gpt-5.6-terra` 当前业务价格；
- fast 模式按渠道基础计价的两倍结算；
- fast 倍率不被渠道模型定价覆盖；
- 0.1.170 利润控制和账号倍率自动同步默认关闭，不应改变现有调度和计费。

## 9. 升级与回滚注意事项

- 生产切换前备份 PostgreSQL、Redis、`/app/data`、Compose、`.env` 和配置文件；
- PostgreSQL 备份必须通过 `pg_restore --list` 和临时数据库实际恢复验证；
- 只重建应用服务，不重启 PostgreSQL 和 Redis，不删除数据卷；
- 旧镜像只能回退应用代码，不能自动回退已执行的数据库迁移；
- 如旧应用无法兼容迁移后的数据库，必须恢复升级前 PostgreSQL 备份；
- 0.1.170 利润控制和账号倍率同步在验证前保持关闭。

## 10. 本地验收记录

本次升级已完成以下本地验收：

- 前端二开专项测试：6 个测试文件、31 个测试全部通过；
- 前端 lint、类型检查、生产构建和完整 Vitest 测试通过；
- 后端 `go test -tags=unit ./...` 通过；
- 构建并运行 `linux/amd64` 候选镜像 `sub2api-custom:0.1.170-openai-tls-candidate`，镜像内版本为 `0.1.170`；
- `/health` 返回 `{"status":"ok"}`；
- 迁移 `192`、`193`、`194` 已应用，现有用户和分组数据仍可读取；
- TLS 路由器弹窗可读取默认 `OpenAI Codex CLI` 路由；OpenAI OAuth 创建表单显示 TLS 指纹开关；
- OpenAI TLS 身份归一化、固定模板回退、代理保持、HTTP/WebSocket 统一入口及 AST 守卫测试通过；
- 浏览器验证 Vote AI 首页、模型广场、`/pricing` 跳转、站内文档、后台仪表盘和分组管理正常；
- 分组编辑界面显示“启用利润控制”；
- 系统设置显示“启用风控中心”和内容审计配置入口；当前本地配置仍为关闭，因此风控页面按设计跳回系统设置；
- 390 px 移动端视口下首页和分组页无横向溢出，浏览器控制台无应用错误。

本地测试库没有 API Key 和上游账号，因此本次无法执行真实模型请求、余额扣减、`gpt-5.6-luna`、`gpt-5.6-terra` 和 fast 2 倍计费的端到端验证。这些项目仍是生产切换前的业务验收项。
