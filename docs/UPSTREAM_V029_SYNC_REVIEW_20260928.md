# sub2api 同步官方 0.2.9：冲突审核与验证

日期：2026-09-28。状态：本地合并候选已准备，等待机主审核冲突处理。未提交合并、未推进 custom、未部署服务器。

## 固定版本和备份

- 原 custom：`c57d25c1d396eb33d86390e9f64e3dd58e1188bf`。
- 官方目标：`9a62841fd124d026cf3694fcf9b79e98addcdbdc`，VERSION 为 0.2.9。
- 官方 v0.2.9 标签指向的前一提交仍写 VERSION 0.2.8，因此纳入官方紧随其后的版本号修正。
- GitHub 已保存备份分支 `backup/custom-before-v0.2.9-20260928`，指向原 custom。
- 本地候选分支 `sync/upstream-v0.2.9-20260928`，目录 `work/sub2api-github`。
- 使用 merge --no-ff --no-commit，保留完整历史；工作区处于已解决冲突、待确认的 merge 状态，勿 reset 或切换清理。

## 审核重点

| 冲突位置 | 处理结果 |
|---|---|
| .gitignore | 保留定制文档跟踪例外，同时纳入官方其他忽略规则 |
| backend/cmd/server/wire_gen.go | 根据合并后的 Provider 重新生成 Wire；保留 STATE Kit 代理资源目录、TLS 路由、Seedance 服务，接入官方 OpenCode Go 用量服务及 Claude Code 版本同步/关闭流程 |
| backend/internal/service/wire.go | 同时保留定制 internalprobe 与官方 claude 依赖；不丢失任一 Provider |
| backend/internal/pkg/apicompat/types.go | 合并官方现已支持的缓存字段与定制 PromptCacheKey，去除自动合并造成的重复 PromptCacheOptions，保持序列化字段齐全 |
| backend/internal/pkg/apicompat/chatcompletions_to_responses.go | 保留缓存字段的独立拷贝与 PromptCacheKey 传递；加入官方 message 类型标识；保留原 system/developer 角色；纳入空文本缓存断点支持及官方其他转换修复 |
| backend/internal/service/content_moderation_input.go | 保留定制多轮风控、工具来源与元数据边界。官方“尾随 system 消息仍提取最新 user”的修复移入现有 content_moderation_current_user.go 单轮提取器；配套官方回归用例改为测试该实际路径 |
| frontend/src/utils/ccswitchImport.ts | 保留定制客户端目录、别名、模型覆盖及 UTF-8 编码；接入官方动态余额查询脚本和 Codex 根端点保留行为 |
| frontend/src/utils/__tests__/ccswitchImport.spec.ts | 保留定制客户端覆盖用例，纳入官方余额查询用例，并按新的 Codex 根端点行为更新原断言 |
| frontend/src/views/admin/__tests__/GroupsView.codexManifest.spec.ts | 使用官方当前 getModelAllowlistCandidates 测试桩，清除旧名称造成的重复/失效 mock |
| frontend/src/views/user/KeysView.vue | 保留统一一键配置和 RikkaHub 入口，避免重新引入旧的重复 CC Switch 处理函数；官方余额查询逻辑在现有 CodexOneClickModal.vue 接入 |

### 需要机主了解的可见变化

CC Switch 的 Codex 导入不再无条件追加 `/v1`：配置为根地址就保留根地址，原本配置了 `/v1` 则继续保留。余额查询脚本会根据 CC Switch 内保存的地址动态生成且只保留一个 `/v1/usage`。

这是接入官方修复后的预期行为，不影响 RikkaHub 的独立设置：RikkaHub 仍使用带 `/v1` 的 Base URL，Responses 开关开启、路径 `/responses`。

代码层保留品牌/文档、STATE Kit、Seedance、风控上下文改进、提示缓存和 RikkaHub；没有以官方界面或二进制覆盖定制功能。

## 已完成验证

- 前端两批定向测试按文件最后一次结果去重：26 个测试文件、353 项通过。覆盖 CC Switch、一键配置、密钥页、RikkaHub、插件管理、设置、账号编辑、模型白名单、模型广场定价、品牌首页、风控组件和语言完整性。
- 后端定向 Go 测试：11 个包通过，5,806 项测试及子测试通过；2 项按原条件跳过。
- 跳过项：TestContentModerationTypeSafeLive（需真实外部服务）、TestPluginRuntimeIntegration（需显式外部插件包）。未伪造外部服务验收。
- 另行运行 STATE Kit 真实插件进程契约测试：13 项通过，使用临时签名包和模拟上游，不使用生产凭证或代理。
- Vue/TypeScript 类型检查、冲突及适配文件 ESLint、语言完整性、Vite 正式构建、Wire 重新生成、git diff --check：通过。
- Linux amd64 嵌入前端后端构建：通过，生成候选二进制但没有运行或部署。

不运行本地全项目全量测试；GitHub CI 留待机主审核后提交候选，再核对准确 SHA 的结果。实际安卓联网调用、数据库迁移恢复和服务器部署不包含在本轮本地验证里。

## 数据库和发布边界

新增两份 SQL 迁移：

1. 239_channel_reasoning_effort_multipliers.sql：增加思考力度倍率字段，并转换原 max 定价配置和分组 JSON；旧 JSON 字段会被移除，部署回滚需要专门验证数据兼容。
2. 240_affiliate_ledger_operation_id.sql：联盟账本新增幂等标识和唯一索引。

本轮没有执行数据库迁移；现有定制迁移文件未被替换。这两份代码已参与定向迁移检查，但不代表真实生产数据库已备份、恢复或通过升级演练。

未来部署还需核对 custom 相对 OVH 生产 0.2.7 的全部累积变更，特别是 Seedance 旧视频收尾，不得顺手运行破坏性 finalizer。

## 审核后步骤

仓库 CUSTOM_UPGRADE.md 要求：“出现冲突时，处理后必须停下，由项目负责人最终审核冲突文件。”

机主认可以上处理后，完成 merge commit、推送同步分支并创建指向 custom 的 PR；待该提交远端 CI 通过后合并。临时同步/备份分支在确认收录和归档后清理。此确认不包含生产部署。
