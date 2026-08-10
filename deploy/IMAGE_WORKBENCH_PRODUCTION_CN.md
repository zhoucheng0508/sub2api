# Vote 无限画布生产配置清单

本清单配合 `docs/INFINITE_CANVAS_IMAGE_WORKBENCH_DESIGN_CN.md` 和 `docs/ASYNC_IMAGE_TASKS.md` 使用。所有命令中的地址、容器名和证书路径必须按生产实际布局替换；不得把 API Key、对象存储密钥或数据库凭据写入仓库。

## 1. 发布前配置

### 1.1 生图分组

`image生图` 分组必须同时满足：

- 平台为 OpenAI；
- 只包含指定的 Pro OAuth 账号；
- `allow_image_generation=true`；
- 自定义模型列表已启用，并且唯一模型为 `gpt-image-2`；
- 账号模型映射只允许 `gpt-image-2`；
- 关闭未映射模型透传；
- `fallback_group_id=null`；
- `fallback_group_id_on_invalid_request=null`；
- 分组倍率保持当前生产值，不在本次发布中改价。

测试用户必须创建只绑定该分组的独立 API Key。不得使用混合分组或文本分组 Key。

### 1.2 私有对象存储

可以复用数据库备份使用的 S3/R2 账号，但必须新建独立私有桶，例如 `sub2api-images`，不得与备份共用桶。Sub2API 配置基线：

```yaml
image_storage:
  enabled: true
  endpoint: "<S3-compatible endpoint>"
  region: "auto"
  bucket: "sub2api-images"
  access_key_id: "<secret>"
  secret_access_key: "<secret>"
  prefix: "images/"
  force_path_style: false
  public_base_url: ""
  presign_expiry_hours: 24
  max_download_bytes: 33554432
```

桶策略要求：

- 禁止匿名读取和目录列举；
- 对 `images/` 设置 2 天自动删除生命周期；
- 桶 CORS 只允许 Origin `https://canvas.vote520.com`；
- 只允许 `GET`、`HEAD`；
- 不允许浏览器执行 `PUT`、`POST`、`DELETE`；
- 对象存储凭据只进入 Sub2API 服务端。

参考 CORS 规则（字段名按供应商控制台转换）：

```json
[
  {
    "AllowedOrigins": ["https://canvas.vote520.com"],
    "AllowedMethods": ["GET", "HEAD"],
    "AllowedHeaders": ["*"],
    "ExposeHeaders": ["Content-Length", "Content-Type", "ETag"],
    "MaxAgeSeconds": 3600
  }
]
```

### 1.3 image.vote520.com

1. 将 `deploy/nginx/image.vote520.com.conf.example` 复制到服务器 Nginx 配置目录。
2. 修改 TLS 证书路径和私有 Sub2API upstream。
3. 执行 `nginx -t`，通过后再 reload。
4. Cloudflare 开启代理与严格 TLS。
5. 为 `image.vote520.com/*` 设置 CDN Cache Bypass；不得缓存 API 响应。
6. Cloudflare 与 Nginx 请求体上限均设置为 100 MB。
7. 不为兼容同步生图接口暴露源站或关闭 Cloudflare；Canvas 始终使用异步接口。

本地静态守卫：

```bash
bash deploy/tests/image-gateway-nginx-test.sh
```

部署网关后，先执行不会提交生图任务、不会触发扣费的 PowerShell 预检：

```powershell
$env:IMAGE_GATEWAY_API_KEY = "<绑定 image生图 分组的测试 Key>"
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\deploy\tests\Test-ImageWorkbenchGateway.ps1
Remove-Item Env:IMAGE_GATEWAY_API_KEY
```

脚本只调用`OPTIONS`和`GET`，验证路由白名单、CORS、`Cache-Control: no-store`、匿名访问被拒绝、Key鉴权以及模型列表精确为`gpt-image-2`。不要把Key作为命令行参数保存到Shell历史；不设置环境变量时仍会执行无鉴权的路由、CORS、缓存和匿名鉴权拒绝检查。`-ExecutionPolicy Bypass`只作用于该子进程，不修改系统持久执行策略。

### 1.4 canvas.vote520.com

- 静态指纹资源长期缓存；HTML 与运行时配置 `Cache-Control: no-store`；
- `frame-ancestors` 只允许自身和 `https://ai.vote520.com`；
- `connect-src` 只允许自身、`https://image.vote520.com` 和实际私有桶签名 URL 域名；
- `img-src` 只允许自身、`blob:`、`data:` 和实际私有桶签名 URL 域名；
- 禁止第三方插件、分析脚本和远程脚本；
- `robots.txt` 与页面元信息保持 `noindex`。

对象存储域名确定前不得用 `https:` 通配符替代精确 CSP 域名。

## 2. 发布验收

### 2.1 路由与 CORS

使用无敏感信息的测试 Key 验证：

- 允许的 `/v1/models`、`/v1/sub2api/billing`、同步/异步 Images 和任务轮询路由可达；
- `/v1/chat/completions`、`/v1/responses`、`/api/v1/admin/*`、用户和运维接口在图片域名返回 404；
- `https://canvas.vote520.com` 预检响应包含唯一的 `Access-Control-Allow-Origin`；
- 其他 Origin 不包含 `Access-Control-Allow-Origin`；
- 所有 API 响应包含 `Cache-Control: no-store`；
- Nginx 日志没有 Authorization、API Key、查询串或签名 URL 参数。

上述无副作用项目应先使用`deploy/tests/Test-ImageWorkbenchGateway.ps1`自动执行并保存输出，再进入真实生图、审核、计费和并发测试。

### 2.2 Key 与模型隔离

- 生图专用 Key 的 `/v1/models` 精确返回一个模型：`gpt-image-2`；
- 混合 Key 在 Canvas 连接检查中被拒绝；
- 两个 Pro 账号均不可用时直接失败，不跨分组回退；
- 账号模型白名单外的模型不能调度。

### 2.3 任务、审核和计费

- 正常提示词只审核一次；参考图 Base64 不进入审核请求或日志；
- 审核拒绝和审核服务全故障时不创建任务、不调用 Pro 账号、不扣生图费用；
- 同一 Key 第二个活动任务返回 `429 IMAGE_TASK_ALREADY_ACTIVE` 与 `Retry-After: 3`；
- 其他 Key 可独立提交；其他 Key 轮询已有任务统一返回 404；
- 重复轮询不重复扣费或上传；
- 对象上传失败时任务失败，Redis 不保存 Base64，并释放 Key 活动槽位；
- 成功任务只扣一次，倍率和改造前一致。

### 2.4 灰度

按设计文档的 2、4、8、12、20 总并发逐级压测，每档 3 批。只启用最后一个同时满足成功率、错误率、P95、账号状态、计费和资源占用指标的档位。

先开放管理员和 1 至 3 名测试用户，累计至少 100 张真实生成且验收指标达标后，再把主站菜单开放给所有用户。

## 3. 回滚

1. 主站隐藏“无限画布”菜单。
2. Nginx 或 Cloudflare 禁止新的异步任务提交，但保留旧任务轮询路由。
3. 关闭 Sub2API 异步图片任务开关；已创建任务仍可查询。
4. `canvas.vote520.com` 切换维护页。
5. 回滚到上一版 Canvas 静态产物和 Sub2API 镜像。
6. 不删除 Redis 任务、数据库记录或对象存储中的在途结果。
7. 验证 `ai.vote520.com` 文本业务未受影响。
