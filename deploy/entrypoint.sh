#!/bin/sh
# 启动脚本：后台运行 Nginx，前台运行 Go 后端
nginx -g "daemon off;" &
NGINX_PID=$!

/app/octotify --config /app/config/config.yaml &
APP_PID=$!

# 捕获信号并转发
trap "kill $NGINX_PID $APP_PID; exit" TERM INT

wait
