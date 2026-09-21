# v0.2.7 定制同步与冲突审核

日期：2026-09-21。依据根目录 CUSTOM_UPGRADE.md 执行，采用 merge，不改写 custom 历史。

## 基线与范围

- 生产及 custom 基线：custom-v0.2.4-4，5d24d82127fd9b8a408f74b8951bb7acf75b4202。
- 官方目标：v0.2.7，aea725f2ea644d5592d0bbb1d63b607efa7e200a。
- 升级分支：codex/sync-upstream-v0.2.7；独立工作区 C:/Users/Administrator/.codex/worktrees/sub2api-upgrade-v027/sub2api。
- 本地备份分支：backup/custom-before-v0.2.7-20260921-215728。
- 备份及验证记录：D:/sub2api-upgrade-prep/v0.2.7-20260921-215728。
- 原 D:/sub2api 工作区的 16 个未提交文件已单独复制和校验，未纳入本次从生产基线进行的合并；原文件哈希复核未变化。其中包含未发布的 Seedance 供应商适配和 Windows 一键安装器修改，后续发布范围应明确区分。

官方 v0.2.4 到 v0.2.7 共改变 541 个文件，与已提交定制内容交叉 90 个文件。保留 Vote AI 首页、首页优先级、上传 Logo、站内文档、模型广场跳转、账号 TLS 路由、提示缓存设置、内容审查、图片任务、视频钱包/账本/续传和一键接入功能。

## 冲突决策

| 文件或范围 | 处理方式 |
| --- | --- |
| .gitignore | 保留定制文档规则，加入官方归因说明文档例外 |
| backend/cmd/server/VERSION | 明确设为 0.2.7；官方标签中的 0.2.5 是 release 工作流注入前的值 |
| backend/go.mod | 保留定制已有 x/image v0.45.0，Go 1.27.0 工具链验证 |
| backend/cmd/server/wire_gen.go | 由 Wire 重新生成，合入官方插件 KV 依赖并保留定制依赖 |
| backend/internal/handler/admin/group_handler.go | Laogou 和官方 OpenCode 平台同时可用，保留单条视频价格字段 |
| backend/internal/handler/dto/settings.go | 同时保留 pass_user_context 和 hide_open_button |
| backend/internal/handler/grok_media.go | 先按官方协议解析 Seedance，再生成定制审查输入 |
| backend/internal/handler/openai_gateway_handler.go | 采用官方 XML 心跳校验，移除重复的旧版校验函数 |
| backend/internal/service/account_test_models_test.go | 保留官方模型显示名断言，使用定制 HTTP 传输测试桩 |
| backend/internal/service/openai_models_list_test.go | 同上，保证目录字段与共享缓存行为 |
| backend/internal/service/channel_monitor_checker.go | 保留内部探测签名及重定向防泄露，加入官方 URL 路径拼接修复 |
| backend/internal/service/composite_platform_test.go | 平台断言同时包含 Laogou 和 OpenCode |
| backend/internal/service/scheduler_snapshot_service.go | 保留两种平台的调度桶，平台数组扩为 11 项 |
| frontend/src/App.vue | 同时保留 Vote AI 品牌逻辑及官方计费/功能标志逻辑 |
| frontend/src/types/index.ts | 平台、提示缓存和官方 Seedance 能力类型合并 |
| frontend/src/views/admin/SettingsView.vue | 保留两个独立的自定义菜单选项 |
| frontend/src/views/user/KeysView.vue | 保留一键接入，同时合入官方分组分类和批量编辑；清理不再使用的导入 |
| frontend/src/views/user/__tests__/KeysView.spec.ts | 保留两种弹窗桩；消除自动合并产生的重复 actions 槽，保留原行为断言 |

## 自动合并复核及补充修复

- frontend/src/utils/keyGroupProviders.ts 补充 Laogou 分类，避免平台类型扩展后遗漏映射。
- 官方在生成文件中手工调用 SetAccountDirectory，Wire 再生成会丢失该调用。新增 ProvidePluginManager 将装配纳入源依赖图，再生成 wire_gen.go；插件能力门控测试通过正式 provider 构建实例，验证授权插件取得目录、未授权插件仍无法取得目录。
- 原定制账号弹窗缺失官方请求 ID 响应头和图片 URL 转 base64 选项；新官方编辑测试暴露这些遗漏。恢复创建/编辑界面、初始化、保存和中英文文案，同时保留提示缓存及 TLS 定制字段。保留 Laogou 禁用上游倍率探测的逻辑。
- 管理分组的模型候选请求桩适配官方接口名，防止测试挂载时出现无效 API 调用。
- 已执行的定制迁移 194—196、235—239 原文件保持不变；仅加入官方新迁移。

## 验证记录

详细结果及限制以备份目录中的 VALIDATION_RESULTS.md 为准。未运行本地全量测试，按固定流程运行 Makefile 前端关键用例、Vote AI 专项及交叉文件用例，后端运行文档、视频、模型、调度、内部探测、审查、提示缓存、TLS、插件及迁移相关定向用例。

Windows 一键安装器实机扩展测试执行未正常结束，已终止该测试进程且未记为通过；其源文件不在本次官方交叉改动范围，原工作区对此的未提交修改未合入。该工具的两个静态测试通过。后续若一并发布未提交的一键安装器修改，必须另行完成实机回归。

## 数据库和上线关卡

生产只读核验：应用 healthy，数据库和 Redis 正常，本机及公网 /health 正常，Nginx 配置通过；根分区可用约 34 GB，数据库约 4,298 MB。查询时没有待完成或账务未结清视频任务，切换前需重新统计。

新增官方迁移为 238_opencode_go_platform.sql 和 238_purge_unlimited_user_platform_quotas.sql。前者扩展平台约束；后者清理三档限额均为 NULL 的配额行，预检时 632 行。迁移按完整文件名识别，与定制 238 文件不重名。静态迁移测试不代替真实数据库升级演练。

9 月 16 日现有数据库备份目录校验通过，本轮尚未做生产快照恢复、迁移/回滚演练或应用切换。正式上线前应重新取得一致性备份，验证恢复，并按当前运行容器记录的完整 8 层 Compose 覆盖配置仅重建 sub2api。

## 当前流程位置

2026-09-21，项目负责人已审核冲突处理与发布范围，并明确批准继续下一步。按 CUSTOM_UPGRADE.md 完成 merge commit、推进 custom 和远端 CI；仅在 CI 通过后创建发布标签。生产切换仍须先完成最新备份、真实恢复及迁移验证。
