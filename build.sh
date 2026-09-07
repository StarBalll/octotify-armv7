#!/usr/bin/env bash
# ==============================================================================
# OctoTify ARMv7 (linux/arm/v7) 全流程交叉编译 + 镜像构建脚本
# 在 x86_64 编译机上运行；产出 deploy/ 构建上下文与 octotify-armv7.tar
#
# 用法:  ./build.sh          # 完整流程
#        ./build.sh --no-clone  # 复用已有源码（跳过 clone，便于增量）
# ==============================================================================
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$ROOT/OctoTify"
TOOLS="$ROOT/tools"
OUT="$ROOT/out"
DEPLOY="$ROOT/deploy"
GO_VERSION="1.26.8"
TC_VER="2024.05-1"
TC_DIR="armv7-eabihf--musl--stable-${TC_VER}"

# 版本基线（见 VERSION）：镜像打 latest + v<port> 双标签；tar 以版本号命名，另留稳定名副本
PORT_VERSION="$(sed -n 's/^PORT_VERSION=//p' "$ROOT/VERSION" | tr -d '[:space:]')"
UPSTREAM_BASE="$(sed -n 's/^UPSTREAM_BASE=//p' "$ROOT/VERSION" | tr -d '[:space:]')"
[[ -n "$PORT_VERSION" ]] || { echo "VERSION 文件缺少 PORT_VERSION"; exit 1; }

CLONE=1
[[ "${1:-}" == "--no-clone" ]] && CLONE=0

log() { echo -e "\n\033[1;36m==> $*\033[0m"; }

# ------------------------------------------------------------------------------
log "[0/7] 环境预检"
# ------------------------------------------------------------------------------
for t in git docker curl tar file; do
  command -v "$t" >/dev/null || { echo "缺少 $t，请先安装"; exit 1; }
done
docker buildx version >/dev/null 2>&1 || { echo "缺少 docker buildx 插件"; exit 1; }
docker info >/dev/null 2>&1 || { echo "Docker 守护进程未运行"; exit 1; }
mkdir -p "$TOOLS" "$OUT" "$DEPLOY"

# ------------------------------------------------------------------------------
log "[1/7] Go ${GO_VERSION} 工具链（go.mod 要求 >=1.26.2）"
# ------------------------------------------------------------------------------
if [[ ! -x "$TOOLS/go/bin/go" ]]; then
  curl -fL -o "$TOOLS/go.tgz" "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz"
  tar -xzf "$TOOLS/go.tgz" -C "$TOOLS"
fi
export PATH="$TOOLS/go/bin:$PATH"
export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"
export GOTOOLCHAIN=local
go version

# ------------------------------------------------------------------------------
log "[2/7] ARMv7 硬浮点交叉工具链（musl，产出免 glibc 依赖的静态二进制）"
# ------------------------------------------------------------------------------
CC_NAME="arm-buildroot-linux-musleabihf-gcc"
if [[ ! -x "$TOOLS/$TC_DIR/bin/$CC_NAME" ]]; then
  curl -fL -o "$TOOLS/armv7-musl.tar.xz" \
    "https://toolchains.bootlin.com/downloads/releases/toolchains/armv7-eabihf/tarballs/${TC_DIR}.tar.xz"
  tar -xJf "$TOOLS/armv7-musl.tar.xz" -C "$TOOLS"
fi
export PATH="$TOOLS/$TC_DIR/bin:$PATH"
"$CC_NAME" --version | head -1
# 备选：apt 安装 glibc 工具链（arm-linux-gnueabihf-gcc），静态链接下同样可用；
# musl 工具链优先，因其自带完整 sysroot，静态链接成功率最高。

# ------------------------------------------------------------------------------
log "[3/7] 拉取官方源码（锚定 VERSION 的 UPSTREAM_BASE，保证可复现构建）"
# ------------------------------------------------------------------------------
UPSTREAM_REF="${UPSTREAM_REF:-$UPSTREAM_BASE}"
if [[ $CLONE -eq 1 ]]; then
  if [[ -d "$REPO/.git" ]]; then
    git -C "$REPO" fetch origin --tags
  else
    git clone https://github.com/loommii/OctoTify.git "$REPO"
  fi
  # 本地已有补丁分支也强制回到基线，由后续幂等补丁重放修复
  git -C "$REPO" checkout -qf "$UPSTREAM_REF" 2>/dev/null || {
    git -C "$REPO" fetch --unshallow origin 2>/dev/null || git -C "$REPO" fetch origin
    git -C "$REPO" checkout -qf "$UPSTREAM_REF"
  }
fi
echo "上游基线: $(git -C "$REPO" rev-parse --short HEAD) ($(git -C "$REPO" log -1 --format=%cs HEAD))"

# ------------------------------------------------------------------------------
log "[4/7] 前端打包（Vue3 + Element Plus，输出 frontend/apps/web-ele/dist）"
# ------------------------------------------------------------------------------
# 关键点 1：生产环境 API 地址必须为根路径 "/"（前端代码里已写全 /api/... 前缀，
#           经 nginx 同源反代）。若误设为 /api 会拼成 /api/api/** 导致 404「资源不存在」。
printf 'VITE_GLOB_API_URL=/\n' > "$REPO/frontend/apps/web-ele/.env.production.local"
# 关键点 2：上游 locale JSON 存在尾随逗号（rolldown/vite8 严格解析报错），幂等修复：
for f in "$REPO/frontend/apps/web-ele/src/locales/langs/zh-CN/page.json" \
         "$REPO/frontend/apps/web-ele/src/locales/langs/en-US/page.json"; do
  perl -0777 -pi -e 's/("email": "(?:邮件|Email)"),(\s*\})/$1$2/' "$f"
done
# 关键点 2c：来源详情页"查看令牌"通过密码二次验证后仍显示脱敏星号（showTokenPlainText
#           被置 false），用户看不到完整 token——改为验证后直接显示明文（幂等）。
python3 - "$REPO/frontend/apps/web-ele/src/views/source/detail.vue" <<'PYEOF2'
import sys, pathlib
p = pathlib.Path(sys.argv[1])
s = p.read_text()
changed = False
for anchor, repl in [
    ("    tokenValue.value = res.token;\n    showTokenPlainText.value = false;\n    ElMessage.success($t('page.source.viewTokenSuccess'));",
     "    tokenValue.value = res.token;\n    showTokenPlainText.value = true;\n    ElMessage.success($t('page.source.viewTokenSuccess'));"),
    ("    tokenValue.value = res.token;\n    showTokenPlainText.value = false;\n    ElMessage.success($t('page.source.resetTokenSuccess'));",
     "    tokenValue.value = res.token;\n    showTokenPlainText.value = true;\n    ElMessage.success($t('page.source.resetTokenSuccess'));"),
]:
    if repl not in s:
        assert anchor in s, f'plaintext patch anchor not found: {anchor[:60]}'
        s = s.replace(anchor, repl)
        changed = True
p.write_text(s)
print('plaintext-after-stepup patched' if changed else 'plaintext patch already present')
PYEOF2
# 关键点 2d：新建来源时前端丢弃了后端返回的推送令牌（仅创建时返回一次），
#           导致用户从 UI 拿不到初始 token——改为创建成功页展示令牌（明文+复制，幂等）。
python3 - "$REPO/frontend/apps/web-ele/src/views/source/create.vue" <<'PYEOF3'
import sys, pathlib
p = pathlib.Path(sys.argv[1])
s = p.read_text()
if 'createdToken' in s:
    print('create-token display already present')
else:
    # 1) 表单加 v-if，成功后切换为令牌展示区
    old_form = '<ElForm\n        ref="formRef"'
    new_form = '<ElForm\n        v-if="!createdToken"\n        ref="formRef"'
    assert old_form in s, 'create form anchor not found'
    s = s.replace(old_form, new_form)
    # 2) 表单结束后插入令牌展示区
    old_tail = """        </ElFormItem>
      </ElForm>
    </ElCard>"""
    new_tail = """        </ElFormItem>
      </ElForm>

      <!-- 创建成功：展示推送令牌（后端仅创建时返回一次，必须立即展示给用户保存） -->
      <template v-else>
        <ElAlert
          type="success"
          :closable="false"
          show-icon
          title="来源创建成功"
          description="下方为该来源的推送令牌，仅此一次显示，请立即复制保存。之后可在来源详情页查看。"
          class="mb-4 max-w-2xl"
        />
        <ElAlert
          type="warning"
          :closable="false"
          show-icon
          title="令牌等同于推送凭据，请勿提交到公开仓库或明文日志"
          class="mb-4 max-w-2xl"
        />
        <ElInput :model-value="createdToken" readonly size="large" class="max-w-2xl">
          <template #append>
            <ElButton @click="handleCopyCreatedToken">
              <IconifyIcon icon="mdi:content-copy" />
            </ElButton>
          </template>
        </ElInput>
        <div class="mt-4">
          <ElButton type="primary" @click="router.push('/source/list')">
            我已保存令牌，返回列表
          </ElButton>
        </div>
      </template>
    </ElCard>"""
    assert old_tail in s, 'create form tail anchor not found'
    s = s.replace(old_tail, new_tail)
    # 3) 引入 ElAlert 组件
    old_imp = "import {\n  ElCard,\n  ElForm,"
    new_imp = "import {\n  ElAlert,\n  ElCard,\n  ElForm,"
    assert old_imp in s, 'create import anchor not found'
    s = s.replace(old_imp, new_imp)
    # 4) 状态与提交逻辑：保存返回的 token + 复制函数
    old_state = "const channels = ref<ChannelApi.ChannelDTO[]>([]);"
    new_state = ("const channels = ref<ChannelApi.ChannelDTO[]>([]);\n"
                 "// 创建成功后返回的推送令牌（仅创建时返回一次）\n"
                 "const createdToken = ref('');")
    assert old_state in s, 'create state anchor not found'
    s = s.replace(old_state, new_state)
    old_submit = """    await createSourceApi(data);
    ElMessage.success($t('page.source.createSuccess'));
    router.push('/source/list');"""
    new_submit = """    const res = await createSourceApi(data);
    createdToken.value = res?.token ?? '';
    ElMessage.success($t('page.source.createSuccess'));
    if (!createdToken.value) {
      // 异常兜底：后端未返回令牌时保持旧行为返回列表
      router.push('/source/list');
    }"""
    assert old_submit in s, 'create submit anchor not found'
    s = s.replace(old_submit, new_submit)
    # 5) 追加复制函数（放在 handleBack 前）
    old_back = "function handleBack() {"
    new_copy = """// 复制新建来源的令牌（HTTP 环境降级：clipboard API 需安全上下文）
async function handleCopyCreatedToken() {
  if (!createdToken.value) return;
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(createdToken.value);
    } else {
      const textarea = document.createElement('textarea');
      textarea.value = createdToken.value;
      textarea.style.position = 'fixed';
      textarea.style.opacity = '0';
      document.body.appendChild(textarea);
      textarea.select();
      const ok = document.execCommand('copy');
      document.body.removeChild(textarea);
      if (!ok) throw new Error('execCommand copy failed');
    }
    ElMessage.success('令牌已复制到剪贴板');
  } catch {
    ElMessage.error('复制失败，请手动选择复制');
  }
}

function handleBack() {"""
    assert old_back in s, 'create handleBack anchor not found'
    s = s.replace(old_back, new_copy)
    p.write_text(s)
    print('create-token display patched')
PYEOF3
# 关键点 2b：来源详情页复制按钮依赖 navigator.clipboard（仅 HTTPS 可用），
# 局域网 HTTP 部署必然报"复制失败"——注入 execCommand 降级方案（幂等，已含则跳过）。
python3 - "$REPO/frontend/apps/web-ele/src/views/source/detail.vue" <<'PYEOF'
import sys, pathlib
p = pathlib.Path(sys.argv[1])
s = p.read_text()
if 'execCommand' not in s:
    old = "    await navigator.clipboard.writeText(tokenValue.value);"
    new = """    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(tokenValue.value);
    } else {
      // HTTP fallback: clipboard API requires secure context; LAN HTTP deploy
      const textarea = document.createElement('textarea');
      textarea.value = tokenValue.value;
      textarea.style.position = 'fixed';
      textarea.style.opacity = '0';
      document.body.appendChild(textarea);
      textarea.select();
      const ok = document.execCommand('copy');
      document.body.removeChild(textarea);
      if (!ok) throw new Error('execCommand copy failed');
    }"""
    assert old in s, 'clipboard patch anchor not found'
    p.write_text(s.replace(old, new))
    print('clipboard fallback patched')
else:
    print('clipboard fallback already present')
PYEOF
cd "$REPO/frontend"
command -v pnpm >/dev/null || npm install -g pnpm@10
pnpm install --frozen-lockfile
pnpm run build --filter=@vben/web-ele
[[ -f apps/web-ele/dist/index.html ]] || { echo "前端 dist 未生成"; exit 1; }

# ------------------------------------------------------------------------------
log "[5/7] Go 后端交叉编译（CGO_ENABLED=1 GOOS=linux GOARCH=arm GOARM=7）"
# ------------------------------------------------------------------------------
# 关键点 3：禁止 -tags=sonic —— sonic 不支持 32 位架构，armv7 编译期硬报错；
#           省略该 tag 时 JSON 走标准库回退路径，功能等价。
# 关键点 4：CGO 必须开启（gorm sqlite 驱动 = mattn/go-sqlite3 需要 C 编译器）。
# 关键点 5：-extldflags -static 产出全静态二进制，容器内零 libc 依赖。
cd "$REPO/backend"
go mod download
CGO_ENABLED=1 GOOS=linux GOARCH=arm GOARM=7 CC="$CC_NAME" \
  go build -ldflags='-s -w -extldflags -static' -o "$OUT/octotify-server" ./cmd/server
file "$OUT/octotify-server"
grep -q "ARM, EABI5" <(file "$OUT/octotify-server") || { echo "产物不是 ARM ELF！"; exit 1; }

# ------------------------------------------------------------------------------
log "[6/7] 组装镜像构建上下文（官方架构复刻：nginx 托管前端 + /api 反代后端）"
# ------------------------------------------------------------------------------
cp "$OUT/octotify-server"                          "$DEPLOY/octotify-server"
cp "$REPO/docker/entrypoint.sh"                    "$DEPLOY/entrypoint.sh"
cp "$REPO/docker/nginx.conf"                       "$DEPLOY/nginx.conf"
mkdir -p "$DEPLOY/config" "$DEPLOY/dist"
cp "$REPO/backend/config/config.yaml"              "$DEPLOY/config/config.yaml"
rm -rf "$DEPLOY/dist" && mkdir -p "$DEPLOY/dist"
cp -r "$REPO/frontend/apps/web-ele/dist/."         "$DEPLOY/dist/"
chmod +x "$DEPLOY/entrypoint.sh" "$DEPLOY/octotify-server"

# ------------------------------------------------------------------------------
log "[7/7] buildx 构建 linux/arm/v7 镜像并导出 tar"
# ------------------------------------------------------------------------------
cd "$DEPLOY"
# --provenance/--sbom 关闭：避免 attestation manifest list，老版本 Docker 的 ARM32 设备
# docker load 兼容性更好
docker buildx build --platform linux/arm/v7 --provenance=false --sbom=false \
  -f Dockerfile.armv7 -t octotify-armv7:latest -t "octotify-armv7:v${PORT_VERSION}" .
docker save octotify-armv7:latest -o "$OUT/octotify-armv7-v${PORT_VERSION}.tar"
cp "$OUT/octotify-armv7-v${PORT_VERSION}.tar" "$OUT/octotify-armv7.tar"   # 稳定名副本(设备流程引用)
sha256sum "$OUT/octotify-armv7-v${PORT_VERSION}.tar" | tee "$OUT/octotify-armv7-v${PORT_VERSION}.tar.sha256"
docker image inspect octotify-armv7:latest --format '镜像架构: {{.Os}}/{{.Architecture}}/{{.Variant}}  大小: {{.Size}}'
echo
echo "✅ 完成（OctoTify ARMv7 v${PORT_VERSION}，上游基线 ${UPSTREAM_BASE}）。交付物："
echo "   - $OUT/octotify-armv7-v${PORT_VERSION}.tar   （镜像归档，版本化命名）"
echo "   - $OUT/octotify-armv7.tar                    （稳定名副本，设备流程引用）"
echo "   - $ROOT/docker-compose.yml  （设备端部署文件）"
echo "   - $ROOT/import-and-run.sh   （设备端导入+启动）"
echo "   - $ROOT/verify.sh           （部署验证 + curl 推送样例）"
