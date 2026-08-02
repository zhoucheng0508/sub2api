# Sub2API 二开版本安全同步与升级提示词

## 1. 文档用途

本文用于以后将 Sub2API 官方新版本同步到 Vote AI 二开版本，并完成：

1. Git 检查点和升级分支创建；
2. 官方代码合并与二开功能保留；
3. 前后端测试、本地 Docker 构建和浏览器回归；
4. 提交到 `bupiter/sub2api`；
5. 生产数据备份、应用升级、验证和回退记录。

本文的核心原则是：每个阶段都必须验收通过，才能进入下一阶段。不能为了“完成升级”跳过冲突检查、测试、备份恢复验证或生产数据核对。

## 2. 仓库与分支约定

### 2.1 Git 远程

| 远程名 | 仓库 | 用途 |
| --- | --- | --- |
| `origin` | `https://github.com/bupiter/sub2api.git` | Vote AI 自有仓库，保存二开代码 |
| `upstream` | `https://github.com/Wei-Shaw/sub2api.git` | Sub2API 官方仓库，默认从这里同步官方更新 |
| `secondary` | `https://github.com/zhoucheng0508/sub2api.git` | 历史二开参考源，默认不参与后续官方同步 |

除非用户明确要求，不要把 `secondary/custom` 当作官方更新反复合并。它可能已经包含旧的官方合并和二开提交，重复合并会增加历史和冲突复杂度。

### 2.2 长期分支

| 分支 | 用途 |
| --- | --- |
| `custom` | 长期二开主分支，只接收已测试通过的升级和二开修改 |
| `codex/sync-upstream-vX.Y.Z` | 单次官方升级临时分支，从最新 `custom` 创建 |
| `codex/isolate-customizations` | 0.1.169 时用于完成二开隔离的阶段性分支，不应成为永久升级主线 |

建议流程：

```text
upstream/main ───────┐
                     ├─> codex/sync-upstream-vX.Y.Z ─> PR ─> custom
origin/custom ───────┘                                  │
                                                        └─> custom-vX.Y.Z-N
```

### 2.3 首次使用前的一次性检查

截至 `custom-v0.1.169-1`，最新二开隔离提交位于 `codex/isolate-customizations`，而 `custom` 仍停留在更早的提交。下一次正式同步前，必须先确认生产标签已经属于长期 `custom` 分支历史：

```powershell
git fetch origin --prune --tags
git merge-base --is-ancestor custom-v0.1.169-1 origin/custom
if ($LASTEXITCODE -ne 0) { throw "origin/custom 尚未包含当前生产版本，先合并二开隔离分支" }
```

如果检查失败，应先把 `codex/isolate-customizations` 通过 PR 合并到 `custom`，再开始新版本同步。不能直接从落后的 `custom` 创建升级分支。

## 3. 每次使用前需要填写的信息

复制主提示词前先填写：

```text
目标官方版本：<例如 0.1.170>
当前生产版本：<例如 0.1.169>
当前生产标签：<例如 custom-v0.1.169-1>
发布序号：<通常为 1>
官方目标引用：<upstream/main、官方 tag 或明确 commit>
生产 SSH 主机：<不要在提示词中填写密码>
生产站点 URL：<例如 https://example.com>
生产 Compose 目录：<例如 /root/sub2api-deploy>
生产应用服务名：<例如 sub2api>
生产 PostgreSQL 服务或容器名：<例如 sub2api-postgres>
生产 Redis 服务或容器名：<例如 sub2api-redis>
```

不要把服务器密码、数据库密码、API Key、`.env` 内容或用户隐私数据写入提示词。

## 4. 可直接复制的主提示词

将下面整段复制给 Codex，并替换所有 `<...>` 变量。

````text
你现在负责将 Vote AI 的 Sub2API 二开版本安全同步到官方 <TARGET_VERSION>，完成本地验证、GitHub 提交和生产升级。

已知信息：
- 仓库目录：C:\Users\Administrator\Documents\AI_project
- 自有远程：origin = https://github.com/bupiter/sub2api.git
- 官方远程：upstream = https://github.com/Wei-Shaw/sub2api.git
- 历史参考远程：secondary = https://github.com/zhoucheng0508/sub2api.git
- 长期二开分支：custom
- 当前生产版本：<CURRENT_PROD_VERSION>
- 当前生产标签：<CURRENT_PROD_TAG>
- 目标官方引用：<OFFICIAL_REF>
- 目标版本：<TARGET_VERSION>
- 发布标签：custom-v<TARGET_VERSION>-<RELEASE_SEQ>
- 生产 SSH 主机：<PROD_SSH_HOST>
- 生产 URL：<PROD_URL>
- 生产 Compose 目录：<PROD_COMPOSE_DIR>
- 应用服务：<APP_SERVICE>
- PostgreSQL 容器：<POSTGRES_CONTAINER>
- Redis 容器：<REDIS_CONTAINER>

总体要求：
1. 一步一步执行直到完成，但必须设置阶段关卡；上一阶段未通过不得进入下一阶段。
2. 每个阶段开始和结束都向我报告当前状态、证据和下一步。
3. 不要向我索要或输出密码、数据库凭据、API Key、.env 内容和用户隐私。
4. 发现不确定的生产布局、目标版本不一致、无法恢复的冲突或数据风险时停止并说明，不要猜测。
5. 保留用户已有的未提交文件，不得 reset、checkout、clean 或覆盖无关修改。
6. 不得通过“全部接受 ours”或“全部接受 theirs”解决冲突。逐文件理解官方变化和二开接入点。
7. 默认只从 upstream 同步官方代码。secondary 只用于历史对照，除非我明确要求，否则不要合并 secondary/custom。
8. 启用并保留 Git 冲突记忆：git config rerere.enabled true。

阶段 A：Git 和版本预检
1. 读取 git status、当前分支、remotes、最近提交、标签和 backend/cmd/server/VERSION。
2. 获取 origin 和 upstream 的最新分支与标签，但不要 pull，不要切换工作区内容：
   git fetch origin --prune --tags
   git fetch upstream --prune --tags
3. 核对远程 URL，确认 upstream 确实是官方仓库。
4. 确认 origin/custom 包含当前生产标签：
   git merge-base --is-ancestor <CURRENT_PROD_TAG> origin/custom
5. 如果不包含，停止升级，先处理长期 custom 分支落后问题。
6. 核对 <OFFICIAL_REF> 的 VERSION、提交和目标版本。版本不一致时停止。
7. 记录升级前 commit、tag、官方 commit、工作区状态和二开差异统计。

阶段 B：创建升级分支和合并官方代码
1. 从最新 origin/custom 创建 codex/sync-upstream-v<TARGET_VERSION>，不能从旧同步分支或 secondary/custom 创建。
2. 在合并前创建可回退检查点；如果当前稳定提交没有明确标签，创建 annotated tag。
3. 使用 --no-ff 合并已核对的官方引用。
4. 如有冲突，逐项分析：
   - 官方核心网关、计费、渠道、账号调度、迁移和安全修复原则上采用官方新逻辑；
   - Vote AI 二开功能必须通过隔离目录和小型接入点保留；
   - 不得恢复已删除的静态定价页和 pricing-data.ts；
   - /pricing 应继续跳转到官方 /model-plaza；
   - Logo 继续优先使用后台上传的站点 Logo，不写死图片替换逻辑。
5. 解决冲突后检查 git diff、未合并文件和合并结果，不要立即提交。

阶段 C：二开边界专项审计
1. 阅读 frontend/src/custom/vote-ai/README.md。
2. 执行：
   rg "CUSTOM\(VOTE-AI" frontend backend
3. 确认以下二开能力仍存在：
   - Vote AI 默认品牌首页；
   - 首页配置优先级仍为自定义 home_content、compact 首页、Vote AI 默认首页；
   - /docs/:slug? 站内文档页面；
   - 管理员维护文档的后端服务、接口和路由；
   - /pricing 到 /model-plaza 的兼容跳转；
   - 后台上传 Logo 在首页、登录页、控制台布局中生效；
   - Vote AI 主题和前端构建接入点；
   - frontend/src/custom/vote-ai/ 下的专项测试。
4. 对比 upstream 与当前分支，只把真正的 Vote AI 差异保留为二开，不复制官方已经提供的功能。
5. 检查是否误提交构建产物、日志、数据库目录、备份、镜像包、.env 或本地品牌素材。
6. 更新 docs/CUSTOMIZATION_DIFF_V<TARGET_VERSION>_CN.md，删除已经过时的功能说明。

阶段 D：代码测试关卡
所有命令都要记录退出码和测试数量。任何一项失败都要修复并重跑，不能带失败进入 Docker 阶段。

前端至少执行：
```powershell
cd frontend
corepack pnpm install --frozen-lockfile
corepack pnpm test:custom
corepack pnpm lint:check
corepack pnpm typecheck
corepack pnpm build
corepack pnpm test:run
```

后端至少执行：
```powershell
cd backend
go test -tags=unit ./...
```

如集成测试环境可用，再执行：
```powershell
go test -tags=integration ./...
```

另外执行：
- 检查 backend/cmd/server/VERSION 等于 <TARGET_VERSION>；
- 检查数据库迁移文件序号、可重复运行风险和向后兼容性；
- 检查 CUSTOM(VOTE-AI-*) 标记未丢失；
- 检查 git diff 中没有无关格式化和依赖锁文件噪音。

阶段 E：本地 Docker 候选镜像
1. 先读取仓库现有 Dockerfile、CI 工作流和本地构建方式，不要凭空发明构建步骤。
2. 构建全新的 Linux amd64 镜像，标签使用：
   sub2api-custom:<TARGET_VERSION>-candidate
3. 不能复用旧版本二进制或旧 frontend/dist 冒充新镜像。
4. 如果 Docker Hub 网络不可用，可使用仓库已有的预编译 Dockerfile 流程，但必须重新完成：
   - 前端生产构建；
   - 嵌入新前端资源的 Linux amd64 Go 二进制；
   - 镜像构建；
   - 镜像架构、创建时间和 ID 核对。
5. 启动本地测试容器时保留旧容器或旧镜像作为回退点，不删除已有数据卷。
6. 等待 /health 返回 200 和 {"status":"ok"}。

阶段 F：本地浏览器回归
使用浏览器真实打开本地候选环境，至少验证：
- 首页和后台配置的首页优先级；
- 登录、退出、会话过期行为；
- 普通用户控制台；
- 管理员仪表盘、用户、账号、渠道、分组、系统设置；
- 模型广场真实模型和分组价格；
- /pricing 自动跳转到 /model-plaza；
- /docs 和管理员文档维护；
- 后台上传站点 Logo 后，首页、登录页和控制台一致更新；
- 桌面与移动端无溢出、遮挡、空白 Canvas；
- 浏览器控制台没有应用错误。

计费专项验证：
- 核对 gpt-5.6-luna 和 gpt-5.6-terra 的官方基础价格仍采用当前业务要求；
- 核对 fast 模式按渠道基础计价的 2 倍结算；
- 核对 fast 倍率不会被渠道模型定价覆盖；
- 使用测试账号和可核对的最小请求验证账单前后差额，不使用生产用户 API Key；
- 如果业务价格要求已经变化，停止并向我确认，不能自行改价。

阶段 G：Git 提交、PR 和发布标签
1. 测试通过后提交升级分支，提交信息清楚说明官方版本和二开保留内容。
2. 推送 codex/sync-upstream-v<TARGET_VERSION> 到 origin。
3. 创建 PR，目标分支必须是 custom。
4. PR 检查：
   - 官方提交完整进入；
   - Vote AI 差异只集中在隔离目录、文档后端和带 CUSTOM 标记的小型接入点；
   - 没有本地数据、日志、镜像、备份、密钥和 .env；
   - CI 和本地测试均通过。
5. PR 合并后更新本地 custom，并确认合并提交包含升级分支。
6. 只有已测试提交进入 custom 后，才创建 annotated tag：custom-v<TARGET_VERSION>-<RELEASE_SEQ>。
7. 推送 custom 和发布标签。确认 GitHub 上分支、提交和标签均存在。

阶段 H：生产升级前只读审计
连接生产服务器后先只读检查，不要立刻部署：
- uname -m、磁盘空间、时间；
- Docker 和 Compose 版本；
- compose ls、运行容器、健康状态；
- 实际 Compose 文件组合、服务名和当前镜像；
- 应用、PostgreSQL、Redis 的挂载类型和路径；
- 数据库大小、公共表数量和关键表行数；
- 当前公网和本机 /health 状态。

禁止输出 docker inspect 的完整 Env 数组或 .env 内容。

阶段 I：生产备份与可恢复验证
在服务仍在线时完成：
1. PostgreSQL custom-format pg_dump；
2. PostgreSQL globals 备份；
3. /app/data、Compose 文件、.env 和 Redis 持久化文件归档；
4. 记录升级前镜像 ID、关键表行数和容器状态；
5. 对所有备份生成 SHA-256；
6. 使用 pg_restore --list 验证备份目录；
7. 把备份实际恢复到临时数据库，核对 users、accounts、api_keys、channels、usage_logs、payment_orders、settings 等关键表；
8. 验证后删除临时数据库；
9. 记录备份路径和恢复验证结果。

在线备份期间 usage_logs 仍可能增长。恢复库的 usage_logs 少量低于稍后读取的生产行数属于正常时间差，但用户、账号、密钥、渠道、订单和设置应一致。

没有通过实际恢复验证，不得进入生产切换。

阶段 J：镜像传输和生产切换
1. 确认候选镜像 OS/Architecture 与服务器一致。
2. 通过 GHCR 拉取或通过 docker save/scp 传输镜像。
3. 传输前后核对 SHA-256；不一致立即停止。
4. docker load 后核对 RepoTag、镜像 ID、架构和创建时间。
5. 备份生产 Compose 覆盖文件。
6. 修改镜像标签后，先执行 docker compose config --quiet 和 docker compose config --images。
7. 必须确认 Compose 最终解析出的应用镜像就是目标镜像。
8. 如果使用 docker-compose.custom.yml，必须显式使用两个 -f，或在 .env 固化：
   COMPOSE_FILE=docker-compose.yml:docker-compose.custom.yml
9. 只重建应用服务：
   docker compose up -d --no-deps --pull never <APP_SERVICE>
10. 不重启 PostgreSQL 和 Redis，不删除卷，不覆盖生产 config.yaml 和 .env。
11. 等待目标镜像容器 healthy，并核对实际 .Config.Image 和 .Image ID；不能只看 HTTP 200。

绝对禁止：
- docker compose down -v
- docker volume rm
- 删除 postgres_data、redis_data 或 /app/data
- 用本地 .env、config.yaml 或数据库目录覆盖生产文件
- 未备份验证就运行可能迁移数据库的新版本
- 镜像不正确但因为 health=200 就宣布成功

阶段 K：生产验收
至少完成：
- 应用、PostgreSQL、Redis 均 healthy，重启次数正常；
- /health、本机入口和公网入口返回 200；
- 升级后关键表行数不低于升级前；
- schema_migrations 状态合理；
- 真实登录和 /api/v1/auth/me 成功；
- 使用专用测试账号完成一个最小模型请求和账单核对；
- 首页、模型广场、/pricing、文档页、管理员后台浏览器回归；
- 核对 luna、terra 展示价格和 fast 模式实际计费；
- 检查容器启动以来 panic、fatal、error 日志；
- 观察一段时间，确认真实请求仍成功且 usage_logs 持续增长。

阶段 L：回退资产和最终报告
1. 保留上一个生产镜像，不要立即清理。
2. 保留数据库备份、数据归档、旧 Compose 配置和新镜像传输包或可重新获取的发布标签。
3. 写入 ROLLBACK.txt，包含应用层回退命令和数据库恢复位置。
4. 说明应用镜像回退不一定能回退数据库迁移；涉及不兼容迁移时必须恢复数据库备份。
5. 最终报告必须列出：
   - 官方起止 commit；
   - 二开分支、提交和发布标签；
   - 测试结果；
   - 镜像标签和 ID；
   - 生产备份路径；
   - 数据升级前后对比；
   - 浏览器验证结果；
   - 回退路径；
   - 失败、异常、临时修正和剩余风险，不能隐瞒。

任何阶段出现以下情况必须停止升级并报告：
- 当前生产标签不属于 origin/custom；
- 官方目标版本或 commit 无法确认；
- 工作区存在会被覆盖的未知修改；
- 合并冲突无法确认业务语义；
- 二开专项测试、类型检查、构建或核心后端测试失败；
- 本地镜像不是新构建或架构错误；
- 数据库备份为空、校验失败或无法恢复；
- 生产挂载位置、数据库容器或 Compose 文件组合不明确；
- 镜像校验和、实际镜像 ID 或目标标签不一致；
- 升级后关键业务数据减少；
- 新容器不健康、反复重启或出现严重错误。
````

## 5. Vote AI 二开不变量

每次升级后都应重新验证这些约束，不能只依赖 Git 自动合并成功。

### 5.1 二开拥有的代码

主要集中在：

```text
frontend/src/custom/vote-ai/
backend/internal/service/setting_docs.go
backend/internal/handler/setting_handler.go
backend/internal/handler/admin/setting_handler.go
backend/internal/server/routes/auth.go
backend/internal/server/routes/admin.go
```

官方文件里的接入位置使用以下标记：

```text
CUSTOM(VOTE-AI-HOME)
CUSTOM(VOTE-AI-DOCS)
CUSTOM(VOTE-AI-THEME)
CUSTOM(VOTE-AI-BUILD)
CUSTOM(VOTE-AI-BRANDING)
```

### 5.2 当前应该保留的功能

- Vote AI 品牌首页和交互式地球；
- 可由管理员维护的站内文档；
- 后台站点 Logo 驱动的品牌显示；
- Vote AI 主题与构建接入；
- `/pricing` 兼容跳转到官方 `/model-plaza`；
- 二开专项自动化测试。

### 5.3 当前不应恢复的功能

- 独立静态模型定价页面；
- `PricingView.vue`；
- `pricing-data.ts`；
- 在源码中写死站点 Logo；
- 与官方模型广场重复的数据维护逻辑。

模型价格展示应由官方模型广场读取后台真实配置。实际扣费必须由后端计费逻辑和测试账单验证，不能只看前端展示。

## 6. 快速人工检查清单

### 合并前

- [ ] 当前生产版本已提交并有标签；
- [ ] 生产标签属于 `origin/custom` 历史；
- [ ] 工作区无未知修改；
- [ ] `origin`、`upstream` URL 正确；
- [ ] 官方目标版本、commit、`VERSION` 一致；
- [ ] `rerere.enabled=true`。

### 合并后

- [ ] 无未解决冲突；
- [ ] `CUSTOM(VOTE-AI-*)` 标记完整；
- [ ] 没有恢复静态定价页；
- [ ] 二开差异仍集中；
- [ ] 前后端测试通过；
- [ ] 新 Docker 镜像已本地运行；
- [ ] 浏览器功能和计费专项验证通过。

### 生产前

- [ ] PR 已合并到 `custom`；
- [ ] 发布标签指向 `custom` 内的已测试提交；
- [ ] 生产只读审计完成；
- [ ] PostgreSQL 备份已实际恢复验证；
- [ ] 数据和配置归档已校验；
- [ ] 新镜像传输 SHA-256 一致；
- [ ] Compose 解析出的实际目标镜像正确；
- [ ] 已记录应用层和数据库层回退方法。

### 生产后

- [ ] 三个服务健康；
- [ ] 实际运行镜像标签和 ID 正确；
- [ ] 关键表数据未减少；
- [ ] 公网、登录、API 和浏览器页面正常；
- [ ] 模型价格和 fast 计费正常；
- [ ] 严重错误日志为零或已解释；
- [ ] 旧镜像、备份和回退记录仍保留。

## 7. 为什么这些关卡不能省略

### 7.1 Git 合并成功不等于功能正确

Git 只判断文本是否冲突。官方可能修改了路由、首页优先级、设置结构或依赖接口，即使没有文本冲突，也可能让 Vote AI 功能失效。

### 7.2 HTTP 200 不等于镜像正确

Compose 如果没有加载覆盖文件，可能启动基础文件里的旧镜像，而且旧镜像同样会返回健康状态。生产切换后必须核对容器的 `.Config.Image` 和镜像 ID。

### 7.3 有备份文件不等于可以恢复

备份可能为空、损坏、权限错误或缺少对象。必须执行 `pg_restore --list`，并恢复到临时数据库核对关键表。

### 7.4 切回旧镜像不一定能回退数据库

新版本启动时可能自动执行迁移。若迁移不向后兼容，仅切换旧镜像可能无法运行，必须使用升级前 PostgreSQL 备份恢复。

### 7.5 页面价格不等于实际计费

模型广场展示值、渠道定价、分组倍率、用户倍率和 fast 模式可能经过不同计算路径。价格调整必须同时验证页面展示和测试请求的实际账单差额。

## 8. 文档维护要求

每次升级完成后更新本文中的：

- 当前生产版本和一次性分支提醒；
- 远程仓库或长期分支策略；
- 二开拥有的文件和 `CUSTOM` 标记；
- 已删除或由官方替代的二开功能；
- 测试命令；
- 生产 Compose 布局和已知风险；
- 新版本迁移和回退注意事项。

同时为目标版本更新或新建差异说明：

```text
docs/CUSTOMIZATION_DIFF_V<TARGET_VERSION>_CN.md
```

差异说明必须以当前 Git 实际比较结果为准，不能复制旧版本的文件数量、测试数量或功能描述。
