# Sub2API DeepSeek 快审延时优化测试计划

> 基线：`custom-v0.1.170-7`
>
> 候选版本：`custom-v0.1.170-latency-rc6`
>
> 本地隔离环境：Sub2API `127.0.0.1:18081`、PostgreSQL `15432`、Redis `16380`、Mock OpenAI `19090`

## 1. 测试目标

1. 证明普通请求只执行 fast，不提前构建或调用 full/max。
2. 证明 `fast_stage_budget_ms=3000` 能吸收 DeepSeek 正常尾部抖动，同时仍有明确超时上限。
3. 证明明确风险仍可由本地规则快速拦截，渐进风险仍能依赖会话状态升级。
4. 证明缩短输入和输出协议不会造成误放行、误拦截或解析异常。
5. 证明双 Key、缓存、代理、邮件、封禁和解禁逻辑没有回归。
6. 证明管理 API、UI、持久化配置和审核日志使用相同字段语义。

## 2. 固定测试配置

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

所有网关请求使用 `gpt-5.6-terra`，OpenAI 上游由本地 mock 账号承接；DeepSeek 使用隔离环境中已配置的两个审核 Key。

## 3. 单元与静态测试

| ID | 用例 | 通过条件 |
| --- | --- | --- |
| UT-01 | 旧配置缺少 `fast_stage_budget_ms` | 自动归一化为 3000，不需要迁移 |
| UT-02 | 快审预算小于 500 或大于 3000 | 自动回落默认值 |
| UT-03 | 快审预算大于总同步预算 | 被限制到总同步预算 |
| UT-04 | 父 context 无 deadline | fast context 仍获得独立 deadline |
| UT-05 | 父 deadline 短于快审预算 | 保留较短父 deadline |
| UT-06 | fast JSON 三个必填字段完整 | 正常解析 |
| UT-07 | fast 缺字段、未知字段、尾随 JSON、非法信号 | 一律解析失败 |
| UT-08 | full/max 缺少完整字段 | 一律解析失败 |
| UT-09 | 正常 fast 路径 | 不构建 full/periodic 输入 |
| UT-10 | periodic/风险升级/forced | 仅在需要时构建对应复核输入 |
| UT-11 | HTTP 客户端 | HTTP/2 尝试、连接池和代理不回退直连均通过 |
| UT-12 | 前端配置组件 | 四项性能配置独立更新、保存、回读和边界校验通过 |

## 4. Docker 网关测试

| ID | 场景 | 输入与步骤 | 预期结果 |
| --- | --- | --- | --- |
| GW-01 | 普通短请求 | 普通项目说明 | HTTP 200；stage=fast；不调用 full/max |
| GW-02 | 明确防御性请求 | API Key 保护建议 | HTTP 200；不因安全词误拦；无 timeout |
| GW-03 | 明确高风险 | 未授权认证绕过和令牌提取 | HTTP 403；优先由 local 快速拦截 |
| GW-04 | 相同请求精确缓存 | 同用户、Key、会话、状态重复请求 | 第二次命中结果缓存且不调用 provider |
| GW-05 | 缓存隔离 | 改变用户、Key、会话或风险状态 | 不错误复用另一身份的审核结论 |
| GW-06 | 20 轮正常对话 | 每轮追加普通项目进度 | 全部 200；不随历史长度增加 fast 本地准备耗时 |
| GW-07 | 8 轮渐进风险 | 前 6 轮防御铺垫，第 7～8 轮转为未授权绕过 | 前 6 轮放行；第 7～8 轮拦截 |
| GW-08 | 会话隔离 | 会话 A 风险、会话 B 正常 | B 不继承 A 风险 |
| GW-09 | 无会话 ID | 正常、风险、正常三次 | 风险请求拦截；两个正常请求放行且不串状态 |
| GW-10 | previous_response_id | 首次有效，后续引用 mock 不存在响应 | 首次 200；后续按协议返回 400，不产生 5xx |
| GW-11 | 图片占位输入 | 带图片和文本 | DeepSeek 只审核规范化文本描述，不因不支持多模态异常 |
| GW-12 | 同会话并发 | 同时发送两个普通请求 | 两个请求均完成；无死锁、重复状态覆盖或 5xx |
| GW-13 | 周期复核 | 到达第 25 个用户轮次 | 仅该轮触发 periodic/full，之前普通轮次保持 fast |
| GW-14 | 风险分升级复核 | fast 分数超过 0.55 或增幅超过 0.25 | 触发 full；日志记录明确升级原因 |
| GW-15 | 本地构建基准 | 1、10、40、100、512 轮 | fast-only P95 保持毫秒级；不随历史线性增长 |

## 5. DeepSeek 与故障测试

| ID | 场景 | 预期结果 |
| --- | --- | --- |
| DS-01 | 两个 Key 健康检查 | 两个不同 Key 均返回 ok |
| DS-02 | Key 轮转 | 两个 Key 均实际承接审核调用，状态和用量可归因 |
| DS-03 | 同 Key 内部重试 | fast 不发生同 Key hedge/fallback |
| DS-04 | 429/5xx/网络错误 | 仅允许外层切换另一个健康 Key；尝试次数有界 |
| DS-05 | provider 超过 3000ms | `failure_policy=block` 返回审核不可用且不进入 OpenAI mock |
| DS-06 | 父同步预算先耗尽 | 使用较短总 deadline，不突破 4800ms |
| DS-07 | 代理失败 | 失败返回错误，不允许绕过代理直连 |
| DS-08 | 非法 JSON | 记录审核异常，不当作低风险结论 |
| DS-09 | DeepSeek 前缀缓存 | 分 Key 记录 cached/uncached token，不能跨 Key 合并统计 |

## 6. 业务回归测试

| ID | 场景 | 通过条件 |
| --- | --- | --- |
| RG-01 | 用户过滤 | 被排除管理员和用户不产生无效审核 |
| RG-02 | 账号过滤 | 混合分组中仅目标 Pro 账号进入审核范围 |
| RG-03 | 故障切换 | 切换账号后重新按最终目标账号判断审核范围 |
| RG-04 | 非命中日志 | `record_non_hits=true` 时可见；关闭时不写冗余日志 |
| RG-05 | 邮件去重 | 同一风险会话在去重窗口内不连续发送多封邮件 |
| RG-06 | 自动封禁 | 达阈值时只封禁一次，计数规则不回归 |
| RG-07 | 手工解禁 | 解禁后清除对应短期封禁状态，不自动继承旧封禁结论 |
| RG-08 | UI | 能查看审核总耗时、阶段耗时和快审阶段预算；旧日志兼容 |

## 7. 通过门槛

- 所有安全正确性断言必须 100% 通过。
- 正常、防御性请求不得出现 `audit_timeout` 或错误拦截。
- 明确风险和渐进风险测试不得出现新增漏报。
- 正常 fast 请求不得调用 full/max。
- 本地准备 P95 小于 50 ms；当前目标保持约 1 ms。
- DeepSeek fast P95 生产灰度目标不高于 2200 ms，3,000 ms 超时异常率低于 0.5%。
- 网关不得出现 5xx、代理绕过、跨身份缓存污染或跨会话风险污染。
- Docker 测试完成后必须恢复账号、API Key、邮件、封禁和风控配置。

## 8. 证据产物

每轮测试保存：候选镜像 ID、配置回读 JSON、网关结果、审核日志摘要、阶段调用计数、延时分位、双 Key 用量、缓存统计、失败断言和配置恢复结果。只有这些证据全部满足通过门槛，才能创建 GitHub PR。
