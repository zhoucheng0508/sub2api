# 视频生成与冻结扣款人工验收

本文件是可重复使用的操作说明，不表示本机或生产已经创建账号、通过验收。镜像构建、完整迁移链、备份恢复和回退步骤见 [发布说明](MEDIA_VIDEO_RELEASE.md)。接口字段见 [视频接口说明](LAOGOU_SEEDANCE_VIDEO_API.md)。

视频使用内部余额，不要求有效订阅，也不累计订阅用量；Key 配额、滚动限额和原子余额检查仍适用。

## 准备

1. 配置 `SUB2API_IMAGE` 为本次构建镜像，启动 `deploy/docker-compose.video-acceptance.yml`。该环境使用独立数据卷；管理页面与 API 均为 `http://127.0.0.1:18088`。默认本地管理员为 `video-acceptance@local.test`，初始密码由 `VIDEO_ACCEPTANCE_ADMIN_PASSWORD` 配置，仅可用于本机隔离环境。
2. 管理员本人阅读并确认管理声明，然后创建 Laogou 分组、启用视频生成权限，单条价格设置为 2。
3. 配置 Laogou 上游账号及密钥，启用调度，绑定该分组；有代理时核对代理有效性。不要把供应商密钥交给验收用户。
4. 创建验收用户，余额设置为 10，创建并绑定上述分组的本站 API Key。记录实际账号/分组 ID，不使用文档中的固定 ID。

## 本站端到端验收

在仓库根目录运行 `./tools/accept_media_video.ps1`，交互输入本站 Key。脚本只有在人工输入 `GENERATE` 后才发送一次创建请求，可能产生真实供应商费用。

保存显示的原始提示词、幂等键和本站任务 ID。中断后使用 `-TaskID vid_...` 继续；首次提交超时且未获得 ID 时，使用原 `-Prompt` 与 `-IdempotencyKey` 恢复已有订单。该方式不会向供应商重新提交，也不会在找不到订单时新建。不要换一个新幂等键尝试恢复。

验收标准：

- 初始可用余额 10、冻结 0；创建后可用 8、冻结 2。
- 成功且成片探测通过后，可用 8、冻结 0，`billing_status=settled`。
- Range 下载返回 206、`video/mp4`、`Content-Range`，完整视频可播放。
- 重复查询、下载与幂等恢复不重复扣款。
- 供应商明确失败时可用余额回到 10、冻结 0，`billing_status=released`。
- 分组改价仅影响新订单；存量按价格快照结算。
- 创建结果不确定时保留冻结，交给指定对账负责人，不能因 24 小时过期自动认定失败。

按用户要求，本次新增的自动化测试代码和离线测试脚本不随交付保留。人工脚本默认下载到 `accepted-video.mp4`，已从 Git 与 Docker 上下文排除。验收报告与媒体证据存入受控附件。

## 供应商只读探测工具

`tools/probe_laogou_video_api.py` 需要 Python 3.10+ 和 `requests`（`py -3 -m pip install requests`）。默认只探测模型接口；只有显式传入 `--create` 才允许一次付费生成。

密钥通过环境变量 `LAOGOU_API_KEY` 注入，或使用 `--key-file` 指向只包含密钥的 UTF-8 文件；不读取、不执行任何 `test.py`，不接受 Python 赋值语句。密钥文件放在仓库外，不应提交。

```powershell
# 先通过本机安全方式设置 LAOGOU_API_KEY，再运行：
py -3 tools/probe_laogou_video_api.py
# 或使用仓库外的纯文本密钥文件：
py -3 tools/probe_laogou_video_api.py --key-file C:/secure/laogou-api-key.txt
```

报告与恢复状态保存在 `outputs/laogou_video_probe/`，该目录不交付。恢复已有付费探测时必须沿用原输出目录，不删除状态记录后重试。供应商探测不经过本站余额体系，不能代替本站端到端验收。
