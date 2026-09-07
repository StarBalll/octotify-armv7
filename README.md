# OctoTify ARMv7 工作区

> **正式名称：OctoTify ARMv7** —— OctoTify 官方项目的 ARMv7 (linux/arm/v7, 32位) 移植构建。
> 不另起品牌名：应用与功能完全归属上游 **OctoTify**，本项目只承担「32位 ARM 交叉编译与镜像交付」。

针对 OctoTify 官方不支持 ARMv7 (32位) 的问题，完成全流程交叉编译 + 自定义 Docker 镜像 + ARM 设备部署。
**详细文档：[DELIVERY-ARMv7.md](./DELIVERY-ARMv7.md)** · 设备端速查：[device-package/DEPLOY-CARD.md](./device-package/DEPLOY-CARD.md)

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
| 版本号 | `vMAJOR.MINOR.PATCH`（tag: `v1.0.0-armv7`） | SemVer；基线见 [VERSION](./VERSION) |
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

## 上游源码修改（4 处，均为 armv7/局域网 HTTP 场景必需）

| # | 文件 | 问题 |
|---|---|---|
| 1 | locale `page.json` ×2 | 尾随逗号导致 vite8/rolldown 构建失败 |
| 2 | `source/detail.vue` | 复制按钮依赖 clipboard API（仅 HTTPS），局域网 HTTP 必失败 |
| 3 | `source/detail.vue` | 查看令牌经密码二次验证后仍强制脱敏显示 |
| 4 | `source/create.vue` | 创建来源时丢弃后端返回的推送令牌（仅此一次返回） |

构建参数关键差异（详见交付文档）：去 `-tags=sonic`（不支持32位）、musl 交叉工具链、`VITE_GLOB_API_URL=/`。
