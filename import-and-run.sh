#!/usr/bin/env bash
# 在 ARMv7 设备上：导入镜像并启动服务
# 用法: 把 octotify-armv7.tar、docker-compose.yml、本脚本放同一目录，然后 ./import-and-run.sh
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TAR="${1:-$DIR/octotify-armv7.tar}"

echo "==> 设备架构检查（预期 armv7/armv8l 32位用户态）"
uname -m
docker version --format '{{.Server.Os}}/{{.Server.Arch}}'

echo "==> 导入镜像"
docker load -i "$TAR"

echo "==> 启动服务"
cd "$DIR" && docker compose up -d

echo "==> 等待健康检查（约 30s）"
sleep 30
docker ps --filter name=octotify
curl -s --max-time 5 http://127.0.0.1:5233/ping && echo && echo "✅ 服务已就绪: http://<设备IP>:5233"
