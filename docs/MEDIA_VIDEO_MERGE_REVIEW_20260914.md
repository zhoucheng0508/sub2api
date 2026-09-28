# 视频功能合并审核（2026-09-14）

## 当前状态

工作分支：`codex/release-media-video-v0.2.4`。原始视频改动已保存为 `91d61586e`；升级前主线备份为 `backup/custom-before-video-v0.2.4-20260914`，视频改动备份为 `backup/video-before-sync-20260914`。

合并目标：`origin/custom` 的 `c4c7f91a1056a3a483e8713a8badf0d41f94c80f`（0.2.4，已发布 `custom-v0.2.4-2`）。双方交叉修改 48 个文件，其中 8 个发生文本冲突。当前冲突已解决，合并提交尚未完成，未推送、创建发布标签或部署。

## 八个冲突文件的处理

| 文件 | 解决结果 |
| --- | --- |
| `backend/ent/runtime/runtime.go` | 采用 0.2.4 的 `model_allowlist`，保留视频单价后正确偏移字段位置；隔离重新生成 Ent 后结果完全一致。 |
| `backend/internal/handler/admin/group_handler.go` | 创建/更新分组同时允许 Laogou 和 MiniMax；保留视频单价、新版模型白名单及 Simple 模式处理。 |
| `backend/internal/server/routes/gateway.go` | 沿用 0.2.4 的 `rootRoute`，另行注册六个 MediaVideo 路由；继承分组鉴权、模型白名单等中间件，移除原视频路由重复挂载的同一组中间件。 |
| `backend/internal/service/composite_platform_test.go` | 调度目录断言同时包含 Laogou 和 MiniMax。Laogou 并未因此加入通用复合路由目标白名单。 |
| `backend/internal/service/plugin_package.go` | 上游已经提供 Windows 关闭 ZIP 句柄的修复，采用上游统一的 `closeArchive`，该文件相对 0.2.4 无额外差异。 |
| `backend/internal/service/plugin_package_test.go` | 保留普通文件检查，仅在非 Windows 平台检查 Unix 执行权限。 |
| `backend/internal/service/scheduler_snapshot_service.go` | 保留两个新增平台，调度目录长度更新为 10，批量账号更新也涵盖两者。 |
| `frontend/src/types/index.ts` | 账号、分组平台联合类型同时包含 Laogou 和 MiniMax，保留视频报价及 0.2.4 新字段。 |

## 额外收尾

- 设置页的现有测试隔离独立保存的视频下载卡片，避免该卡片的 Pinia/API 生命周期干扰父表单用例；生产组件逻辑未因测试而弱化。
- 供应商探测工具不再重发结果不确定的创建请求，关闭自动重定向，模型查询失败及不合格下载探测返回非零退出码。
- 发布说明统一为 `custom` CI 通过后创建新标签，由 GitHub Actions 构建 GHCR 镜像；候选标签 `custom-v0.2.4-3`，发布前须再次检查占用情况。
- 保留视频 235—239 的完整迁移文件名及内容。远端同号迁移涉及模型白名单和 MiniMax，迁移器按完整文件名记录和校验，两个安装路径均实际通过。
- 修正 `handler.go` 格式；生成工具临时引入的依赖校验文件变动已还原，未加入额外依赖。

## 已验证

| 检查 | 本次结果 |
| --- | --- |
| 前端依赖锁定安装（pnpm 9） | 通过，锁文件未修改 |
| Vue 类型检查、ESLint、前端生产构建 | 通过 |
| 设置页、账号创建、分组复制、复合平台、平台目录、模型列表定向测试 | 6 个文件、94 项全部通过 |
| 后端 service/repository/handler/routes/middleware/cmd/server/migrations 定向执行及包编译 | 通过；未匹配用例的包仅计编译验证 |
| unit 标签鉴权、Recovery、模型白名单、共享余额定向回归 | 通过 |
| Ent/Wire | 在隔离副本重新生成，输出与合并结果一致 |
| Linux amd64 后端编译 | 嵌入本次新构建的前端资源，编译通过；这是合并检查产物，正式镜像仍由标签工作流生成 |
| 独立 PostgreSQL 16 全新安装 | 292 份迁移全部执行，逐文件校验和一致，重复启动通过 |
| 独立 PostgreSQL 16 模拟从 0.2.4 升级 | 先运行远端迁移，再补入视频迁移；292 份校验和及重复启动通过，模拟用户余额 37.25、冻结 0 保持不变 |
| 上述升级库备份恢复 | 恢复至第三个独立数据库，292 份迁移记录及模拟用户余额一致 |
| 供应商探测脚本离线检查 | 9 个检查通过，没有调用真实供应商 |
| 格式与冲突检查 | 无未合并文件；本次相对 `origin/custom` 的差异空白检查和 Go 格式检查通过 |

全量暂存差异包含远端新增提示词文本原有的行末空格；该文件与远端保持一致，未为清理上游格式而改写其内容。

本地日志位于 `outputs/video-release-*.log`，测试库备份 SHA-256：`44b1f679dbfbbd09d404f3ecc51d60c81c6f12f55e1a9da06b6bf2bc24403796`。日志、模拟数据库备份、生成副本及临时测试不随代码发布。独立数据库容器已停止，临时迁移测试源码已删除。

Docker 启动期间有两处不可访问的残留套接字文件，涉及推理服务与 Secrets Engine；同时备份并重建两处运行目录后，Docker 29.6.2 已正常响应且实际运行了上述测试数据库。没有恢复出厂设置，也没有删除原有镜像、数据库或数据卷。

## 审核后的发布安排

冲突说明和验证结果已提交负责人查看。负责人随后要求“按照当前项目的标准上线流程帮我完成上线提交”，本次据此继续完成 merge commit、推进 `custom`、等待该提交 CI，再创建新标签并验证最终镜像。

本次数据库验证使用模拟数据，不代表生产快照升级、生产入口、付费生成或实际上一镜像回退已验收。生产启用仍须按 `MEDIA_VIDEO_RELEASE.md` 完成相应检查。

## 提交前补充复核

供应商探测工具的两轮修复已纳入本次最终提交：58 个本地模拟场景通过；另独立复测 HTML、无效 JSON、截断、超长、无效范围及正常响应等 8 个场景全部通过。该数字不与早期 9 项检查累加宣称全量覆盖，没有调用真实供应商。

补充视频浏览器跨域请求头（幂等键、只读恢复、Range/If-Range）及下载响应头暴露；现有 CORS unit 回归通过。SettingsView 仅补充测试替身说明，生产组件未额外修改。此前所有生产环境验证边界保持不变。
