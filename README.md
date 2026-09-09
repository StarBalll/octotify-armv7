# OctoTify ARMv7 工作区

> **正式名称：OctoTify ARMv7** —— OctoTify 官方项目的 ARMv7 (linux/arm/v7, 32位) 移植构建。
> 不另起品牌名：应用与功能完全归属上游 **OctoTify**，本项目只承担「32位 ARM 交叉编译与镜像交付」。

## 上游项目与致谢

- **上游官方仓库**：[loommii/OctoTify](https://github.com/loommii/OctoTify) —— 一个轻量自托管通知网关，单 API 广播到多渠道（钉钉/飞书/TG/邮件/Gotify 等）。本项目的全部应用功能、前端 UI 与后端服务均来自该上游，**本仓库不修改任何业务功能**。
- **本仓库**：[StarBalll/octotify-armv7](https://github.com/StarBalll/octotify-armv7) —— 仅承担「32位 ARM 交叉编译 + Docker 镜像交付 + 设备端部署编排」。
- 补丁后源码快照：分支 [`octotify-source-armv7-fixes`](https://github.com/StarBalll/octotify-armv7/tree/octotify-source-armv7-fixes)（完整上游历史 + 4 处环境适配修复 + 2 项功能增强/修复，见 [patches/](./patches/)）。
- 若此项目对你有用，请先给上游 [loommii/OctoTify](https://github.com/loommii/OctoTify) 点个 Star ⭐。

针对 OctoTify 官方不支持 ARMv7 (32位) 的问题，完成全流程交叉编译 + 自定义 Docker 镜像 + ARM 设备部署。
**详细文档：[DELIVERY-ARMv7.md](./DELIVERY-ARMv7.md)** · 设备端速查：[device-package/DEPLOY-CARD.md](./device-package/DEPLOY-CARD.md)

## CI 自动构建（GitHub Actions）

推送 `v*` 标签（如 `v1.1.0-armv7`）即自动：全流程交叉构建（GitHub 云端 x86 runner，无需本地编译机）→ 发布 GitHub Release（附镜像 tar）→ 推送 Docker Hub。手动触发（`workflow_dispatch`）仅构建验证不发布。

启用步骤：
1. 仓库 Settings → Secrets → Actions 添加：
   - `DOCKERHUB_USERNAME`：你的 Docker Hub 用户名
   - `DOCKERHUB_TOKEN`：Docker Hub → Account Settings → Security → New Access Token
2. 推送标签：`git tag v1.1.0-armv7 && git push origin v1.1.0-armv7`
3. 产物：GH Release 附件（`octotify-armv7-v*.tar`）+ Docker Hub `<用户名>/octotify-armv7:latest,v<版本>`
4. 不配置两个 Secret 时仅执行构建验证并发布 Release 附件，跳过 Docker Hub 推送（不报错）。

## 可复现构建（Reproducible Build）

本工程的构建是**输入级可复现**的：`build.sh` 锚定 `VERSION` 声明的上游 commit（`UPSTREAM_BASE`），幂等重放 `patches/` 全部修复，工具链版本固定——任何人重新构建得到的镜像在功能与层内容上等价。
但 `docker save` 的 tar 内嵌构建时间戳（镜像 config/history），**tar 字节哈希每次构建必然不同**——完整性校验请使用各版本 Release 随附的 `.sha256` 文件（对应 `VERSION` 的 `IMAGE_TAR_SHA256`）。

## 命名规范（全项目统一）

| 对象 | 规范名 | 说明 |
|---|---|---|
| 项目名（人读） | **OctoTify ARMv7** | 官方名 + 平台后缀，文档/标题用 |
| 标识符（机器读） | `octotify-armv7` | kebab-case：git 仓库名 / 镜像名 / 归档前缀 |
| Docker 镜像 | `octotify-armv7:latest` + `octotify-armv7:v<版本>` | 双标签：latest 供设备流程，版本标签供追溯 |
| 容器名 | `octotify` | 与官方一致 |
| 数据卷 | `octotify-data` | 与官方一致 |
| 镜像归档 | `out/octotify-armv7-v<版本>.tar` | 版本化命名（正式交付物） |
| 稳定名副本 | `out/octotify-armv7.tar` | 设备端脚本引用的固定名 |
| 部署包 | `octotify-armv7-deploy.tar.gz` | 稳定名（拷贝到设备的工作文件） |
| 版本号 | `vMAJOR.MINOR.PATCH`（tag: `v1.1.0-armv7`） | SemVer；基线见 [VERSION](./VERSION) |
| 分支 | `main`（本工程）· `armv7-fixes`（上游源码补丁） | 补丁可 export 为 patches/ 应用到上游新版 |

## 目录结构

```
├── build.sh                  # 一键全流程：clone→前端→交叉编译→buildx→导出tar（含全部源码补丁，幂等）
├── VERSION                   # 版本基线（端口版本 + 上游锚定 commit + 产物指纹）
├── DELIVERY-ARMv7.md         # 完整交付文档（可行性核查/编译参数差异/实测记录/设备兼容性/FAQ）
├── docker-compose.yml        # 设备端生产编排（持久化+自启+健康检查）
├── import-and-run.sh         # 设备端导入+启动
├── verify.sh                 # 设备端验证 + curl 推送样例
├── deploy/                   # 镜像构建上下文（Dockerfile.armv7 + 产物快照，产物不入库）
├── patches/                  # 上游源码修复的 git patch（可应用到官方新版本）
├── device-package/           # 发往 ARM 设备的部署包内容（tar 不入库）
├── OctoTify/                 # 官方源码 clone（分支 armv7-fixes，补丁见 patches/；不入库）
├── out/                      # 编译产物：octotify-armv7.tar 等（不入库）
└── tools/                    # Go 1.26.8 + bootlin musl 工具链（build.sh 自动下载，不入库）
```

## 快速开始

```bash
# 编译机（x86_64，需 docker+buildx）
./build.sh                 # 产出 out/octotify-armv7.tar
# 组装部署包
cp out/octotify-armv7.tar device-package/ && tar -czf octotify-armv7-deploy.tar.gz -C device-package .
# ARM 设备
docker load -i octotify-armv7.tar && docker compose up -d
```

## 发送消息（核心用法）

部署完成后，任意项目只需一次 HTTP 请求即可把消息广播到所有已绑定渠道。

### 1. 获取推送令牌

登录 WebUI（`http://<设备IP>:5233`）：

- **来源管理 → 新建来源**：创建成功页直接显示推送令牌（本工程的补丁已修复官方"创建后令牌不可见"的问题）；
- 或 **来源管理 → 已有来源 → 查看令牌**：输入登录密码二次验证后查看。

令牌格式如 `src01xxxx...`，**创建时只显示一次，请保存**。在来源上绑定要广播的渠道（钉钉/飞书/TG/邮件/Gotify 等，先在渠道管理里建好）。

### 2. 发送（curl）

```bash
curl -X POST "http://<设备IP>:5233/api/push/<推送令牌>" \
  -H 'Content-Type: application/json' \
  -d '{"title":"构建完成","message":"octotify-armv7 v1.1.0 已发布，请查看 Release"}'
```

- `title` / `message` 均为字符串；响应 JSON 含**各渠道逐一投递的 results**（单渠道故障不影响其他渠道）。
- 同一来源绑定多个渠道时，一次调用即全部广播。
- 消息详情 WebUI 正文支持 Markdown/HTML 富文本展示（GFM + DOMPurify 净化，可切换"原文"）。

### 3. 集成到脚本（示例：CI 发布后通知 / 定时备份通知）

```bash
# 简单封装成函数，脚本里随处可用
notify() {
  curl -sf -X POST "http://<设备IP>:5233/api/push/<推送令牌>" \
    -H 'Content-Type: application/json' \
    -d "$(printf '{"title":"%s","message":"%s"}' "$1" "$2")" >/dev/null
}
notify "夜间备份" "/data 备份完成, 大小 3.2G"
```

完整端到端验证（注册→登录→建来源→推送）可直接跑仓库里的 `verify.sh`；其他可用字段与渠道类型见 `GET /api/channel-types`。

## 上游源码修改（patches/，build.sh 幂等重放）

### 环境适配修复（0001，4 处，局域网 HTTP 场景必需）

| # | 文件 | 问题 |
|---|---|---|
| 1 | locale `page.json` ×2 | 尾随逗号导致 vite8/rolldown 构建失败 |
| 2 | `source/detail.vue` | 复制按钮依赖 clipboard API（仅 HTTPS），局域网 HTTP 必失败 |
| 3 | `source/detail.vue` | 查看令牌经密码二次验证后仍强制脱敏显示 |
| 4 | `source/create.vue` | 创建来源时丢弃后端返回的推送令牌（仅此一次返回） |

### 功能增强（0002，消息详情富文本渲染）

| 文件 | 内容 |
|---|---|
| `message/detail.vue` | 消息详情正文支持 **Markdown（GFM）与 HTML** 安全渲染：marked 解析 + DOMPurify 净化（禁 iframe/object/embed/form 等，链接强制 `_blank`+`noopener`）；正文卡片提供「渲染视图 / 原文」切换；纯文本消息亦获得换行/链接/加粗等基础排版；中性配色兼容亮/暗主题 |
| `web-ele/package.json` + `pnpm-lock.yaml` | 新增依赖 `marked@^18.0.12`、`dompurify@^3.4.15`（锁文件净 +17 行，frozen 安装验证通过） |
| locale `page.json` ×2 | 新增 `page.message.viewRendered` / `viewSource` 文案 |

### 缺陷修复（0003 + 0004，概览/消息列表表格与跳转）

| 文件 | 内容 |
|---|---|
| 后端 `dto.MessageDTO` + `message_service.go`（0003） | 消息列表/筛选接口补齐 `source_name`/`channel_name`/`channel_type`（原只回 ID，UI 表格来源名称/渠道名称列恒为空）；批量 IN 去重填充，避免 N+1 |
| 前端 `dashboard/index.vue`（0003） | 修复点击消息标题仅弹提示无法跳转——改为 `router.push('/message/detail/<id>')`，整行可点击；列名/空态/错误文案接入 i18n |
| 前端 `message/list.vue`（0004） | 修复表头渲染出原始键名 `dashboard.messageTitle` 等——i18n 键误用缺 `page.` 前缀，修正为 `page.dashboard.*`；来源/渠道名称列补 `--` 空值占位 |
| e2e `B-dashboard.spec.ts`（0003） | B-55 用例由「toast 非导航」改为「跳转详情页」断言 |

构建参数关键差异（详见交付文档）：去 `-tags=sonic`（不支持32位）、musl 交叉工具链、`VITE_GLOB_API_URL=/`。
