# Sub2API DeepSeek 快审延迟优化设计

> 适用基线：Vote AI `custom-v0.1.170-7`
>
> 设计日期：2026-08-07
>
> 状态：实现与本地验证中

## 1. 目标

在不把未经审核的用户内容提前发送给 OpenAI 上游、不削弱本地明确风险检测、不破坏长会话风险状态的前提下，尽可能降低 DeepSeek 快审对用户首字时间的影响。

正常快审目标：

| 指标 | 目标 |
| --- | --- |
| 快审同步总耗时 P50 | 不高于 500 ms |
| 快审同步总耗时 P95 | 不高于 900 ms |
| 快审同步总耗时 P99 | 不高于 1,200 ms |
| 本地准备耗时 P95 | 不高于 50 ms |
| 40 轮会话本地准备耗时 P99 | 不高于 150 ms |
| 快审异常率 | 低于 0.5% |
| 高风险测试集召回率 | 不低于改造前基线 |
| 防御性请求误报率 | 不高于改造前基线 |

完整复核、极高风险复核不使用上述快审延迟目标。风险升级请求允许使用更长预算，以保护上游账号。

## 2. 不可破坏的安全边界

1. 审核通过前不得向 OpenAI 账号发送用户请求。
2. 不采用“先向 OpenAI 投机转发、审核失败后取消”的方案。
3. 本地确定性检测器继续扫描完整、规范化后的当前审核目标。
4. 快审采样不得影响本地明确风险词、组合规则和候选规则判断。
5. `auth_bypass`、`secret_extraction`、`malware_delivery`、`policy_evasion`、`progressive_escalation` 等强信号仍可触发完整或极高风险复核。
6. 账号过滤、用户过滤、模型过滤和管理员内部探针绕过逻辑保持不变。
7. 审核代理配置失败时不得绕过代理直连。
8. 不允许不同用户、API Key 或会话之间复用依赖上下文的审核结论。

因此，严格同步语义审核不可能真正增加 0 ms。工程目标是把正常快审压缩到网络抖动级别，使其相对 GPT 首字时间不明显，同时拒绝以号池安全换取表面延迟。

## 3. 生产现状与证据

2026-08-07 生产只读检查观察到：

- 短会话审核总耗时约 600 至 800 ms；
- 长会话连续请求审核总耗时约 3,764 至 4,351 ms；
- Worker 排队仅 3 至 5 ms，不是主要瓶颈；
- 一个代表性快审阶段的 DeepSeek 调用耗时为 1,075 ms；
- 同一记录 DeepSeek 前缀缓存命中率为 94.5%；
- 长会话越长，列表中的审核总耗时越高。

现有列表字段 `upstream_latency_ms` 实际记录从审核核心开始到获得模型结果的总时间，而详情中的 `stages[].latency_ms` 只记录单个 DeepSeek 阶段。列表“上游耗时”名称会误导运维判断。

现有增量快审准备路径还存在以下问题：

1. 遍历全部历史轮次并对所有历史文本执行脱敏；
2. 快审前同时构建完整复核输入；
3. 快审前同时构建周期复核输入；
4. 提取、身份归一化和审核准备之间重复规范化文本；
5. 多次对相同字符串执行 `[]rune` 转换、裁剪和拼接；
6. 即使最终没有升级，仍支付完整复核的数据准备成本；
7. 单一 `synchronous_budget_ms` 同时约束快审和风险升级，不利于控制正常请求长尾；
8. 外层 API Key 重试与适配器内层 fallback/hedge 叠加，尾部延迟难以解释。

## 4. 总体方案

```text
请求进入
  -> 范围过滤
  -> 并行启动会话状态读取
  -> 单次结构化提取与身份归一化
  -> 完整当前目标本地确定性检测
  -> 精确请求结论缓存
  -> 仅构建快审所需的最小上下文
  -> DeepSeek 快审
  -> 不升级：立即放行并异步记录
  -> 需要升级：按原因惰性构建 full 或 periodic 输入
  -> DeepSeek 完整/极高风险复核
  -> 更新风险状态与记录
```

改造分为四个可独立审阅和回滚的阶段：

1. 指标口径与基线；
2. 本地热路径与惰性构建；
3. 快审协议、缓存和连接；
4. 分阶段预算、Key 调度和生产灰度。

## 5. PR1：延迟指标与口径修复

### 5.1 新增诊断字段

在 `ContentModerationAuditDetails` 增加可选字段：

```go
TotalLatencyMS          *int `json:"total_latency_ms,omitempty"`
ExtractionLatencyMS     *int `json:"extraction_latency_ms,omitempty"`
ProvenanceLatencyMS     *int `json:"provenance_latency_ms,omitempty"`
DeterministicLatencyMS  *int `json:"deterministic_latency_ms,omitempty"`
VerdictCacheLatencyMS   *int `json:"verdict_cache_latency_ms,omitempty"`
ContextLoadLatencyMS    *int `json:"context_load_latency_ms,omitempty"`
FastBuildLatencyMS      *int `json:"fast_build_latency_ms,omitempty"`
ReviewBuildLatencyMS    *int `json:"review_build_latency_ms,omitempty"`
ProviderLatencyMS       *int `json:"provider_latency_ms,omitempty"`
PostprocessLatencyMS    *int `json:"postprocess_latency_ms,omitempty"`
```

所有字段只记录时间，不记录原始内容、API Key、密码或完整会话。

### 5.2 计时边界

- `TotalLatencyMS`：进入 `Check` 到形成同步决定；
- `ExtractionLatencyMS`：JSON 结构验证、协议提取和初次规范化；
- `ProvenanceLatencyMS`：客户端身份判断和审核目标选择；
- `DeterministicLatencyMS`：完整当前目标的本地规则扫描；
- `VerdictCacheLatencyMS`：请求结论缓存和 suppression fence；
- `ContextLoadLatencyMS`：Redis 审核上下文读取；
- `FastBuildLatencyMS`：最小快审输入构建；
- `ReviewBuildLatencyMS`：升级后 full/periodic 输入构建；
- `ProviderLatencyMS`：DeepSeek 请求发出到完整 JSON 返回；
- `PostprocessLatencyMS`：风险状态计算和同步决定生成。

HTTP 细分使用 `httptrace` 采样记录 DNS、连接、TLS、等待响应头和读取响应体时间。默认只采样慢请求和少量正常请求，避免高并发下增加明显开销。

### 5.3 UI 调整

- 列表“上游耗时”改名为“审核总耗时”；
- 保留“排队耗时”；
- 详情增加“延迟分解”；
- 逐阶段诊断继续显示 DeepSeek 阶段耗时；
- 慢请求明确显示瓶颈阶段；
- 兼容没有新字段的旧日志。

### 5.4 运行指标

按以下维度维护 P50/P95/P99：

- fast/full/max；
- 结果缓存命中/未命中；
- 会话轮次区间：1、2-10、11-40、41-100、100+；
- 请求字符区间；
- DeepSeek API Key 哈希；
- 冷连接/复用连接；
- 成功/超时/临时错误。

## 6. PR2：快审本地热路径优化

### 6.1 单次提取和规范化

现有提取器已经返回结构化 `Turns`，后续不得重新把全部轮次渲染成大字符串再解析。

改造要求：

1. `ExtractContentModerationInputOutcome` 负责唯一一次协议提取；
2. 身份归一化直接处理结构化轮次；
3. 增量模式不再为快审重建完整 `content.Text`；
4. 删除无语义变化的重复 `Normalize()`；
5. 当前审核目标生成规范化文本和脱敏文本后复用；
6. 缓存每段文本的字符数和是否截断，避免重复 `[]rune`。

### 6.2 最小快审视图

引入只服务于快审的结构：

```go
type FastAuditView struct {
    TargetKind       string
    TargetText       string
    SupportingTurns []AuditTurn
    RiskDigest       string
    Truncated        bool
}
```

构建规则：

- 从目标位置向前扫描，不从第一轮开始遍历；
- 默认保留最近 2 个用户轮次及其直接关联的助手/工具结果；
- 系统和开发者元数据继续按 provenance 规则处理；
- 当前目标过长时使用确定性的头部/中部/尾部采样；
- Redis 风险摘要固定长度并保持稳定前缀；
- 不复制未进入快审窗口的历史文本。

### 6.3 惰性完整复核

`contentModerationIncrementalPlan` 不再提前持有完整字符串，而是保存足够的结构化引用：

```go
type contentModerationIncrementalPlan struct {
    turns       []auditcontext.Turn
    targetIndex int
    targetKind  string
    targetText  string
    fastInput   auditcontext.FastInput
    // 其余状态字段保持不变
}
```

只有 `DecideFullReview` 返回需要升级后才执行：

- 纯 `periodic`：构建最近 10 个用户轮次的紧凑轨迹；
- 其他风险原因：构建受 `full_review_max_input_chars` 限制的完整复核输入；
- `max`：沿用风险驱动完整输入并启用最大推理；
- 未升级：不得调用任何 full/periodic 构建函数。

兼容现有前缀连续性时必须满足：

- `forceFullReview` 在发起 DeepSeek 调用前立即构建 full 输入；
- 普通 fast 路径只保留结构化 `turns`、`targetIndex`、目标文本和状态，不提前渲染大字符串；
- `DecideFullReview` 命中 `periodic` 后构建 periodic 输入，其他原因构建 full 输入；
- 只有 full/max 确实调用 provider 后才更新 `CanonicalPrefixHash`、`LastPrefixChars`、`AuditKeyHash` 和连续性观测；
- 惰性构建必须复用现有 compaction、history rewrite、history truncated 和 previous-prefix 判定，不能另写一套前缀算法；
- 通过现有 `content_moderation_stage_prefix_test.go` 和增量守卫测试证明同一输入的 canonical prefix 与改造前一致。

### 6.4 会话状态读取

会话 ID 在进入审核前已经可用。状态读取可以与结构化提取并行，但需要遵守：

- 使用当前请求 context，可取消且有独立短超时；
- 不允许 goroutine 泄漏；
- Redis 读取失败沿用现有低权重/空状态语义并记录指标；
- 不引入跨实例不一致的可写 L1 会话状态；
- 状态更新仍以 Redis 原子逻辑为准。

第一版可以先完成惰性构建，再根据指标决定是否并行读取，避免一次 PR 引入过多并发复杂度。

## 7. PR3：专用快审协议和缓存

### 7.1 阶段专用提示词

快审不需要输出面向管理员的长理由。新增短协议：

```json
{"flagged":false,"risk_score":0.1,"signals":["defensive_context"]}
```

规则：

- `flagged`、`risk_score`、`signals` 必须存在；
- `categories` 和 `reason` 在 fast 阶段可省略；
- full/max 阶段继续返回完整结构和中文理由；
- fast 解析失败不得被当作安全结论；
- 系统提示词使用稳定公共前缀，阶段指令保持短小稳定；
- 用户输入始终放在独立 user message，不能覆盖系统规则。

不能直接复用当前宽松的 `ParseResult` 做字段存在性判断。Go 的零值无法区分 `flagged:false` 与字段缺失，也无法区分 `risk_score:0` 与字段缺失。实现时增加 stage-aware 严格解析入口：

- 先解析为字段使用指针的内部 wire struct，明确验证 fast 的三个必填字段均存在；
- 继续启用 `DisallowUnknownFields`、单一 JSON 对象、风险分范围和枚举白名单校验；
- fast 允许省略 `categories/reason`，解析后规范化为空数组和空字符串；
- full/max 要求 `flagged`、`risk_score`、`categories`、`signals`、`reason` 全部存在；
- 旧配置未启用短协议时继续走旧解析入口，避免一次发布改变历史 provider 兼容性；
- 缺字段、未知字段、尾随内容或非法枚举一律视为审核异常，不能按低风险放行。

初始参数建议：

| 参数 | 当前 | 灰度初值 |
| --- | ---: | ---: |
| fast_input_chars | 6000 | 3000 |
| recent_user_turns | 2 | 2 |
| summary_max_chars | 800 | 500 |
| fast_max_output_tokens | 256 | 128 |
| thinking | fast 已关闭 | 保持关闭 |

最大 Token 只用于限制异常长输出。正常 JSON 应在约 20 至 60 Token 内结束。

### 7.2 完整目标安全补偿

缩短 DeepSeek 快审输入不能缩短本地扫描范围：

- 本地确定性检测继续扫描完整当前目标；
- 工具输出继续使用全量本地规则后再采样外发；
- 明确规则候选仍强制进入完整复核；
- 输入截断且存在非防御风险信号时仍升级；
- 长对话通过风险状态和周期复核维持上下文安全。

### 7.3 精确结果缓存

增加：

1. 进程内短 TTL L1；
2. Redis L2；
3. 同键 `singleflight` 合并并发请求。

缓存键必须包含：

- Base URL 和模型；
- 策略版本；
- 审核阶段；
- 规范化审核目标哈希；
- 必需的直接支持上下文哈希；
- 风险状态摘要；
- 提示词版本；
- 输出协议版本。

以下请求禁止跨会话复用：

- “继续”“展开”“写成脚本”等依赖历史的目标；
- 包含 `progressive_escalation` 的状态；
- 无法确认稳定会话且需要历史解释的请求；
- 管理员已 suppress 对应哈希后的旧结论。

缓存命中必须跳过 DeepSeek 网络调用，并保留“结果缓存命中”和“DeepSeek 前缀缓存命中”两个独立指标。

## 8. PR4：连接、Key 调度和分阶段预算

### 8.1 HTTP 客户端

现有客户端池已经复用连接，继续保留代理不回退直连的边界，并补充：

- 对 DeepSeek 客户端启用 `ForceAttemptHTTP2`；
- 提高 `MaxIdleConnsPerHost`，初值 32；
- 根据并发上限设置 `MaxConnsPerHost`；
- 配置更新后关闭旧空闲连接；
- 服务启动后执行不含用户内容的连接预热；
- 记录冷连接和复用连接命中率。

不得假设 HTTP/2 一定更快，必须通过生产相同代理链路 A/B 验证后启用。

### 8.2 API Key 调度

现状不是一个干净的单层重试：适配器内 fast 会在同一 Key 上启动 fallback/hedge，服务层 `callModeration` 又会按 `retry_count` 串行切换 Key。第一步必须先统一为由服务层掌握跨 Key 调度、适配器只执行一次有界请求，不能在现有两层机制上再叠加第三套 hedge。

- 使用每个 Key 的成功率和延迟 EWMA 选择主 Key；
- 429、5xx、网络超时进入现有健康退避；
- 正常同步快审不做串行重试；
- 主请求超过可配置 hedge 延迟后，可向第二个不同健康 Key 发起竞争请求；
- 任一成功后取消另一个；
- hedge 设全局并发和比例上限；
- 记录竞争请求的额外成本，不能把未知消耗统计为 0；
- full/max 默认不 hedge，避免高成本重复调用。

迁移规则：

- 默认关闭同步 fast 的同 Key fallback；JSON mode 或请求格式导致的 400 不得通过同 Key、不同协议静默重试；
- `retry_count` 对 fast 的新语义为“可用的不同 Key 数量上限”，正常成功路径只调用一次；
- 只有网络错误、429 和可重试 5xx 才允许切换健康 Key；
- 可选 hedge 必须由服务层选择第二个不同 Key，并受独立开关、比例、并发和延迟阈值约束；
- 灰度前同时记录 provider attempt 数、distinct key 数、hedge winner/loser 和已知 token 消耗；
- legacy 非增量审核继续保持原语义，直到有独立回归测试后再迁移。

初始 hedge 延迟不写死，先取生产 fast provider P95 的 60% 至 70%，下限 400 ms，上限 700 ms。

### 8.3 分阶段预算

新增配置：

```json
{
  "fast_stage_budget_ms": 3000,
  "full_review_budget_ms": 4500,
  "max_review_budget_ms": 5000,
  "fast_hedge_delay_ms": 600
}
```

兼容规则：

- 旧配置只有 `synchronous_budget_ms` 时继续按旧逻辑工作；
- 开启新预算后，正常 fast 不得占用 full/max 的预算；
- fast 升级后重新建立受请求总 deadline 限制的 review context；
- 客户端已断开时立即取消所有审核调用；
- 预算耗尽必须产生明确诊断。

`request verdict` 的 singleflight follower 当前按 `synchronous_budget_ms + grace` 等待。启用分阶段预算时必须同步调整 claim TTL、leader 工作超时和 follower 等待时间：

- 仅 fast 的请求按 fast 预算等待；
- 可能升级的 leader 以“请求总预算”为硬上限，不允许 follower 先于合法 full/max 决策超时；
- follower 的 fallback 决策继续复用现有失败策略，不得自行放行；
- Redis claim TTL 必须覆盖总预算和现有 lease grace，防止 leader 尚未结束就被第二实例重复执行；
- 客户端 context 取消时 leader 和 follower 都应尽快退出，后台补审走现有独立队列语义。

### 8.4 超时策略

“低延迟、绝不漏过未知语义风险、第三方审核故障时仍然放行”三者无法同时满足。

不新增第三套失败策略枚举，直接沿用现有配置：

- `failure_policy=block`：快审预算耗尽后返回审核服务不可用，不向 OpenAI 转发；
- `failure_policy=allow`：本地未发现明确风险时放行并进入补审，但存在语义漏检窗口。

生产保护 Pro 账号推荐 `block`。双 Key、连接复用和有限 hedge 用于降低 fail-closed 的不可用率，而不是偷偷改成 `allow`。

## 9. 完整复核频率和成本参数

延迟改造同时建议灰度：

| 参数 | 当前 | 建议 |
| --- | ---: | ---: |
| full_review_threshold | 0.40 | 0.55 |
| full_review_risk_delta | 0.15 | 0.25 |
| periodic_full_review_turns | 10 | 25 |
| full_review_max_input_chars | 60000 | 30000 |
| full_max_output_tokens | 1024 | 768 |
| max_review_max_output_tokens | 1536 | 1024 |

上述参数只减少普通分数、风险波动和周期原因触发的完整复核。`forced`、`strong_signal`、`cumulative_risk`、`progressive_language` 和 `truncated_risk` 不得被这些参数绕过。

## 10. 测试矩阵

### 10.1 单元测试

1. 正常快审不构建 full/periodic 输入；
2. `periodic` 只构建周期轨迹；
3. 风险升级只构建 full 输入；
4. 本地候选规则仍强制完整复核；
5. fast 短协议缺字段、非法 JSON、超范围分数均失败；
6. fast 省略 `reason/categories` 可正常解析；
7. 完整阶段仍要求完整字段；
8. 重复 Normalize 被消除但提取结果不变；
9. 头中尾采样覆盖目标尾部风险；
10. 支持上下文请求不跨会话缓存；
11. singleflight 合并并发相同审核；
12. suppression fence 使旧缓存失效；
13. `failure_policy=block` 超时不调用 OpenAI 上游；
14. `failure_policy=allow` 超时只产生一次补审；
15. hedge 只能使用不同健康 Key，成功后取消 loser；
16. 代理失败不直连。

### 10.2 基准测试

对 1、2、10、40、100、512 轮会话分别测试：

- 5 KB、50 KB、200 KB、1 MB 请求体；
- 本地提取；
- provenance 归一化；
- 快审视图构建；
- full/periodic 惰性构建；
- 内存分配次数和字节数。

必须与 `custom-v0.1.170-7` 基线做同机对比，不能只报告优化后绝对值。

### 10.3 集成测试

- DeepSeek 200、400、429、500、超时、空内容、非法 JSON；
- 单 Key 和双 Key；
- 直连和生产同类型代理；
- HTTP 冷连接和热连接；
- Redis 正常、慢响应和不可用；
- 缓存命中、未命中、并发击穿；
- pre_block/observe；
- 正常、防御性、隐晦风险、渐进升级和明确高风险。

### 10.4 本地 Docker 端到端

1. 使用管理员 API Key 通过本地 Sub2API 发起真实请求；
2. 使用记录和风控记录必须同时可见；
3. 正常快审、完整复核、周期复核、缓存命中和 `failure_policy=block` 超时均有记录；
4. 验证混合分组只对目标 Pro 账号审核；
5. 验证故障切换不会绕过账号审核范围；
6. 验证长会话第 2、10、25、40、65 轮行为；
7. 验证邮件、封禁、解禁和去重不回归；
8. 统计用户感知首字、审核总耗时和 DeepSeek 阶段耗时。

## 11. 灰度与回滚

### 11.1 灰度顺序

1. 只上线指标，不改行为；
2. 上线惰性构建，保持原有 DeepSeek 参数；
3. 对一个测试用户启用 fast 短协议；
4. 对一个 Pro 账号启用新预算；
5. 扩大到 10%、30%、100% 目标账号；
6. 最后调整完整复核频率参数。

每阶段至少观察：

- fast 总耗时 P50/P95/P99；
- provider 耗时 P50/P95/P99；
- 审核异常率；
- `failure_policy=block` 审核不可用拒绝率；
- full/max 比例；
- 误报和漏报测试集；
- DeepSeek 输入、输出和 hedge 额外成本；
- OpenAI 上游账号异常率。

### 11.2 自动回滚条件

任一条件满足即停止扩大灰度：

- 高风险测试集出现新增漏报；
- 防御性误报率显著上升；
- 快审异常率超过 0.5%；
- `failure_policy=block` 不可用率超过约定阈值；
- fast P95 连续 15 分钟高于 1,200 ms；
- hedge 调用比例或成本超过配置上限；
- Redis、Worker、邮件、封禁或解禁出现回归。

所有新行为必须有独立开关。回滚配置不得要求数据库回滚；镜像回滚不得破坏旧版本读取现有配置。

## 12. 实施顺序与提交边界

建议提交顺序：

1. `docs(risk-control): design fast audit latency optimization`
2. `feat(risk-control): add audit latency breakdown`
3. `perf(risk-control): lazily build review context`
4. `perf(risk-control): add compact fast audit protocol`
5. `perf(risk-control): add exact fast verdict coalescing`
6. `perf(risk-control): add staged budgets and key hedging`
7. `test(risk-control): add fast audit latency matrix`
8. `docs(custom): document fast audit latency changes`

每个功能提交都必须可单独审阅；不得把生产配置修改、部署产物、数据库备份或本地日志提交到仓库。

## 13. 方案审阅结论门槛

进入代码实现前必须确认：

- 安全边界与现有账号范围过滤兼容；
- 惰性构建不会丢失 full/max 所需历史；
- fast 短协议不会被旧解析器误判；
- 新字段对旧配置、旧日志和旧前端兼容；
- `failure_policy=block/allow` 行为明确且有测试；
- hedge 不会复用同一个 Key，也不会无限增加调用；
- 所有指标不包含用户隐私和凭证；
- 本地测试、GitHub CI、生产灰度和回滚证据路径完整。

只有上述门槛全部通过，才进入代码修改。

## 14. 2026-08-07 本地实现与实测结论

### 14.1 已验证的延时构成

惰性构建完成后，正常 fast-only 路径的本地准备耗时已经稳定在约 1 ms：

| 历史轮数 | fast-only 本地准备 | full-review 本地准备 |
| ---: | ---: | ---: |
| 1 | 0.43 ms | 0.90 ms |
| 10 | 0.90 ms | 5.28 ms |
| 40 | 0.90 ms | 18.88 ms |
| 100 | 0.95 ms | 45.27 ms |
| 512 | 0.91 ms | 428.98 ms |

Docker 网关端到端测试中，两次正常快审的 DeepSeek provider 耗时分别约为 1,235 ms 和 1,428 ms；本地提取、上下文读取和 fast 输入构建均约为 0～1 ms。因此正常快审的主要瓶颈已经明确为 DeepSeek 网络与模型响应，而不是 Sub2API 本地处理。

### 14.2 快审预算修正

早期实现使用固定 1,500 ms 快审超时。第二轮端到端测试中，一个正常防御性请求因 provider 偶发超过 1,500 ms，在 `failure_policy=block` 下被错误返回为审核不可用。这一数值不能直接进入生产。

最终改为独立配置：

```json
{
  "fast_stage_budget_ms": 3000,
  "synchronous_budget_ms": 4800
}
```

- `fast_stage_budget_ms` 默认 3,000 ms，允许范围 500～3,000 ms，且不得超过总同步预算；
- 它只限制第一次 DeepSeek fast 调用；
- `synchronous_budget_ms` 继续限制整条前置审核链路，包括可能发生的 full/max 复核；
- 旧配置缺少新字段时自动使用 3,000 ms，不需要数据库迁移；
- 2,200 ms 在本地 20 轮正常对话矩阵中仍产生 1 次 fail-closed 超时，因此默认值提高到 3,000 ms；正常响应会立即返回，提高上限不会增加 P50/P95，只用于容纳尾部抖动；
- 生产灰度后应依据真实 provider P99 和异常率调整，而不是为追求表面低延时把预算压到正常抖动范围以内。

### 14.3 用户无感的可实现边界

严格要求“审核通过前不得进入 OpenAI 号池”时，DeepSeek 未命中的请求不可能真正增加 0 ms。可实现的分层目标是：

1. 明确本地规则或 Sub2API 精确结果缓存命中：不调用 DeepSeek，新增延时应接近本地处理级别；
2. 正常 DeepSeek 快审未命中缓存：控制在 provider 正常响应分位内，当前实测约 1.2～1.5 秒；
3. 风险升级请求：允许使用更长总预算，以保护 Pro 账号，不用普通请求的延时目标约束；
4. 禁止用“先向 OpenAI 转发、DeepSeek 稍后补审”换取表面零延时。

### 14.4 低风险稳定状态缓存量化

最终候选版已经实现受约束的低风险稳定摘要量化。仅当当前分和历史最高分都低于历史风险阈值、风险等级为空或 `low`、趋势为空或 `stable`、没有类别/信号/原因且请求不依赖历史时，快审摘要固定为：

```text
[SESSION-RISK-SUMMARY]
state=low_stable
turn_bucket=N
```

周期桶按 `turn_count / periodic_full_review_turns` 计算。空状态与持久化后的显式 `low/stable` 被归一化为相同风险摘要，避免第一次和第二次请求因等价状态的字符串差异产生不同结果缓存键。

以下任一条件会立即禁止量化或使缓存键变化：风险分达到历史阈值、`observe/high` 等级、`rising` 趋势、类别或风险信号存在、请求依赖历史、跨周期桶、策略版本变化。对应守卫测试证明风险状态、趋势变化、类别、信号、历史依赖和周期边界不会错误复用低风险结论。

Docker 实测中，同一低风险审核目标第一次调用 DeepSeek，审核总耗时 738 ms；第二次 `provider_called=false`、`sub2api_result_cache_hit=true`，审核总耗时 1 ms。

### 14.5 最终候选实测

最终候选镜像为 `sub2api-custom:test-latency-20260807-rc6`。100 次正常请求实测：P50 815 ms、P95 1,089 ms、最大 1,820 ms，无 4xx/5xx 和审核异常，完整复核比例 4%。20 轮正常长对话全部放行，快审端到端耗时主要位于 613～876 ms；渐进风险第 7～8 轮均被拦截，会话隔离、无会话 ID、图片占位和同会话并发均通过。

双 Key 完整复核测试中，两个 Key 各承接 3 次调用，DeepSeek 前缀缓存命中率在预热后稳定为约 95.0%，用量按 Key 独立归因。浏览器实测确认列表使用“审核总耗时”，设置界面显示独立快审预算，详情能拆分本地准备、DeepSeek 和后处理耗时。

三个明确高风险样例在完整复核阶段耗尽 4,800 ms 总预算后按 `failure_policy=block` 拦截，安全断言通过，但日志仍记录为 `action=error`。该现象只发生在风险升级路径，不计入普通 fast 延时目标；生产监控必须分别统计 fast provider 异常与高风险复核不完整，避免把两者混成一个“审核异常率”。
