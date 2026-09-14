# 视频功能发布与回退

## 本次交付

业务实现、235—239 迁移、接口说明、人工验收说明和探测工具随 Git 交付。按要求移除本次新增测试代码；原有测试保留必要的接口适配。`.pnpm-store/`、`accepted-video*.mp4`、`tools/1.png`、`outputs/` 不进入 Git 或 Docker 构建上下文，验收证据另存受控附件，不能包含密钥或签名下载地址。

2026-09-11 重新验证：Docker Desktop 已恢复，本地验证镜像 `sub2api:video-retest-20260911` 构建和健康启动通过；独立 PostgreSQL 16 空库的全部 288 份迁移及校验和核对通过，重启和备份恢复至新库通过，模拟 uncertain 冻结订单及余额/账本完整保留。另有 7 项 PostgreSQL 18 迁移集成检查通过。前端构建、35 项相关测试、代理连接池和探测工具验证通过。此前 4 个插件安装测试的 Windows 文件占用问题已修复：归档解压后、提交安装路径前关闭 ZIP 句柄；原有测试仅在支持的平台检查 Unix 执行权限。修复后 Windows service 包完整默认回归通过（含子测试 6866 项通过、2 项按原条件跳过），Linux 后端构建及 9 项插件包测试通过。临时验证代码已删除，隔离容器已停止。此前的 video-retest 镜像未包含这次插件修复，正式提交后须重新构建并验证最终镜像。本地镜像未推送，以下生产快照升级、真实入口和实际上一版本回退步骤仍待执行，不代表已完成生产核查。

## 镜像与回退版本

默认、local、standalone Compose 均要求 `SUB2API_IMAGE`；独立验收 Compose 使用同一镜像。禁止用上游 `latest` 代替本次构建，也不要覆盖已发布的版本标签。

在仓库根目录执行（将示例仓库替换为自己的仓库，先提交交付文件）：

```powershell
$releaseCommit = git rev-parse --short=12 HEAD
$releaseVersion = "video-20260911-$releaseCommit"
$env:SUB2API_IMAGE = "registry.example.com/team/sub2api:$releaseVersion"
docker build --build-arg "VERSION=$releaseVersion" --build-arg "COMMIT=$releaseCommit" -t $env:SUB2API_IMAGE .
if ($LASTEXITCODE -ne 0) { throw '镜像构建失败' }
```

验证成功后按实际发布流程推送镜像，记录仓库摘要、Git 提交、构建日志和验证记录。生产 `deploy/.env` 的 `SUB2API_IMAGE` 推荐填 `自己的仓库/sub2api@sha256:实际摘要`。上线前读取当前容器镜像 ID/摘要，将可重新拉取的旧摘要写入 `SUB2API_PREVIOUS_IMAGE` 并保留仓库镜像和当前本地镜像；该变量仅记录版本，不会自动回退。

```powershell
docker inspect sub2api --format '{{.Config.Image}} {{.Image}}'
docker compose --env-file deploy/.env -f deploy/docker-compose.yml config --quiet
```

## 独立数据库迁移及恢复演练

只在专用验收项目中执行，生产密码、数据卷和数据库地址不得用于下列空库步骤。验收服务仅绑定 `127.0.0.1:18088`，管理页面也使用这个地址，内置前端来自待发布镜像。

1. 使用新的验收项目名启动空库，应用启动会运行全部嵌入迁移。保存启动日志，检查健康状态和迁移记录，不只确认最大版本号。

```powershell
$env:COMPOSE_PROJECT_NAME = 'sub2api-video-release-20260911'
docker compose -p $env:COMPOSE_PROJECT_NAME -f deploy/docker-compose.video-acceptance.yml up -d
docker compose -p $env:COMPOSE_PROJECT_NAME -f deploy/docker-compose.video-acceptance.yml logs backend
docker compose -p $env:COMPOSE_PROJECT_NAME -f deploy/docker-compose.video-acceptance.yml exec -T postgres psql -U sub2api -d sub2api_video_acceptance -c "SELECT filename,checksum,applied_at FROM schema_migrations ORDER BY filename;"
```

2. 核对记录覆盖仓库每一份 SQL（包含历史同号/非事务迁移），235—239 的校验和与构建提交一致。重启 backend，再次检查健康和日志，验证重复启动不重做迁移。完成 [人工验收](MEDIA_VIDEO_ACCEPTANCE.md)，记录订单、余额和账本结果。
3. 对演练库执行备份并恢复到另一个新库。备份先写到容器文件，再复制到主机，避免 PowerShell 文本重定向破坏二进制备份。每个命令的退出码都必须为 0；`pg_restore` 使用 `--exit-on-error`。

```powershell
New-Item -ItemType Directory -Force -Path ./outputs | Out-Null
docker compose -p $env:COMPOSE_PROJECT_NAME -f deploy/docker-compose.video-acceptance.yml stop backend
docker compose -p $env:COMPOSE_PROJECT_NAME -f deploy/docker-compose.video-acceptance.yml exec -T postgres pg_dump -U sub2api -d sub2api_video_acceptance -Fc -f /tmp/video-release.dump
docker compose -p $env:COMPOSE_PROJECT_NAME -f deploy/docker-compose.video-acceptance.yml cp postgres:/tmp/video-release.dump ./outputs/video-release.dump
docker compose -p $env:COMPOSE_PROJECT_NAME -f deploy/docker-compose.video-acceptance.yml exec -T postgres createdb -U sub2api video_restore
docker compose -p $env:COMPOSE_PROJECT_NAME -f deploy/docker-compose.video-acceptance.yml exec -T postgres pg_restore -U sub2api -d video_restore --no-owner --exit-on-error /tmp/video-release.dump
$env:VIDEO_ACCEPTANCE_DB = 'video_restore'
docker compose -p $env:COMPOSE_PROJECT_NAME -f deploy/docker-compose.video-acceptance.yml up -d backend
```

提前创建本机 `outputs` 目录。恢复后核对迁移表、用户可用/冻结余额、`media_video_tasks`、`media_video_ledger`、`usage_logs` 和幂等资金流水，重复查询/下载不得重复扣款。保存备份 SHA256 和耗时。恢复演练失败则不能发布。

4. 空库演练不能替代升级演练：获取生产当前 `schema_migrations` 全表、数据库主版本和一份一致性备份，在有权限的隔离环境恢复生产升级前快照，再用新镜像启动并运行完整迁移链。使用与生产一致的 PostgreSQL 主版本，禁用外部流量/调度，脱敏并按生产数据访问要求保管备份。确认成功后，在该升级后的隔离库上用记录的旧镜像进行兼容性与重启演练。当前提供的验收库默认为 PostgreSQL 16；生产版本不同须调整后再验收。

生产备份必须覆盖数据库、应用数据/加密配置和恢复所需密钥。记录最近可恢复时间、恢复负责人、备份位置和实际恢复耗时；不能仅以“备份命令成功”判定可恢复。

## 回退前处理视频存量

先在所有入口关闭新建：仅阻断 `POST /v1/media/videos`（如有备用入口也同步），保留查询、下载和新版本后台服务。阻断期间同路径的幂等恢复请求也会被阻断，可通过已有任务 ID 查询。等待已进入服务的创建请求结束，至少覆盖当前两分钟提交超时，并持续观察存量变化。

```sql
SELECT status, submission_state, billing_status, count(*), sum(hold_amount)
FROM media_video_tasks
GROUP BY status, submission_state, billing_status
ORDER BY status, submission_state, billing_status;

SELECT task_id, upstream_account_id, upstream_task_id, status,
       submission_state, billing_status, hold_amount, updated_at
FROM media_video_tasks
WHERE billing_status NOT IN ('settled','released')
   OR status IN ('creating','queued','running')
ORDER BY created_at;
```

所有待冻结、已冻结、待结算、待释放和 uncertain/reconciling 订单必须有人处理。`users.frozen_balance` 还可能包含图片订单，不能把全部冻结余额当作视频退款金额。禁止直接改余额或仅因 24 小时到期退款。

已冻结订单尚未处理完时不能直接停掉唯一的新视频后台。优先保留新镜像、关闭新建并完成存量。全部视频资金操作结束后，才按隔离环境验证过的旧镜像摘要回退应用，保留新表、账本及迁移记录。旧镜像不提供视频查询和下载；若还需要这些能力，应继续保留新版本的受控服务，不能把新版本实例放在仍可新建的入口后面。

不要执行删除 235—238 表/列的“降级 SQL”。恢复升级前整库备份会丢失之后的所有业务写入，仅能在停写维护窗口、明确可接受的数据损失范围并完成对账后执行。

## 入口及运营签收

70 MiB 原图 Base64 约 93.34 MiB，另有 JSON 开销。应用默认全局及网关上限均为 256 MiB，仍须核对运行配置和 CDN/WAF/负载均衡/反代的每一层。建议视频入口请求体上限 128 MiB 或更高；真实最大请求经生产入口验证，不能只测直连服务。

可合并到现有 Nginx `server` 中的视频入口示例（替换 upstream；TLS、认证、其他路径沿用生产配置）：

```nginx
location /v1/media/ {
    client_max_body_size 128m;
    client_body_timeout 300s;
    proxy_connect_timeout 15s;
    proxy_send_timeout 300s;
    proxy_read_timeout 900s;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header Range $http_range;
    proxy_set_header If-Range $http_if_range;
    proxy_buffering off;
    proxy_cache off;
    proxy_next_upstream off;
    proxy_pass http://sub2api:8080;
}
```

应用提交上游最多等待两分钟；入口/客户端需允许上传时间加上这段等待。供应商 POST 不得自动重试；超时保留幂等键并对账。下载允许流式传输，验证 `Range: bytes=0-0` 返回 206、`Content-Range`、`video/mp4` 且只有一个字节，同时验证完整下载可播放。CDN 限制不能由 Nginx 配置代替确认。

发布记录必须填写实际值：生产入口及负责人；镜像新旧摘要；数据库版本/迁移清单/备份恢复记录；分组 ID、视频权限、单条价格；上游账号 ID、调度状态及代理；用户 Key 对应分组；对账主负责人和替补；退款审批人及响应时限。分组价格留空时默认 2 内部余额单位，免费价格需显式设置 0；存量按下单快照结算。

结果不确定时保留冻结，由对账负责人使用订单创建时的供应商账号取得请求/任务证据，再由具备受限数据库权限的运营人员调用 `reconcile_media_video_submission`。确认拒单才按拒单处理，确认接单则绑定供应商任务等待后台完成；争议退款经审批调用 `refund_media_video_dispute`，保存证据和审批单。函数定义在迁移 238，禁止授予 PUBLIC 执行权限。函数只安排后续处理，必须继续检查后台释放成功、余额、账本及用户通知后才关闭工单。


## 视频下载保护与手动调整（30 Mbps 初始配置）

后台入口：**系统设置 → 网关 → 视频下载保护**。使用该卡片自己的“保存视频下载设置”按钮；“填入 30 Mbps 推荐值”只修改表单，点击保存才生效。设置保存在现有 settings 表，不需要修改代码或重启服务。并发、请求频率和带宽设置约 2 秒内在实例间更新；超时设置对新下载生效。降低并发不主动中断已开始的下载。

默认：全站同时下载 4 个，每用户 2 个；全站合计 1536 KiB/s（约 12.58 Mbps），每用户合计 512 KiB/s（约 4.19 Mbps）；每用户 60 次/分钟，单次写入超时 30 秒，下载总时限 1800 秒。每用户所有 Key 和 Range 下载共享限额，视频计数与聊天、图片请求独立。允许按实际带宽手动调整，不能用 0 关闭保护。

并发/请求频率超限在连接供应商之前返回 429 和 Retry-After；共享限流保护不可用时返回 503。使用可续期、带唯一标识的 Redis 名额，异常退出会回收；按字节共享限速支持多实例。客户端断开或慢速写入超时会关闭上游并释放名额。应用不保存视频文件、不整段载入内存，响应带 private/no-store 和 X-Accel-Buffering: no；仍需确认生产反代/CDN尊重这些设置及其实际缓冲策略。日志记录下载字节数、耗时和中断状态，不记录视频内容、密钥或签名 URL。

迁移 239 为视频表补充账号生命周期和 Key 账务索引，并在用户/账号删除及视频订单入库时增加数据库保护。存在未完成生成或未结清视频账务时不能删除用户；删除上游账号还须等待成片下载有效期结束。可先停用访问或账号调度，让后台继续收尾。单删、批删和数据库软/硬删除都受约束；创建与删除争用时不会出现检查后刚好插入新订单的漏洞。冲突会在接口显示未处理数量，不自动退款、不抹除账本。该迁移不重算既有余额。

这次保护功能已通过独立 PostgreSQL/Redis 集成验证：删除与创建的竞争、普通用户删除事务回归、跨实例并发/频率、全站与用户实际带宽预算、Range 转发、慢客户端关闭及名额释放。后台表单读取、手动保存、非法值拒绝和填入默认值不自动保存的临时检查通过；前端生产构建通过。临时检查源码删除，仅保留本地日志。正式镜像仍按原 CUSTOM_UPGRADE.md 与 custom-image.yml 的 Git 标签自动发布流程生成，本次没有提交、推送或部署。


## 多模型创建适配

视频创建已根据运行时 `/v1/media/models` 能力校验型号、时长、参考图数量、比例、分辨率及图片大小，不再仅允许 seedance2.5/30 秒。2026-09-08 的能力记录包含 seedance2.5（30 秒/30 张）、seedance2.0 和 seedance2.0fast（5/10/15 秒、9 张）、seedance2.0mini（5/10 秒、9 张）；实际支持范围以有效运行时能力为准。

新建 Laogou 账号不再默认填入仅 seedance2.5 的模型列表，也不会回退填入 Claude 模型。现有账号配置不批量修改，分组统一单条价和旧订单价格快照保持原规则。本次不涉及画布，不增加迁移。不同模型的供应商成本在已有能力响应中没有提供，本轮没有执行付费生成或使用供应商探测脚本。


## 保护功能审查补充修复

尚未发布的迁移 239 已将视频入库的用户/账号父行读取锁改为保持业务值不变的行版本更新。这样，在 Read Committed 下删除会等待并检查已提交订单；Repeatable Read、Serializable 的旧快照删除则产生 40001 序列化冲突，不能漏过新视频。更新不改变余额、账号信息或业务更新时间。已执行旧版 239 的临时验证库应重新创建；不要修改迁移校验和绕过检查。

下载已输出内容后若发生限流保护错误或上游读取错误，处理器中止响应，恢复中间件将标准中止信号交给 net/http；HTTP/1 连接关闭、HTTP/2 仅重置该响应流，不再把截断内容正常结束。首字节前的 JSON 错误、响应体关闭和名额释放逻辑保留。普通异常仍返回原有 JSON 500 并记录堆栈，预期的流中止不打印请求凭据。

独立验证覆盖三种事务隔离级别、用户和账号、软/硬删除的 12 种组合；删除先发生的竞争及保护冲突时 Key/账号分组回滚通过。HTTP/1 和 HTTP/2 下的中途字节预算失败、上游读取失败、首字节前失败和正常完整下载共 8 个场景通过，确认 HTTP/2 连接可继续复用。现有 Recovery 回归通过。临时测试源码与 overlay 已删除，保留本地日志。以上不代替生产代理验证或跨实例动态调参、Redis 故障组合验证。
