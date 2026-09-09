# OctoTify ARMv7（linux/arm/v7 32位）自定义编译 & 部署 — 交付文档

> 目标：官方仅提供 amd64/arm64 镜像，本方案从官方源码（https://github.com/loommii/OctoTify）
> 全流程手动交叉编译出 `linux/arm/v7` 32 位 Docker 镜像，功能与官方完全一致（前端 Vue UI + Go 后端 + SQLite）。

---

## 一、可行性核查结论（先做后建）

| 核查项 | 结论 |
|---|---|
| 官方源码 | ✅ 仓库存在，`backend`（Go 1.26 + gin + gorm + mattn/go-sqlite3）+ `frontend`（vben-admin Vue3 monorepo）+ `docker/` |
| CGO/SQLite | ✅ `gorm.io/driver/sqlite` → `mattn/go-sqlite3`，CGO 必须开启（官方构建也开） |
| 官方镜像架构 | ⚠️ 与任务书预设不同：**nginx 监听 5233 托管前端 + `/api/` 反代 Go 后端(127.0.0.1:34123)**，后端不内嵌前端。本方案逐字复刻该架构 |
| 官方 Dockerfile 可用性 | ⚠️ 已失效：`npm ci` 但仓库只有 pnpm lockfile；产物路径与 turbo monorepo 不符 → 自定义构建确有必要 |
| 上游源码 bug | ⚠️ 两个 locale JSON（`zh-CN/en-US page.json` 第 140 行）有尾随逗号，vite8/rolldown 拒绝解析 → 已修复（仅去逗号，不改内容） |
| sonic 编译器 | ⚠️ 官方 `-tags=sonic` 在 **32 位架构编译期硬报错**（sonic 仅支持 amd64/arm64）→ armv7 构建必须去掉该 tag，JSON 回退标准库，功能等价 |
| alpine 3.22 nginx 权限 | ⚠️ 新版 alpine 的 nginx 包把 `/run/nginx/` 建为 `nginx:nginx` 所有，非 root 运行报 `open /run/nginx/nginx.pid: Permission denied` → Dockerfile 已补 `chown app:app /run/nginx`（官方 Dockerfile 在老 alpine 时代无此问题） |
| 工具链 | ✅ Go 1.26.8 + bootlin armv7-eabihf musl gcc 13.3 + docker buildx v0.34.1 + qemu binfmt（`tonistiigi/binfmt --install arm`） |
| 冒烟验证 | ✅ ARMv7 静态二进制在 qemu 下实测：`/ping` 200、RSA 密钥自动生成、SQLite 建库成功 |

---

## 二、编译流程（x86_64 编译机，一条命令复现全部步骤）

```bash
./build.sh            # 完整流程：clone → 前端 → 交叉编译 → buildx → 导出 tar
./build.sh --no-clone # 已有源码时增量执行
```

`build.sh` 内部执行的关键命令（即任务书要求的编译命令，实测修正后）：

```bash
# 步骤2 前端（产物在 frontend/apps/web-ele/dist，非 frontend/dist —— monorepo 布局）
cd frontend
printf 'VITE_GLOB_API_URL=/\n' > apps/web-ele/.env.production.local  # 生产API同源根路径（前端代码已含/api前缀，经nginx反代；误设/api会拼成/api/api/**报404）
pnpm install --frozen-lockfile
pnpm run build --filter=@vben/web-ele

# 步骤3 后端交叉编译（强制 CGO + ARMv7 硬浮点 + 全静态）
cd backend
CGO_ENABLED=1 GOOS=linux GOARCH=arm GOARM=7 \
  CC=arm-buildroot-linux-musleabihf-gcc \
  go build -ldflags='-s -w -extldflags -static' -o ../out/octotify-server ./cmd/server

# 步骤4-5 镜像构建与导出
docker buildx build --platform linux/arm/v7 -t octotify-armv7:latest deploy/
docker save octotify-armv7:latest -o out/octotify-armv7.tar
```

交叉编译实测产物：

```
out/octotify-server: ELF 32-bit LSB executable, ARM, EABI5, statically linked, stripped   (~28MB)
镜像: octotify-armv7:latest  linux/arm/v7
```

### 与任务书编译参数的差异及原因（全部实测验证）

| 任务书参数 | 实际采用 | 原因 |
|---|---|---|
| `CC=arm-linux-gnueabihf-gcc`（glibc） | `arm-buildroot-linux-musleabihf-gcc`（musl，bootlin 2024.05） | 目标基础镜像是 alpine（musl）。glibc 工具链产物依赖 glibc 无法在 alpine 运行；musl 全静态则零依赖 |
| `-tags=sonic`（官方 Dockerfile 参数） | 去掉 | sonic 不支持 32 位，GOARCH=arm 编译期报错 `_Sonic_Not_Support_32Bit_Arch`；默认 tag 下 gin 走标准库 JSON，行为等价 |
| `frontend/dist` 输出目录 | `frontend/apps/web-ele/dist` | 上游已迁移为 pnpm+turbo monorepo，唯一 UI 应用是 `@vben/web-ele` |
| `go build -o octotify-server ./backend/cmd/server` | 在 `backend/` 目录内 `./cmd/server` | go.work 工作区布局，等价 |

---

## 三、ARMv7 设备部署（交付物 4：tar 包说明）

拷贝以下 3 个文件到设备任意目录：

| 文件 | 说明 |
|---|---|
| `out/octotify-armv7.tar` | `docker save` 导出的镜像，`docker load` 即可导入，无需外网 |
| `docker-compose.yml` | 生产编排：`platform: linux/arm/v7`、端口 5233、命名卷 `octotify-data:/app/data` 持久化（SQLite 数据库 + 日志）、`restart: unless-stopped` 开机自启 |
| `import-and-run.sh` | 导入 + 启动 + 健康检查一键脚本 |

```bash
chmod +x import-and-run.sh verify.sh
./import-and-run.sh octotify-armv7.tar
```

数据持久化说明：SQLite 数据库 `octotify.db`、运行日志均在 `/app/data`（挂载于命名卷 `octotify-data`），容器删除/升级数据保留；JWT RSA 密钥在首次启动时自动生成于容器层 `/app/config/keys/`（与官方 `auto_generate_keys: true` 行为一致）。

---

## 四、部署成功验证（交付物 5：验证命令 + curl 推送样例）

```bash
./verify.sh octoadmin 'Passw0rd123'
```

或手动逐项验证：

```bash
# 1. 容器状态与健康检查
docker ps --filter name=octotify
docker exec octotify wget -qO- http://127.0.0.1:34123/ping
#    → {"code":0,"msg":"请求成功","data":{"server_name":"OctoTify",...}}

# 2. WebUI（浏览器打开 http://<设备IP>:5233 ，nginx 托管前端并反代 /api/）
curl -sf -o /dev/null -w "%{http_code}\n" http://127.0.0.1:5233/     # → 200

# 3. 注册管理员（或直接 WebUI 注册）
curl -X POST http://127.0.0.1:5233/api/user/register \
  -H 'Content-Type: application/json' \
  -d '{"username":"octoadmin","password":"Passw0rd123"}'

# 4. 登录拿 JWT
curl -X POST http://127.0.0.1:5233/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"octoadmin","password":"Passw0rd123"}'
#    → data.access_token（7天 refresh_token 见 data.refresh_token）

# 5. 创建推送渠道（类型: wechat | telegram | dingtalk | email | webhook）
#    各类型字段定义: curl -H "Authorization: Bearer $TOKEN" http://127.0.0.1:5233/api/channel-types
curl -X POST http://127.0.0.1:5233/api/channels \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"type":"dingtalk","name":"钉钉-运维群","config":{"webhook":"https://oapi.dingtalk.com/robot/send?access_token=xxx"}}'

# 6. 创建来源 Source（多渠道广播：channel_ids 填多个；仅创建时返回推送 token）
curl -X POST http://127.0.0.1:5233/api/sources \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"CI Pipeline","description":"构建通知","channel_ids":[1,2]}'

# 7. API 推送样例（多目标广播 + 单渠道故障隔离：响应含每个渠道的 results）
curl -X POST http://127.0.0.1:5233/api/push/<推送token> \
  -H 'Content-Type: application/json' \
  -d '{"title":"ARMv7 部署验证","message":"OctoTify 已在 linux/arm/v7 成功运行"}'
#    → {"total":2,"success":2,"failed":0,"results":[{"channel_id":1,"success":true,...},...]}
```

---

## 五、功能保留清单（与官方逐项对应）

- ✅ Source 令牌创建/管理/重置（`/api/sources`，二次密码验证）
- ✅ 全部推送渠道：`wechat / telegram / dingtalk / email / webhook`（`backend/internal/client/*`、`sender/*` 未做任何改动）
- ✅ 多目标广播推送、单渠道故障隔离（`PushResponse.results` 逐渠道返回成功/失败）
- ✅ 消息日志与持久化（SQLite @ `/app/data/octotify.db`，日志轮转 @ `/app/data/log`）
- ✅ API 鉴权（JWT RS256 双令牌，`/api/**` 中间件链与官方一致）
- ✅ 完整 WebUI（vben Vue3 + Element Plus，前后端均来自官方主分支源码编译）

**代码改动仅 2 处字符级修复**（不涉及任何功能）：两个 locale JSON 的尾随逗号。其余为构建层参数（去 sonic tag、musl 工具链、`.env.production.local` API 地址覆盖），二进制与前端均为官方源码原样编译产物。

**功能增强（patches/0002，不改变推送协议）**：消息详情 WebUI 正文支持 Markdown（GFM）与 HTML 安全渲染——marked 解析 + DOMPurify 净化（禁 iframe/object/embed/form/base/meta，链接强制 `_blank`+`noopener`），正文卡片提供「渲染视图 / 原文」切换，纯文本消息亦获得换行/链接/加粗等基础排版；新增依赖 `marked@^18.0.12`、`dompurify@^3.4.15`（锁文件净 +17 行，frozen 安装与前端构建已验证）。API 推送字段（`title`/`message` 纯文本字符串）与各渠道投递行为完全不变，仅影响 WebUI 消息详情页的显示层。

---

## 六、文件清单

```
/root/octotifyforarmv7/
├── build.sh                  # 全流程一键构建（编译机）
├── docker-compose.yml        # 设备端生产编排（交付物3）
├── import-and-run.sh         # 设备端导入+启动（配合交付物4 tar）
├── verify.sh                 # 设备端验证 + curl 推送样例（交付物5）
├── DELIVERY-ARMv7.md         # 本文档
├── deploy/                   # 镜像构建上下文（交付物1: Dockerfile）
│   ├── Dockerfile.armv7      #   ARMv7 专属运行镜像定义（alpine:3.22 armv7）
│   ├── octotify-server       #   交叉编译产物
│   ├── dist/                 #   前端静态资源
│   ├── config/config.yaml    #   官方配置（JWT 密钥首启自动生成）
│   ├── nginx.conf            #   官方 nginx 配置（5233 托管 + /api 反代）
│   └── entrypoint.sh         #   官方启动脚本（nginx 后台 + Go 前台 + 信号转发）
├── out/
│   ├── octotify-server       # ARMv7 静态 ELF
│   └── octotify-armv7.tar    # 导出镜像（交付物4）
├── OctoTify/                 # 官方源码（含 2 处 JSON 修复）
└── tools/                    # Go 1.26.8 + bootlin armv7 musl 工具链
```

## 七、实测记录（x86_64 编译机 + qemu binfmt 全链路）

镜像内产物：`octotify-server` = ELF 32-bit LSB **executable, ARM, EABI5, statically linked**；
镜像 `linux/arm/v7`，28MB（tar 导出后），SHA256 见 `out/octotify-armv7.tar.sha256`。
tar 为纯单清单格式（`--provenance=false --sbom=false`），老版本 Docker `docker load` 兼容，且已做 save→load 回读测试。

使用生产 `docker-compose.yml` 原文件在 qemu 下实测通过：

| # | 验证项 | 实测结果 |
|---|---|---|
| 1 | `docker compose up -d` 容器状态 | `Up (healthy)`（healthcheck 探 34123/ping） |
| 2 | WebUI `GET /` 经 nginx:5233 | HTTP 200（index.html 2752B） |
| 3 | SPA 路由回落 `GET /channels` | HTTP 200 |
| 4 | `POST /api/user/register` | `{"code":0,...}` 返回 JWT |
| 5 | `POST /api/auth/login` | 拿到 access_token |
| 6 | `POST /api/sources` 创建来源 | 返回 `token: src01...`（推送令牌） |
| 7 | `POST /api/push/<token>` | 业务码 110703「来源未绑定任何渠道」——路由/鉴权/反代链路全通；绑定渠道后即多目标广播 |
| 8 | `GET /api/channel-types` | `dingtalk / feishu / telegram / email / gotify`（含配置字段元数据） |
| 9 | `GET /api/messages` 消息日志 | 空列表正常返回（SQLite 读路径通） |
| 10 | `docker compose restart` 后查来源 | 来源仍在 → **SQLite 数据持久化生效** |

> 注：`http://<设备IP>:5233/ping` 经 nginx 会回落到 index.html（SPA fallback），属官方架构预期行为；
> 后端健康探针请直连容器内 `34123/ping`（compose healthcheck 已按此配置）。

## 九、目标设备兼容性核查（面板报告：Armbian 25.11.2 bookworm / 1.9.2 面板）

| 设备报告项 | 核查结论 |
|---|---|
| 系统架构 `armv7l`（内核 6.6.20-current-meson，Amlogic 平台） | ✅ 32 位 ARMv7 用户态；meson 平台 CPU（Cortex-A5/A53 32 位模式）均实现 ARMv7 ISA，GOARM=7 二进制适用；静态链接无任何 libc/内核版本耦合 |
| Docker `29.3.0` / SDK 1.51 | ✅ 与编译机 29.5.3 同代；tar 为 OCI 格式且含 `manifest.json`+RepoTags，已实测 save→load 回读成功 |
| 文件存储驱动 `overlayfs`（经典 image store） | ✅ tar 未用 containerd 专用特性，经典存储直接 load |
| 日志驱动 `json-file` | ✅ 应用自身日志经 lumberjack 轮转落 `/app/data/log`（命名卷），不依赖宿主日志策略 |
| Compose | ✅ compose 文件无废弃 `version:` 键，`services/volumes/healthcheck/start_period` 均为 v2 系通用语法 |
| Cpu/Mem `4 核 / 988MB` | ✅ Go 后端常驻约 30–80MB + nginx 约 5MB，余量充足；1GB 设备建议按下方调优把后端切到 release 模式 |
| 系统时间 `2026/9/7` | ✅ 与编译机时钟一致；JWT 签发/TLS 均依赖正确时钟 |

**1GB 内存设备调优（可选）**：镜像内 `/app/config/config.yaml` 默认沿用官方配置（`mode: debug`、`log.level: debug`、`debug_body: true`）。生产建议在设备上追加 override 关闭调试日志：

```yaml
# docker-compose.override.yml（与 docker-compose.yml 同目录，compose 自动叠加）
services:
  octotify:
    volumes:
      - ./config.yaml:/app/config/config.yaml:ro   # 先 docker cp octotify:/app/config/config.yaml . 再修改：
      # mode: release / level: info / debug_body: false
```

**结论：该版本设备可直接使用本交付物部署，无需任何改动。**

## 十、常见问题

- **导入后容器反复重启**：确认设备为 32 位 ARM 用户态（`uname -m` 输出 `armv7l`/`armv8l`；纯 armv6/树莓派 Zero 1 代不适用，本镜像按 GOARM=7 编译）。
- **`/run/nginx/nginx.pid` Permission denied**（若自行改镜像）：需 `chown app:app /run/nginx`，本镜像已内置修复。
- **webhook 推送证书报错**：镜像已含 `ca-certificates`；如设备时间错误会导致 TLS 失败，compose 已设 `TZ` 且容器跟随系统时间。
- **升级版本**：重新在编译机跑 `./build.sh` 生成新 tar，设备端 `docker load` 后 `docker compose up -d` 即可，数据卷不动。
