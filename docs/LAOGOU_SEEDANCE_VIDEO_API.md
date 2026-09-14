# Laogou Seedance 视频接口实现说明

计费规则：视频独立计价并使用内部余额冻结、结算和释放；不以订阅有效性或订阅日/周/月限额作为准入条件，不累计订阅用量。Key 配额与滚动限额继续适用，视频用量不关联为订阅消费。

供应商地址：`https://api.laogou.org`。供应商已验证接口：

```text
GET  /v1/media/models
POST /v1/media/videos
GET  /v1/media/videos/{upstream_task_id}
GET  /v1/media/videos/{upstream_task_id}/content
```

Sub2API 对客户端提供自己的任务 ID，并保存供应商任务 ID 映射。客户端不接触供应商 API Key。任务访问有效期 24 小时，过期后返回 404；永久资金流水和订单快照不随任务展示数据清理。创建结果不确定、尚待对账的订单保留冻结和记录。

## 对外接口

```text
POST /v1/media/videos
GET  /v1/media/videos/tasks
GET  /v1/media/videos/{task_id}
GET  /v1/media/videos/{task_id}/content
GET  /v1/media/models
```

所有接口要求：

```http
Authorization: Bearer <Sub2API API Key>
```

## 创建任务

```http
POST /v1/media/videos
Content-Type: application/json
Idempotency-Key: <unique-key>
```

文生视频：

```json
{
  "model": "seedance2.5",
  "prompt": "一个火柴人在白色背景中行走，电影感镜头",
  "duration": 30,
  "ratio": "16:9",
  "resolution": "720p"
}
```

图生视频：

```json
{
  "model": "seedance2.5",
  "prompt": "让图片中的主体自然运动",
  "duration": 30,
  "ratio": "16:9",
  "resolution": "720p",
  "images": ["data:image/png;base64,<base64-data>"]
}
```

`images` 已通过真实生成验证。每个元素是图片 Data URL，当前确认供应商接受 `data:image/png;base64,...`。

| 字段 | 类型 | 必需 | 规则 |
|---|---|---:|---|
| `model` | string | 是 | 使用 `/v1/media/models` 返回的模型 ID |
| `prompt` | string | 是 | 非空文本 |
| `duration` | integer | 是 | 必须包含在所选模型的 `durations` 中 |
| `ratio` | string | 是 | 使用模型级 `ratios`；缺省时使用能力响应顶层 `ratios` |
| `resolution` | string | 是 | 使用模型级 `resolutions`；缺省时使用能力响应顶层 `resolutions` |
| `images` | array[string] | 否 | 不超过所选模型的 `max_reference_images`，并满足单张/总字节限制 |

能力接口还声明：单张图片最大 12 MiB，所有图片总计最大 70 MiB。

创建成功建议返回 `202 Accepted`：

```json
{
  "task_id": "vid_01K...",
  "object": "media.video_task",
  "status": "queued",
  "model": "seedance2.5",
  "duration_seconds": 30,
  "downloadable": false,
  "created_at": 1788841904,
  "expires_at": 1788928304
}
```

幂等要求：

- 每次新建任务生成新的 `Idempotency-Key`。
- 网络超时或 5xx 后恢复已有订单必须复用原值；供应商幂等合同未确认，服务端不自动重发创建 POST。不确定结果进入待对账，详见 `../MEDIA_VIDEO_REVIEW_FIXES.md`。
- 相同 Key 和相同请求体返回原任务。
- 相同 Key 但请求体不同返回 `409`。
- 调用供应商前持久化幂等记录，避免重复创建。

## 进行中任务列表

```http
GET /v1/media/videos/tasks?status=queued,running&limit=20&cursor=<opaque-cursor>
```

默认只返回当前用户和当前 API Key 的 `queued`、`running` 任务：

```json
{
  "object": "list",
  "data": [
    {
      "task_id": "vid_01K...",
      "model": "seedance2.5",
      "status": "running",
      "progress": null,
      "duration_seconds": 30,
      "ratio": "16:9",
      "resolution": "720p",
      "created_at": 1788841904,
      "updated_at": 1788842500,
      "expires_at": 1788928304
    }
  ],
  "has_more": false,
  "next_cursor": null
}
```

供应商状态没有稳定的 `progress` 字段，`progress` 可以为 `null`。列表应读取本地任务索引，不要为每一项同步请求供应商。

## 查询单个任务

```http
GET /v1/media/videos/{task_id}
```

完成响应：

```json
{
  "task_id": "vid_01K...",
  "object": "media.video_task",
  "status": "succeeded",
  "model": "seedance2.5",
  "duration_seconds": 30,
  "downloadable": true,
  "created_at": 1788841904,
  "completed_at": 1788849692,
  "expires_at": 1788928304
}
```

失败响应：

```json
{
  "task_id": "vid_01K...",
  "object": "media.video_task",
  "status": "failed",
  "downloadable": false,
  "error": {
    "type": "video_generation_error",
    "message": "生成失败，请稍后重试"
  },
  "created_at": 1788841904,
  "completed_at": 1788844336,
  "expires_at": 1788928304
}
```

已确认状态：`queued`、`running`、`succeeded`、`failed`。未知状态应原样保存并按非终态处理。

## 获取成片

```http
GET /v1/media/videos/{task_id}/content
Accept: */*
Range: bytes=0-1048575
```

后端验证任务归属和 `downloadable=true` 后，使用创建任务时绑定的供应商账号请求上游 `/content`，并流式转发：

```http
Content-Type: video/mp4
Accept-Ranges: bytes
Content-Disposition: inline; filename="video.mp4"
```

供应商已验证支持 `206 Partial Content`、`Content-Range` 和断点下载。不得将完整视频一次性读入内存；禁止跟随未经校验的外部重定向。

## 能力发现

```http
GET /v1/media/models
```

2026-09-08 保存的供应商真实能力响应示例（不是永久模型白名单）：

```json
{
  "platform": "seedance",
  "models": [
    {"id": "seedance2.5", "durations": [30], "max_reference_images": 30},
    {"id": "seedance2.0", "durations": [5, 10, 15], "max_reference_images": 9},
    {"id": "seedance2.0fast", "durations": [5, 10, 15], "max_reference_images": 9},
    {"id": "seedance2.0mini", "durations": [5, 10], "max_reference_images": 9}
  ],
  "ratios": ["1:1", "3:4", "4:3", "9:16", "16:9", "21:9"],
  "resolutions": ["720p"],
  "camera_movement": ["auto", "fixed"],
  "max_image_bytes": 12582912,
  "max_images_total_bytes": 73400320
}
```

后端按账号及凭证/代理指纹缓存有效能力 5 分钟，模型列表接口和创建校验共用该能力结构。账号或凭证变化不会沿用其他账号的缓存。响应按本站上传安全上限归一化：最多 30 张、单张最多 12 MiB、总计最多 70 MiB；上游更严格时采用更严格的值。

创建前按候选账号的实际能力选择支持请求参数的账号，然后才入库和冻结余额。能力不可用或格式无效返回 503，不根据历史记录猜测支持范围；模型/时长/图片数量不符合已知能力返回 400。组内有多个账号时，列表接口返回首个可用账号的一份完整能力快照，创建时仍按选中账号校验，不把不同账号的能力随意拼成可能无法生成的组合。

正常幂等重试和不确定订单的查询恢复不依赖最新模型目录，不会因为旧模型下架而重新提交。已有订单的型号、时长和价格快照不改变。

现有探测能力响应没有价格字段。本站继续按 `video_price_per_request` 分组统一单条价计费，不因切换模型/时长自动更改售价；`GET /v1/media/billing` 返回当前分组单价。供应商不同模型的成本仍需单独取得有效报价，不能从模型能力列表推导。本次多模型改动不增加数据库迁移，不重算旧账单。

## 本地任务存储

至少保存：

```text
task_id, upstream_task_id, user_id, api_key_id, group_id, upstream_account_id
model, prompt_hash, duration, ratio, resolution, has_images
idempotency_key_hash, request_hash, status, progress, downloadable, error
created_at, updated_at, completed_at, expires_at, last_polled_at, next_poll_at
billing_status
```

任务展示数据按 24 小时过期隐藏；已结算或已释放订单清理前保留永久订单快照，资金流水独立长期保存。创建结果不确定的订单不按到期自动退款或删除。图片 Base64 不应长期保存；仅保留 `has_images` 和请求摘要。

## 后台轮询

后台仅处理未过期的 `queued`、`running` 任务，调用供应商状态接口并更新本地记录。对 `429/502/503/504` 使用指数退避并尊重 `Retry-After`。多实例部署使用租约或 Redis 锁。成功或失败后停止轮询，但记录保留至 24 小时到期。

查询和下载必须使用创建时绑定的上游账号，不能轮询时重新选号。

## 错误响应

```json
{
  "error": {
    "type": "invalid_request_error",
    "message": "resolution is not supported",
    "code": "VIDEO_INVALID_PARAMETER"
  }
}
```

建议状态码：`400` 参数错误，`401` Key 无效，`403` 无权限，`404` 不存在/无权/已过期，`409` 幂等冲突，`429` 限流，`502` 上游错误，`503` 无可用账号，`504` 上游超时。

## 安全边界

- 供应商 API Key 只保存在后端账号凭证中。
- 任务按用户和 API Key 隔离，不暴露供应商原始任务 ID。
- 图片 Data URL 不写入普通日志。
- 任务过期统一返回 404。
- 供应商未确认取消接口，第一版不实现真正取消。


## 下载资源保护

视频下载具有独立的全站/用户并发、用户请求频率及按字节带宽限制，同一用户的所有 API Key 和 Range 请求共享限额。默认每用户 2 路、全站 4 路，用户合计 512 KiB/s、全站 1536 KiB/s；管理员可在系统设置的“视频下载保护”卡片修改。超限返回 429 和 Retry-After，保护服务不可用返回 503。客户端应遵守 Retry-After，发生下载中断时使用 Range 续传，不要重新创建视频。

下载转发不持久化 MP4，响应设置 private/no-store 与 X-Accel-Buffering: no。创建视频的并发与计费规则不因下载设置改变。
