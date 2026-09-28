# 画布渠道引导与用户密钥连接

画布前端为生图、文本分别提供预设地址。Sub2API 通过既有用户 `/api/v1/keys` 列表和详情接口提供本人的可用 Key，不使用管理员账号接口。

- 主站桥接逻辑集中在 `frontend/src/custom/vote-ai/canvas-key-bridge.ts`；`CustomPageView.vue` 仅挂接 iframe 引用和生命周期。
- 用户 Group DTO 增加公开布尔字段 `image_only`，供 UI 区分图片专用组。管理员 `model_allowlist` 仍不对普通用户公开；不涉及数据库迁移。
- 仅接受配置的父子 origin 对及对应 iframe contentWindow；列表响应只有名称、分组、尾号与禁用原因。选择时重新查询所有权和有效状态，再交付一把用户 Key。
- 主站登录 Token 不进入画布 URL 或消息；未知站点、管理员跨用户 Key、过期/停用/额度耗尽 Key 不可选择。会话切换或页面卸载废弃未完成响应。
- 生图筛选 OpenAI 平台图片专用且允许生图的组；文本筛选 OpenAI 平台非图片专用组。其他平台可手动配置。模型读取时仍以实际 Key 权限为准。
- 雨云父页面 `https://hermes.vote520.com:19443`，画布 `https://hermes.vote520.com:19445`。测试生图 API 使用 19443，文本 API 使用 19446，避免两种渠道共用同一地址而无法区分。
- 生产父页面 `https://ai.vote520.com`，画布 `https://canvas.vote520.com`。生产生图接口 `https://image.vote520.com/v1`，文本接口 `https://ai.vote520.com/v1`。
- 优先部署支持 `image_only` 的 Sub2API，再部署画布；旧主站没有桥接时，画布仍提供手动 Key 接入。
- 单独打开画布没有主站登录桥接，需要从主站进入或手动填写。连接只查询模型列表，不自动发起收费生成，不修改客户售价。

本次增加的是 UI 支持能力，后端仅扩展既有 DTO 的能力标记；没有新增独立业务接口或改动调度计费逻辑。真实浏览器点击验收与自动化接口验证应分别记录。
