# Sub2API 0.2.10 合并候选审核

本候选已获用户明确认可：“认可，继续部署雨云”。实际本地提交、备份与部署记录保存在任务目录 `work/sub2api-sync-v0210-stage/`。此次不推送或创建 PR，原工作区继续保留。

## 版本与范围

- 官方目标：Wei-Shaw/sub2api 的 main，`a60a29549f488a854966aaec9541abbe006cac22`，VERSION 0.2.10。
- 上次已集成官方提交：`9a62841fd124d026cf3694fcf9b79e98addcdbdc`。
- 官方增量：33 个提交、118 个文件；无数据库迁移、Ent schema、Go/前端依赖或部署配置变化。
- 定制基线：`8680ea7787b428f08172b5052262afbbedec4911`，另以本地 checkpoint `ac57d2098` 保存待提交的密钥选择区改动。
- 候选分支：`sync/upstream-v0.2.10-rain-20260930`；备份分支：`backup/rain-before-v0.2.10-20260930`。

## 五个冲突文件的处理

| 文件 | 最终处理 |
| --- | --- |
| `backend/cmd/server/wire_gen.go` | 保留定制 TLS 指纹路由器，新增官方 Claude 重置额度查询服务和组合路由解析器依赖。 |
| `backend/internal/handler/openai_gateway_handler.go` | 保留风控上下文和管理员解封 epoch；WebSocket 使用官方组合分组映射模型；新白名单只记录时跳过连接处罚。 |
| `backend/internal/service/content_moderation.go` | 保留幂等日志、审核上下文与异步生命周期；合入白名单只记录逻辑，防止产生风险计数、哈希屏蔽、封禁、邮件或会话处罚；判决缓存按白名单状态隔离。 |
| `backend/internal/repository/content_moderation_repo_test.go` | 同时保留定制审核测试与官方只记录日志排除测试。 |
| `frontend/src/components/keys/__tests__/UseKeyModal.spec.ts` | 保留 RikkaHub 移除、定制模型和目录测试，并加入官方 Claude Code-only 限制用例。 |

## 自动合并后的额外兼容处理

- 新引导同样遵守 `claude_code_only`，并把限制传入手动配置窗口；保持用户明确选择的应用，不静默替换。
- 普通 OpenAI 手动配置按官方移除模型目录；国模 OAI 通过明确的窄参数保留自己的模型目录，在读取成功前不生成猜测模型配置。
- 国模 OAI Windows 脚本显式按 UTF-8 解码 Codex 标准输出/错误，修复原版本同样存在的中文路径 JSON 解析失败；仅使用隔离测试目录验证，没有修改本机真实 Codex 配置。
- 保留这几轮的“开始使用”、同页创建密钥、主按钮与密钥选择卡片；原生和固定输出生图分组、CPA、价格与上游凭证不作配置修改。

## 验证结果

- 前端：36 个定向文件，共 472 项测试通过；TypeScript 检查及生产前端构建通过。
- 后端：401 个顶层测试，含子测试共 932 个唯一通过项，无失败、无跳过；额外 apicompat/claude 兼容库测试及 API 契约检查通过。
- 服务端入口编译通过；Linux 嵌入构建结果见任务目录的 backend-build.log。
- 未执行付费模型调用、真实账号注册、密钥创建或客户端配置写入。

## 雨云部署与回滚条件

目前仍运行 `sub2api-local:quick-connect-v4-20260930`，应用健康，完整 12 文件 Compose 链已核实。PostgreSQL 与备份工具为 18.6。

新增白名单当前未配置，`cyber_log_only` / `risk_control_log_only` 日志数量为 0。部署脚本会先保存新的数据库、完整 Compose/nginx 配置、应用 data 目录和原镜像，仅重建 app，并检查后重载现有 web nginx。

本轮无 schema 改动，可在上述状态下回切旧镜像。若测试时启用新白名单并生成只记录日志，旧版违规计数语义不同，脚本会拒绝直接回切；届时应单独审核恢复数据库或保留新版修复，不能盲目丢弃新数据。

依据仓库 `CUSTOM_UPGRADE.md` 的冲突审核要求，已获得负责人对本表处理的认可，可完成本地合并并部署雨云。此次不创建或更新 PR、不推送、不部署 OVH。
