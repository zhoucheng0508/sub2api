# STATE Kit 0.3.4 接入

本目录保存 [wangyunjeff/sub2api-state-kit](https://github.com/wangyunjeff/sub2api-state-kit) 的独立插件源码、许可、发布说明和本项目接入工具。使用入口是 **插件管理 → STATE Kit → 配置**。

STATE Kit 为 OpenAI OAuth 账号维护 STATE 票据：逐账号开关、Pro/Team 选择、动态代理采集、业务出口复验、续期、异常重采，以及请求模型/返回模型对照测试。它尝试改善上游改路由问题；模型字段一致不等于能力保证，项目没有并发提升的压测结论。

## 固定来源

- 插件版本：0.3.4。
- 上游提交：`31420adaaf2b29ea1ad6bafd108c268b7b8df441`。
- 原始文件及 SHA-256：`UPSTREAM.json`，插件实现原样保存。
- 宿主基线：本项目的官方 0.2.7 合并提交 `430e43e6e0f8fd8719793aab8d8f2e9ebadc8ee8`。
- 许可证：LGPL-3.0；保留 LICENSE、COPYING.GPL3、NOTICE 和 NOTICE.md。
- 未接入基于 0.2.6 的旧 overlay，避免在同一实例运行两套 STATE 实现。

## 宿主适配

宿主改动位于 backend 和 frontend 正常源码目录，不需要部署时再次打补丁。

- 新增 `ListResources` / `ResolveProxy` RPC。只授予具有 OpenAI OAuth 账号权限的插件；页面目录只返回必要元数据，代理认证 URL 仅通过后端 RPC 提供。
- 复用官方最新的账号范围和 `AccountInfo` 元数据；不覆盖 v2 协议已有字段。
- 新增运行中插件 `RunAction` RPC、管理员 actions 路由和受二次验证保护的页面消息接口。
- 添加分组/账号/代理下拉目录，保留沙箱和页面消息令牌校验。
- 资源目录经 `ProvidePluginManager` 装配，重新生成 Wire 也会保留。
- 0.3.4 插件严格要求 HostService v1。宿主优先提供 v2，只有插件明确回复 `unsupported host service API` 才尝试 v1；其他插件仍保留 v2，其他错误不触发重试。
- 无数据库迁移，不修改现有账号配置、代理、密钥或插件信任配置。

`plugin/host-adapter/` 保存上游原始补丁作为参考。该补丁基于较早的 0.2.7 提交，**不要再对当前源码直接应用**，否则可能覆盖这里已保留的官方接口改进。

## 安装使用

1. 构建并部署包含本次适配的宿主前后端。
2. 将 `plugin/release/trusted-publisher.yaml` 的发布者条目追加到服务器已有的 `plugins.trusted_publishers`，保留其他配置并重启应用。
3. 在插件管理上传 [官方签名的 0.3.4 安装包](https://github.com/wangyunjeff/sub2api-state-kit/releases/download/v0.3.4/sub2api-state-kit_plugin_v0.3.4.s2plugin)。支持 Linux amd64/arm64、macOS arm64；官方包不包含 Windows 运行时。
4. 启用插件进程，进入配置，填自己的动态代理，按分组选账号并逐个启用。所有 STATE 开关默认关闭；没填账号与代理不会自动采集。

本次已在外部 output 目录准备安装包并校验发布者签名。包 SHA-256：

```text
e09ac56a13af19f819d3107047f05766401287de853971d406940b68edc6cb5b
```

已在独立测试机安装官方签名包并完成验收，详见 [测试记录](../../docs/STATE_KIT_ACCEPTANCE_20260921.md)。没有启用真实 STATE 账号或部署生产环境。插件目前面向单应用实例。动态代理负责采集，业务请求仍走账号配置的出口。

## 开发与验证

插件是独立 Go 模块，在 `plugin` 目录执行：

```sh
go test ./...
node --test ui-tests/app.test.cjs ui-tests/bridge.test.cjs
go build -buildvcs=false -o /path/outside/repository/state-kit ./cmd/state-kit
```

宿主检查在 backend / frontend 各自目录执行：

```sh
go test -tags=unit ./internal/service ./internal/handler/admin ./internal/server/routes ./cmd/server ./pkg/pluginapi/... -run 'Plugin|BuildHostServices|Wire'
pnpm exec vitest run src/views/admin/__tests__/pluginResources.spec.ts src/views/admin/__tests__/PluginsView.spec.ts
pnpm typecheck
pnpm build
```

真实插件进程测试，从项目根目录执行：

```sh
python integrations/state-kit/scripts/test_host_local.py --openssl /path/to/openssl
```

该工具构建本机插件，使用临时随机签名密钥制作**仅供测试**的包，通过宿主真实安装器、子进程、gRPC 和本地 HTTP 模拟接口运行测试。临时私钥不会加入仓库，也不修改宿主真实信任配置。测试包不是作者发布包。测试使用的签名 key_id 仅存在于隔离测试配置中。

`scripts/package_plugin.py` 是原样保存的上游发布工具，固定使用作者的签名身份，不能拿它假冒作者签名发布。本地开发可直接构建运行时；若要发布修改过的插件，应使用自己的发布者身份和仓库外的私钥配置签名流程。

本次验证：插件 Go 测试 93 项、插件 UI 测试 44 项、宿主定向测试 88 项、宿主页面测试 6 项、真实进程契约测试 13 项均通过（Go 数量含子测试）。常规宿主套件按原条件跳过 1 项外部插件测试；这里另用真实 STATE Kit 进程契约测试覆盖安装、握手、KV、账号目录、默认关闭、二进制与 SSE 转发、取消及重启。前端类型检查、ESLint、生产构建及宿主嵌入前端的构建通过。

未使用实际 OpenAI 账号和代理测试票据效果，未进行生产端到端验收。

## 后续更新

拉取 STATE Kit 新提交后，先依据 `UPSTREAM.json` 比较原始文件，再审查宿主协议和接口变化；不要用旧 overlay 或原始宿主补丁整片覆盖。重新执行以上检查并更新来源提交与哈希记录。保留当前宿主合并提交和插件版本可用于回退。
