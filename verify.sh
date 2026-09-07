#!/usr/bin/env bash
# OctoTify ARMv7 部署验证 + curl 推送样例（在 ARM 设备上执行）
# 用法: ./verify.sh [注册用户名] [注册密码]
#   用户名: 3-64 字符，仅字母/数字/下划线；密码: 8-64 字符，需含大小写字母和数字
set -euo pipefail

USER_NAME="${1:-octoadmin}"
USER_PASS="${2:-Passw0rd123}"
BASE="http://127.0.0.1:5233"

echo "==> [1/6] 容器与架构"
docker ps --filter name=octotify --format '{{.Names}}  {{.Status}}  {{.Image}}'
docker image inspect octotify-armv7:latest --format '镜像: {{.Os}}/{{.Architecture}}/{{.Variant}}'

echo "==> [2/6] 后端健康（容器内直连 34123）"
docker exec octotify wget -qO- http://127.0.0.1:34123/ping; echo

echo "==> [3/6] 前端 UI（nginx 5233 → index.html）"
curl -sf -o /dev/null -w "HTTP %{http_code}, %{size_download} bytes\n" "$BASE/"

echo "==> [4/6] 注册 + 登录"
curl -sf -X POST "$BASE/api/user/register" -H 'Content-Type: application/json' \
  -d "{\"username\":\"$USER_NAME\",\"password\":\"$USER_PASS\"}" > /dev/null || echo "(用户已存在，跳过注册)"
TOKEN=$(curl -sf -X POST "$BASE/api/auth/login" -H 'Content-Type: application/json' \
  -d "{\"username\":\"$USER_NAME\",\"password\":\"$USER_PASS\"}" | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')
[[ -n "$TOKEN" ]] || { echo "登录失败"; exit 1; }
echo "登录成功, access_token: ${TOKEN:0:24}..."

echo "==> [5/6] 创建消息来源 Source（返回推送 Token，仅创建时可见）"
SRC=$(curl -sf -X POST "$BASE/api/sources" -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d '{"name":"curl-demo","channel_ids":[]}')
echo "$SRC"
PUSH_TOKEN=$(echo "$SRC" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')

echo "==> [6/6] 广播推送样例（POST /api/push/:token）"
if [[ -n "${PUSH_TOKEN:-}" ]]; then
  curl -sf -X POST "$BASE/api/push/$PUSH_TOKEN" -H 'Content-Type: application/json' \
    -d '{"title":"ARMv7 部署验证","message":"OctoTify 已在 linux/arm/v7 上成功运行"}'; echo
else
  echo "(未获取到推送 Token，请登录 WebUI 查看)"
fi

cat <<'NOTE'

后续在 WebUI (http://<设备IP>:5233) 中：
  渠道管理 → 新建渠道（type: wechat | telegram | dingtalk | email | webhook，字段见 GET /api/channel-types）
  来源管理 → 绑定渠道 → 广播推送：POST /api/push/<token>  {"title":"...","message":"..."}
  多目标广播与单渠道故障隔离由服务端自动完成（响应含各渠道 results）。
NOTE
