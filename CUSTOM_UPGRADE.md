# Sub2API 定制版升级流程

本文档记录 `custom` 分支同步官方 `upstream/main` 的固定流程。定制版不得使用管理后台在线升级，避免官方二进制或镜像覆盖定制功能。

## 核心原则

- 使用 merge，不使用 rebase，不改写既有历史。
- 每次升级先创建备份分支，再创建同步分支。
- 重点审查官方改动与定制文件的交集，不重复验证官方全部功能。
- 本地只运行定制功能及交叉文件相关的定向测试、类型检查和编译检查。
- 出现冲突时，处理后必须停下，由项目负责人最终审核冲突文件。
- `custom` 推送并通过 CI 后才能创建镜像标签。
- 数据库迁移是单向的，服务器升级前必须备份 PostgreSQL 和部署配置。

## 定制范围

主要定制集中在：

- 前端首页、定价页、文档页及相关样式、路由和测试。
- 后端文档设置服务、公开/管理接口及路由。
- `.github/workflows/custom-image.yml` 定制镜像构建流程。

升级前使用以下命令重新确认实际范围：

```bash
git diff --name-status upstream/main...custom
git diff --stat upstream/main...custom
```

## 同步步骤

假设目标版本为 `vX.Y.Z`：

```bash
git switch custom
git status --short --branch
git fetch upstream --prune --tags
git fetch origin --prune

git branch backup/custom-before-vX.Y.Z custom
git switch -c sync/upstream-vX.Y.Z custom
git merge --no-ff upstream/main
```

合并前先计算官方本次改动与定制文件的交集，交集文件是主要审查对象。无文本冲突也必须检查自动合并后的语义，尤其是路由、设置 DTO、首页条件分支和测试 mock。

## 冲突处理

1. 记录全部冲突文件并立即通知项目负责人。
2. 对照 merge base、`custom` 和 `upstream/main` 三方意图逐块处理。
3. 保留定制产品行为，同时接入官方新增字段、路由或安全修复。
4. 检查无残留冲突标记并运行 `git diff --check`。
5. 完成定向测试后停下，等待项目负责人审核冲突文件。
6. 审核通过后再完成 merge commit、推进 `custom` 和发布。

## 定向验证

不在本地运行全量测试。测试范围按本次交叉文件选择，通常包括：

- 首页、文档 URL、Logo 安全、管理分组和用户资料相关 Vitest。
- 新增官方功能与定制页面交叉时对应的官方定向用例。
- `vue-tsc --noEmit` 类型检查。
- 后端文档服务、设置处理器测试和路由包编译检查。
- `git diff --check`。

测试桩因定制组件、路由、i18n 或 WebGL 产生不兼容时，只适配测试 mock 和断言，不弱化生产逻辑。

## 发布步骤

审核通过后：

```bash
git commit
git switch custom
git merge --ff-only sync/upstream-vX.Y.Z

git push -u origin backup/custom-before-vX.Y.Z
git push -u origin sync/upstream-vX.Y.Z
git push origin custom
```

等待 `custom` 的远端 CI 通过，再创建镜像标签：

```bash
git tag custom-vX.Y.Z-1
git push origin custom-vX.Y.Z-1
```

标签必须指向 `origin/custom` 可达的提交，否则自定义镜像工作流会拒绝构建。

## 备份保留

- Git 升级备份分支建议保留最近 5 组；超过 5 组时提醒清理最早分支。
- 服务器升级备份至少保留 3-7 天，确认运行稳定后再清理。
- 建议保留最近 5 次升级备份，并保留 2-3 个月的月度备份。
- 超过 90 天的普通升级备份应先列出并提醒确认，再执行删除。
- 不得把 PostgreSQL 数据目录、Redis 数据目录或 Docker volume 当作普通备份删除。

服务器更新镜像前必须确认实际 Compose 文件，可通过容器标签查看：

```bash
docker inspect sub2api --format '{{index .Config.Labels "com.docker.compose.project.config_files"}}'
```

升级时只重建 `sub2api` 服务，不无故重启 PostgreSQL 和 Redis。
