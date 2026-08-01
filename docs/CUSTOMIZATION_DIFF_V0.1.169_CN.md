# Sub2API 二开版本与官方版本差异说明

## 1. 文档目的

本文说明当前 Vote AI 二开版本与 Sub2API 官方版本之间的实际代码差异，用于：

- 判断哪些功能属于二开，后续升级时必须保留；
- 区分品牌展示改造与官方核心网关能力；
- 为后续合并官方新版本、测试、发布和回滚提供检查依据；
- 避免把本地部署文件、临时产物或未纳入 Git 的素材误认为二开源码。

本文基于 2026-08-01 本地仓库的 Git 对比结果生成。

## 2. 对比基线

| 项目 | Git 引用 | 提交 |
| --- | --- | --- |
| 当前二开版本 | `codex/sync-upstream-v0.1.169` | `78e3e68e1981bbd06b9f4f493df1ebcd532a83a8` |
| 官方版本 | `upstream/main` | `682c4fe0e61b851508fa976ac693e0f68a0639eb` |
| 二开合并前第一父提交 | `f9ba5eb01246d6342a56598ddf0f454460dbe1d6` | Windows 测试稳定性修复 |
| 应用版本号 | `backend/cmd/server/VERSION` | `0.1.169` |

当前提交 `78e3e68e` 是一个标准双父合并提交：一侧是二开历史，另一侧是官方 `0.1.169` 最新代码。官方提交已经完整进入当前分支，所以下文列出的内容就是当前树相对官方树仍然保留的二开差异。

对比命令：

```powershell
git diff --stat upstream/main..HEAD
git diff --name-status upstream/main..HEAD
```

对比结果：

- 差异文件：29 个；
- 新增：2593 行；
- 删除：572 行；
- 未跟踪的 `assets/branding/` 本地素材不在本次对比和 Git 提交中。

上述 29 个文件统计以提交 `78e3e68e` 为准，不包含本文档本身及为跟踪本文档而增加的 `.gitignore` 白名单。

## 3. 总体结论

当前二开并没有整体重写 Sub2API 的账号调度、计费、渠道、鉴权或 AI 网关核心。主要差异集中在四个方面：

1. Vote AI 品牌化公开首页；
2. 独立的静态模型价格页；
3. 可由管理员维护的站内文档系统；
4. 二开 Docker 镜像的 GitHub Actions 发布流程。

其余大量后端功能仍以官方实现为主，包括账号管理、渠道管理、订阅、支付、请求转发、模型兼容、用量统计、风险控制和系统设置等。

## 4. 功能差异总览

| 领域 | 官方版本 | 当前二开版本 |
| --- | --- | --- |
| 默认公开首页 | 官方 Sub2API 产品首页 | Vote AI 品牌官网式首页 |
| 首页内容 | 通用站点介绍 | Vote AI 定位、套餐、价值说明、FAQ 和全球节点地球仪 |
| 首页文档入口 | 使用可配置的外部 `doc_url` | 固定进入站内 `/docs` |
| 价格页面 | 无独立 `/pricing` 页面 | 新增 Codex 静态分组价格页 |
| 文档页面 | 无内置可管理文档系统 | 新增 `/docs/:slug?`，支持中英文 Markdown |
| 文档管理 | 依赖外部文档地址 | 管理员可在文档页新增、编辑、排序、发布和删除 |
| 文档接口 | 无对应接口 | 新增公开读取及管理员读写接口 |
| 视觉主题 | 青色与冷灰色 | 暖橙、暖灰和暖黑色体系 |
| 首页视觉组件 | 官方装饰背景 | 可拖动、自动旋转的全球节点地球仪 |
| 镜像发布 | 官方发布流程 | 新增 `custom-v*` 标签触发的 GHCR 发布流程 |
| 核心网关与数据库结构 | 官方实现 | 基本沿用官方，本次二开没有新增数据库迁移 |

## 5. Vote AI 公开首页

### 5.1 首页显示优先级

二开版本保留了官方可配置首页机制，但重新设计了默认页面。实际显示优先级为：

1. 后台设置了 `home_content`：显示自定义 HTML，或者将 URL 作为全屏 iframe；
2. 未设置 `home_content`，但启用了 `compact_home_enabled`：显示官方简洁首页模式；
3. 两项都未启用：显示 Vote AI 默认品牌首页。

因此，Vote AI 首页不是无条件显示。如果后台配置了自定义首页内容，它仍会被更高优先级设置覆盖。

主要文件：

- `frontend/src/views/HomeView.vue`
- `frontend/src/views/__tests__/HomeView.compact.spec.ts`

### 5.2 默认首页新增内容

Vote AI 默认首页包含：

- Vote AI 品牌名称和动态站点 Logo；
- 中文、英文双语内容；
- 日间和夜间主题；
- 登录状态识别，管理员进入管理后台，普通用户进入用户控制台；
- “全球 AI API 网关”主视觉；
- Claude、OpenAI、Gemini 等接入定位说明；
- 按量付费、Claude、OpenAI 三类展示卡片；
- 国内直连、高可用、简单集成、兼容官方 API 等价值说明；
- 接入、计费、模型、订阅和 API 使用相关 FAQ；
- 桌面端和移动端响应式导航。

### 5.3 交互式全球节点地球仪

二开新增 `InteractiveGlobe.vue`，并引入 `cobe@0.6.5`。主要能力包括：

- Canvas 地球渲染；
- 自动旋转；
- 鼠标或触控拖动；
- 惯性旋转；
- 明暗主题切换；
- Vote、China、Japan、USA、Australia 节点标签；
- 节点连接线和地球背面渐隐；
- 根据容器宽度和设备像素比自适应渲染。

相关文件：

- `frontend/src/components/home/InteractiveGlobe.vue`
- `frontend/package.json`
- `frontend/pnpm-lock.yaml`

### 5.4 文档入口行为变化

官方首页根据后台 `doc_url` 决定是否显示外部文档链接。二开首页和简洁首页改为固定使用站内路由 `/docs`。

影响如下：

- 后台即使配置了外部 `doc_url`，Vote AI 首页上的文档按钮仍进入站内文档；
- `doc_url` 配置没有从系统中删除，其他官方页面仍可能使用它；
- 后续合并 `HomeView.vue` 时，不能把站内 `/docs` 意外恢复成外部链接。

## 6. 独立模型价格页

二开新增公开路由：

```text
/pricing
```

主要文件：

- `frontend/src/views/PricingView.vue`
- `frontend/src/router/index.ts`

### 6.1 页面能力

- Vote AI 品牌导航；
- 中文和英文切换；
- 日间和夜间主题；
- 桌面端表格和移动端适配；
- 模型 ID 一键复制；
- 分组切换和价格自动计算；
- 登录状态对应的控制台入口。

### 6.2 当前展示模型

当前价格页只展示 Codex 模型族，模型数据直接写在前端：

- `gpt-5.6-sol`
- `gpt-5.6-terra`
- `gpt-5.6-luna`
- `gpt-5.5`
- `gpt-5.4`
- `gpt-5.4-mini`

展示的价格列包括输入、输出和缓存读取价格。

### 6.3 当前分组倍率

| 分组 | 倍率 | 页面显示含义 |
| --- | ---: | --- |
| 特惠 | `0.05` | 官方 API 展示价的 5% |
| Plus | `0.12` | 官方 API 展示价的 12% |
| Pro | `0.20` | 官方 API 展示价的 20% |

计算方式：

```text
页面分组价格 = 前端内置的官方展示价 × 分组倍率
```

重要限制：这个页面目前是静态展示页，没有读取后台渠道定价、用户分组、密钥倍率或实际账单。最终扣费仍以控制台和后端计费记录为准。模型价格或倍率发生变化时，需要修改 `PricingView.vue` 并重新构建前端。

## 7. 站内文档系统

这是二开版本中最完整的一项前后端功能扩展。

### 7.1 路由与接口

前端公开路由：

```text
/docs
/docs/:slug
```

后端接口：

| 方法 | 路径 | 权限 | 用途 |
| --- | --- | --- | --- |
| `GET` | `/api/v1/docs` | 公开 | 只返回已发布文章 |
| `GET` | `/api/v1/admin/docs` | 管理员 | 返回已发布和草稿文章 |
| `PUT` | `/api/v1/admin/docs` | 管理员 | 整体保存文档列表 |

主要后端文件：

- `backend/internal/service/setting_docs.go`
- `backend/internal/handler/setting_handler.go`
- `backend/internal/handler/admin/setting_handler.go`
- `backend/internal/server/routes/auth.go`
- `backend/internal/server/routes/admin.go`

主要前端文件：

- `frontend/src/api/docs.ts`
- `frontend/src/views/DocsView.vue`
- `frontend/src/components/docs/MarkdownContent.vue`

### 7.2 数据存储

文档没有新增数据表，而是将整个文档数组序列化为 JSON，写入系统设置表中的固定键：

```text
docs_content
```

这意味着：

- 不需要新增数据库迁移；
- 现有数据库备份会连同文档一起备份；
- 恢复数据库时也会恢复文档内容；
- 文档数量和总体积适合中小规模接入说明，不适合作为大型知识库。

### 7.3 默认文档

当数据库中不存在有效的 `docs_content` 时，系统提供 4 篇中英文默认文章：

1. 快速开始；
2. 获取 API Key；
3. 客户端接入；
4. 常见问题。

### 7.4 管理能力

管理员登录后访问 `/docs`，页面会自动调用管理员接口，并提供：

- 新增文章；
- 修改 Slug；
- 中英文标题；
- 中英文 Markdown 正文；
- 实时预览；
- 草稿和发布切换；
- 上移、下移排序；
- 删除确认；
- 保存失败时恢复页面原数据。

普通访客只能看到 `published=true` 的文章和公开阅读界面。

### 7.5 输入校验和安全限制

后端限制包括：

- 单次请求体最大 512 KiB；
- 最多 50 篇文章；
- ID 仅允许字母、数字、下划线和连字符；
- Slug 仅允许小写字母、数字和单个连字符分段；
- ID 和 Slug 不允许重复；
- 中英文标题与正文均不能为空；
- 标题最大 200 个字符；
- 单个语言正文最大 100000 个字符；
- JSON 出现未知字段或多余内容时拒绝保存。

Markdown 渲染流程为：

```text
Markdown -> marked 生成 HTML -> DOMPurify 清理 -> 页面展示
```

渲染时禁止 `iframe`、`object`、`embed` 和内联 `style`，代码块提供复制按钮。

## 8. 视觉主题和构建配置

### 8.1 全局颜色体系

二开修改了 Tailwind 颜色配置：

- 主色从青色改为暖橙棕色；
- 灰阶改为暖灰和暖白；
- 深色模式改为暖黑棕色；
- 阴影、发光和渐变随品牌色调整；
- HTML 和 Body 默认背景改为暖白，暗色背景改为暖黑。

相关文件：

- `frontend/tailwind.config.js`
- `frontend/src/style.css`

由于 `primary`、`gray` 和 `dark` 是全局 Tailwind 色板，这项变化不只影响公开首页，也可能影响管理后台和用户控制台。后续升级时应对后台页面做一次视觉回归检查。

### 8.2 前端配置调整

- `postcss.config.js` 显式指定前端目录内的 Tailwind 配置；
- `vite.config.ts` 固定从 `frontend` 目录加载环境变量，避免从仓库根目录启动时读取错误位置；
- 新增 `cobe` 依赖用于地球仪。

## 9. 自定义 Docker 镜像发布流程

二开新增：

```text
.github/workflows/custom-image.yml
```

工作流行为：

1. 推送格式为 `custom-v<版本>-<序号>` 的标签时触发；
2. 校验标签中的版本与 `backend/cmd/server/VERSION` 一致；
3. 校验被打标签的提交属于远程 `custom` 分支历史；
4. 使用 Docker Buildx 构建 `linux/amd64` 镜像；
5. 使用 GitHub Actions 自动提供的 `GITHUB_TOKEN` 登录 GHCR；
6. 将镜像推送到 `ghcr.io/<仓库所有者>/<仓库名>:<Git标签>`；
7. 使用 GitHub Actions 缓存加快后续构建。

注意：当前工作流明确执行 `git fetch origin custom`，并要求标签提交属于 `origin/custom`。当前同步分支名为 `codex/sync-upstream-v0.1.169`。如果在 `bupiter/sub2api` 中不再使用 `custom` 作为长期二开主分支，直接打标签会导致工作流校验失败。正式发布前应先将 PR 合并到 `custom`，或者同步修改工作流中的目标分支规则。

当前工作流只构建 `linux/amd64`，不包含 ARM64 镜像。

## 10. 测试差异

二开新增的核心测试：

- 文档服务默认值、草稿过滤、顺序保持和保存规范化；
- 重复 Slug、非法 ID 和非法 Slug 拒绝逻辑；
- 管理员文档更新接口；
- 未知 JSON 字段和超大请求体拒绝逻辑；
- Vote AI 默认首页和简洁首页优先级；
- 站内文档入口固定为 `/docs`；
- 文档页 Logo URL 清理和管理员文档加载逻辑。

部分测试修改不属于新业务功能，而是为了适配官方升级后的接口和运行环境：

- 回滚 API 测试适配 15 分钟超时参数；
- 分组页面测试补充 `getLiveCapability` Mock；
- 用户资料测试补充 Passkey Mock；
- 内容审核缓存测试显式构造过期快照，使 Windows 环境测试稳定。

## 11. 差异文件清单

### 11.1 CI/CD

- `.github/workflows/custom-image.yml`

### 11.2 后端文档功能

- `backend/internal/service/setting_docs.go`
- `backend/internal/service/setting_docs_test.go`
- `backend/internal/handler/setting_handler.go`
- `backend/internal/handler/admin/setting_handler.go`
- `backend/internal/handler/admin/setting_docs_handler_test.go`
- `backend/internal/server/routes/auth.go`
- `backend/internal/server/routes/admin.go`

### 11.3 前端公开站点

- `frontend/src/views/HomeView.vue`
- `frontend/src/views/PricingView.vue`
- `frontend/src/views/DocsView.vue`
- `frontend/src/components/home/InteractiveGlobe.vue`
- `frontend/src/components/docs/MarkdownContent.vue`
- `frontend/src/api/docs.ts`
- `frontend/src/router/index.ts`

### 11.4 前端主题和构建

- `frontend/package.json`
- `frontend/pnpm-lock.yaml`
- `frontend/postcss.config.js`
- `frontend/tailwind.config.js`
- `frontend/vite.config.ts`
- `frontend/src/style.css`

### 11.5 测试兼容性调整

- `backend/internal/service/content_moderation_runtime_cache_test.go`
- `frontend/src/api/__tests__/admin.system.rollback.spec.ts`
- `frontend/src/components/layout/__tests__/docUrlSanitization.spec.ts`
- `frontend/src/components/layout/__tests__/siteLogoSanitization.spec.ts`
- `frontend/src/views/__tests__/HomeView.compact.spec.ts`
- `frontend/src/views/admin/__tests__/GroupsView.columnSettings.spec.ts`
- `frontend/src/views/admin/__tests__/GroupsView.duplicate.spec.ts`
- `frontend/src/views/user/__tests__/ProfileView.spec.ts`

## 12. 数据库和接口影响

### 12.1 数据库

- 二开没有新增迁移文件；
- 当前数据库最新迁移仍为 `191_passkey_credentials.sql`；
- 文档内容存储在现有设置表，不创建新表；
- 从官方版本切换到二开版本时不需要单独执行二开数据库迁移。

### 12.2 新增接口影响

新增的 `/api/v1/docs` 是公开接口，只返回已发布文章。管理员接口位于既有管理员路由组中，继续使用官方管理员认证和授权中间件。

### 12.3 未修改的主要后端领域

相对官方当前树，二开没有保留以下领域的业务源码差异：

- AI 网关核心转发；
- OpenAI、Claude、Gemini 等协议兼容；
- 账号调度；
- 渠道和分组管理；
- 用户、订阅和支付核心流程；
- 用量统计和计费核心；
- 数据库迁移体系。

这些领域会随官方升级直接更新，但仍需要回归测试，避免公开站点改动与官方公共组件发生间接冲突。

## 13. 已知维护风险

### 13.1 高冲突文件

后续同步官方版本时，以下文件最容易产生冲突：

1. `frontend/src/views/HomeView.vue`：官方首页与 Vote AI 首页在同一文件；
2. `frontend/src/router/index.ts`：官方持续新增或调整路由；
3. `frontend/tailwind.config.js`：二开覆盖全局颜色；
4. `backend/internal/handler/admin/setting_handler.go`：官方设置功能较多；
5. `backend/internal/server/routes/admin.go` 和 `auth.go`：官方路由经常变化；
6. `frontend/package.json` 和 `pnpm-lock.yaml`：官方依赖升级时可能冲突。

### 13.2 硬编码品牌

Vote AI 默认首页、价格页和文档页直接显示 `Vote AI`。后台修改 `site_name` 不会完整替换这些页面中的品牌文字；简洁首页仍使用动态 `site_name`。

### 13.3 静态价格漂移

价格页的数据和倍率是前端硬编码，不会自动跟随后端定价配置、官方模型价格或用户实际分组更新。任何价格调整都必须同步修改源码、测试并重新构建。

### 13.4 文档整体覆盖写入

管理员保存文档时使用 `PUT` 整体覆盖文档数组，不是逐篇写入。多个管理员同时编辑时，最后一次保存可能覆盖之前的改动。当前实现适合低并发管理场景。

### 13.5 CI 分支约束

自定义镜像工作流依赖 `origin/custom`。仓库分支策略变化后必须同步修改，否则标签发布会失败。

## 14. 本次版本验证记录

当前 `0.1.169` 二开版本已完成：

- 前端 lint 通过；
- 前端 TypeScript 类型检查通过；
- 前端生产构建通过；
- 197 个 Vitest 文件通过；
- 1369 项前端测试通过；
- 后端管理员处理器、路由和服务核心测试通过；
- Docker 镜像 `sub2api-custom:0.1.169` 构建并运行；
- `/health` 返回 `{"status":"ok"}`；
- 后端版本显示 `Sub2API 0.1.169`；
- Vote AI 首页、价格页、文档页通过桌面浏览器验证；
- 首页、价格页和文档页通过 390 x 844 移动端宽度验证；
- 管理后台仪表盘、用户、系统设置、渠道和账号页面正常；
- 数据库保留 1 个管理员用户；
- 浏览器控制台未发现警告或错误。

模型广场属于官方已有功能，不是上述 29 个二开差异文件的一部分。当前本地配置关闭了模型广场，因此访问 `/model-plaza` 会按官方路由规则返回首页。

## 15. 后续升级建议

每次同步官方新版本时，建议执行以下顺序：

1. 为当前二开版本创建 Git 提交和标签；
2. 备份 PostgreSQL 数据库；
3. `git fetch upstream` 获取官方更新；
4. 从二开长期分支创建新的同步分支；
5. 合并官方 `upstream/main`；
6. 按“自定义首页优先级 -> 简洁首页 -> Vote AI 默认首页”解决首页冲突；
7. 保留 `/pricing`、`/docs/:slug?` 和文档后端接口；
8. 确认首页文档入口仍指向站内 `/docs`；
9. 检查价格模型及倍率是否仍有效；
10. 执行前端 lint、类型检查、生产构建和完整测试；
11. 执行后端核心测试；
12. 构建 Docker 镜像并使用备份数据进行本地验证；
13. 浏览器验证公开页、移动端和管理后台；
14. 创建合并提交并推送 GitHub；
15. 合并到长期 `custom` 分支后再创建 `custom-v*` 发布标签。

## 16. 快速复核命令

查看当前版本与官方版本的差异：

```powershell
git fetch upstream
git diff --stat upstream/main..HEAD
git diff --name-status upstream/main..HEAD
```

只查看二开公开站点差异：

```powershell
git diff upstream/main..HEAD -- frontend/src/views frontend/src/components/home frontend/src/components/docs frontend/src/api/docs.ts
```

只查看二开后端文档接口差异：

```powershell
git diff upstream/main..HEAD -- backend/internal/service/setting_docs.go backend/internal/handler backend/internal/server/routes
```

检查工作区是否混入本地文件：

```powershell
git status --short
```

本文件应在每次官方版本同步完成后更新对比提交、文件统计、价格数据、测试结果和已知风险。
