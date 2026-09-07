# OctoTify ARMv7 设备端部署速查

目标环境：Armbian bookworm armv7l / Docker 29.3.0（已核查兼容，详见完整文档 DELIVERY-ARMv7.md 第九节）

## 本目录应包含 4 个文件

```
octotify-armv7.tar     镜像包（docker save 导出）
docker-compose.yml     生产编排（含健康检查、数据持久化、开机自启）
import-and-run.sh      一键导入+启动
verify.sh              部署验证 + curl 推送样例
```

## 部署三步

```bash
chmod +x import-and-run.sh verify.sh
./import-and-run.sh            # 导入镜像 + docker compose up -d + 等待健康检查
./verify.sh octoadmin 'Passw0rd123'   # 注册管理员并演示推送（可选参数自定义账号密码）
```

浏览器打开 http://<设备IP>:5233 即官方完整 WebUI。

## 手动逐步执行（等价于 import-and-run.sh）

```bash
docker load -i octotify-armv7.tar
docker compose up -d
docker ps --filter name=octotify                 # 状态应为 Up (healthy)
docker exec octotify wget -qO- http://127.0.0.1:34123/ping
```

## 部署后系统里的数据落点

```
设备 /var/lib/docker/volumes/octotify_octotify-data/_data/   ← 命名卷（宿主机视角）
├── octotify.db            SQLite 数据库（来源/渠道/消息日志/用户）
└── log/
    ├── octotify.log       运行日志（lumberjack 自动轮转）
    └── error.log          错误日志

容器内路径（删容器不丢，跟着卷走）
/app/data/                ← 即上述命名卷
/app/config/config.yaml   官方默认配置（JWT 密钥首启自动生成于 /app/config/keys/）
/usr/share/nginx/html/    前端静态文件
/etc/nginx/http.d/default.conf   nginx 配置（5233 托管前端 + /api/ 反代 34123）
```

## 1GB 内存设备可选调优（关掉官方默认的 debug 全量日志）

```bash
docker cp octotify:/app/config/config.yaml ./config.yaml
# 编辑 ./config.yaml：mode: debug→release，level: debug→info，debug_body: true→false
cat > docker-compose.override.yml <<'EOF'
services:
  octotify:
    volumes:
      - octotify-data:/app/data
      - ./config.yaml:/app/config/config.yaml:ro
EOF
docker compose up -d
```

## 常用运维

```bash
docker compose logs -f octotify     # 看日志（控制台流）
docker compose restart              # 重启（数据不丢）
docker compose down                 # 停止并删容器（命名卷保留，数据不丢）
docker compose down -v              # ⚠️ 连数据一起清除
```
