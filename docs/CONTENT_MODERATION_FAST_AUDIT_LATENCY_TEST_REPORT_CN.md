# Sub2API DeepSeek 快审延时优化测试报告

> 测试日期：2026-08-07
>
> 基线：`custom-v0.1.170-7` (`dad5248e`)
>
> 最终候选：`sub2api-custom:test-latency-20260807-rc6`

## 1. 结论

最终候选通过本地隔离 Docker、Go 回归、前端测试、生产构建和浏览器 UI 烟测。正常请求的 Sub2API 本地快审准备约为 1 ms，实际可见延时主要来自 DeepSeek；精确低风险结果缓存命中时可完全跳过 DeepSeek，实测审核总耗时由 738 ms 降为 1 ms。

100 次正常请求的审核 P50 为 815 ms、P95 为 1,089 ms、最大 1,820 ms，未出现审核异常或网关 5xx。该结果满足“普通快审尽可能不被用户感知”的当前工程门槛，但未承诺所有首次出现、未命中缓存的语义请求都能达到零延时。

## 2. 固定配置

```json
{
  "ai_cache_enabled": true,
  "ai_fast_stage_budget_ms": 3000,
  "ai_synchronous_budget_ms": 4800,
  "ai_fast_input_chars": 3000,
  "ai_fallback_input_chars": 3000,
  "ai_summary_max_chars": 500,
  "ai_fast_max_output_tokens": 128,
  "ai_full_review_threshold": 0.55,
  "ai_full_review_risk_delta": 0.25,
  "ai_periodic_full_review_turns": 25,
  "ai_full_review_max_input_chars": 30000,
  "ai_full_max_output_tokens": 768,
  "ai_max_review_max_output_tokens": 1024
}
```

## 3. 性能结果

| 项目 | 结果 |
| --- | ---: |
| 100 请求 HTTP 200 | 100 / 100 |
| 审核异常 | 0 |
| P50 | 815 ms |
| P95 | 1,089 ms |
| 最大值 | 1,820 ms |
| fast 最终决策 | 96 |
| full 最终决策 | 4 |
| 完整复核比例 | 4% |
| fast 平均输入字符 | 1,126 |
| DeepSeek 前缀缓存命中率 | 78.2% |

本地构建基准：1～512 轮历史下 fast-only 准备约 0.43～0.95 ms；512 轮完整复核输入构建约 429 ms，但只有风险升级后才执行，不再阻塞普通 fast 路径。

## 4. 精确结果缓存

同一低风险目标连续请求两次：

| 次数 | 调用 DeepSeek | Sub2API 结果缓存 | 审核总耗时 |
| --- | --- | --- | ---: |
| 第一次 | 是 | 未命中 | 738 ms |
| 第二次 | 否 | 命中 | 1 ms |

缓存守卫已覆盖：空状态与显式 `low/stable` 等价、跨周期桶失效、风险等级/趋势/类别/信号变化失效、历史依赖请求不量化。

## 5. 安全与业务矩阵

- 20 轮正常长对话全部放行。
- 8 轮渐进风险中第 1～6 轮放行，第 7～8 轮拦截。
- 17 个防御/元讨论误报样例全部放行。
- 9 个明确高风险样例全部在进入 OpenAI mock 前拦截。
- 会话隔离、无会话 ID、`previous_response_id` 协议、图片占位、同会话并发均通过。
- 两个 DeepSeek Key 均健康并各承接 3 次完整复核；预热后每 Key 前缀缓存命中率约 95.0%。
- 用户/账号范围、代理失败不回退直连、邮件去重、自动封禁和手工解禁回归测试通过。

## 6. UI 与可观测性

- 设置页显示“快审阶段预算（毫秒）”，回读值为 3,000。
- 审核列表表头为“审核总耗时”，不再误称“上游耗时”。
- 详情显示请求提取、身份归一化、本地规则、结论缓存、风险上下文、快审构建、完整复核构建、DeepSeek 调用和结果处理耗时。
- 一个完整复核样例中：快审构建 2 ms、完整复核构建 20 ms、DeepSeek 调用 1,601 ms、结果处理 7 ms、审核总耗时 1,638 ms。
- 旧日志缺少新诊断字段时继续按未知值兼容展示。

## 7. 已知限制与灰度门槛

三个明确高风险样例的完整复核耗尽 4,800 ms 总预算后按严格模式拦截，日志记录为 `action=error`。这不影响普通 fast 性能，也没有导致风险请求进入 OpenAI mock，但生产指标必须把“fast provider 异常”和“风险升级复核不完整”分开观察。

生产先只灰度到指定 Pro 账号。若正常 fast P95 连续 15 分钟超过 2,200 ms、3,000 ms 超时率超过 0.5%、出现新增高风险漏报、代理绕过或跨身份缓存污染，应立即停止扩大灰度并回滚候选镜像。

## 8. 验证命令

已通过：

```text
go test ./internal/custom/voteai/auditcontext ./internal/custom/voteai/moderation ./internal/pkg/httpclient ./internal/handler/admin ./internal/service -run "ContentModeration|Moderation|Audit" -count=1
pnpm exec vitest run src/custom/vote-ai/risk-control/__tests__/PromptPerformanceAndTestOutcome.spec.ts src/views/admin/__tests__/RiskControlView.spec.ts
pnpm run typecheck
pnpm run build
git diff --check
```

Docker 证据保存在本地隔离测试产物目录，不提交 API Key、数据库、Redis 数据、原始日志或镜像层到 Git。
