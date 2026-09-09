<template>
  <div class="p-4">
    <ElCard v-loading="loading" shadow="never">
      <template #header>
        <div class="flex items-center justify-between">
          <span class="font-semibold">{{ $t('page.message.detail') }}</span>
          <ElButton type="primary" @click="handleBack">
            <IconifyIcon icon="mdi:arrow-left" class="mr-1" />
            {{ $t('common.back') }}
          </ElButton>
        </div>
      </template>

      <template v-if="messageDetail">
        <!-- 消息基本信息 -->
        <ElDescriptions :column="2" border class="mb-6">
          <ElDescriptionsItem :label="$t('page.message.messageTitle')" :span="2">
            {{ messageDetail.title }}
          </ElDescriptionsItem>
          <ElDescriptionsItem :label="$t('page.message.pushStatus')">
            <ElTag :type="getStatusTagType(messageDetail.status)" size="small">
              {{ getStatusLabel(messageDetail.status) }}
            </ElTag>
          </ElDescriptionsItem>
          <ElDescriptionsItem :label="$t('page.message.createTime')">
            {{ formatTimestamp(messageDetail.created_at_ts) }}
          </ElDescriptionsItem>
          <ElDescriptionsItem :label="$t('page.message.updateTime')">
            {{ formatTimestamp(messageDetail.updated_at_ts) }}
          </ElDescriptionsItem>
        </ElDescriptions>

        <!-- 消息内容：自动识别 Markdown / HTML 富文本并安全渲染，可随时切换回原文 -->
        <ElCard shadow="never" class="mb-6">
          <template #header>
            <div class="flex items-center justify-between">
              <span class="font-semibold">{{ $t('page.message.messageContent') }}</span>
              <ElRadioGroup v-if="hasContent" v-model="viewMode" size="small">
                <ElRadioButton value="rendered">
                  {{ $t('page.message.viewRendered') }}
                </ElRadioButton>
                <ElRadioButton value="source">
                  {{ $t('page.message.viewSource') }}
                </ElRadioButton>
              </ElRadioGroup>
            </div>
          </template>
          <!-- 原文视图：保留推送时的原始文本 -->
          <div
            v-if="viewMode === 'source'"
            class="whitespace-pre-wrap break-words leading-relaxed"
          >
            {{ messageDetail.content || '--' }}
          </div>
          <!-- 渲染视图：Markdown / HTML 经 DOMPurify 净化后展示。
               超长文本走分块渐进渲染（每块间让出主线程），渲染期间显示进度条 -->
          <template v-else>
            <div
              v-if="renderStage === 'rendering'"
              class="mb-2 flex items-center gap-3"
            >
              <span class="text-xs whitespace-nowrap opacity-60">
                {{ $t('page.message.rendering') }}
              </span>
              <ElProgress
                :percentage="renderProgress"
                :stroke-width="6"
                class="flex-1"
              />
            </div>
            <div
              v-show="hasContent"
              ref="mdBox"
              class="message-rich-body"
            ></div>
            <div v-if="!hasContent" class="leading-relaxed opacity-50">--</div>
          </template>
        </ElCard>

        <!-- 来源信息 -->
        <ElCard :title="$t('page.message.sourceInfo')" shadow="never" class="mb-6">
          <ElDescriptions :column="2" border>
            <ElDescriptionsItem :label="$t('page.message.source')">
              {{ messageDetail.source_name || '--' }}
            </ElDescriptionsItem>
            <ElDescriptionsItem label="ID">
              {{ messageDetail.source_id }}
            </ElDescriptionsItem>
          </ElDescriptions>
        </ElCard>

        <!-- 渠道信息 -->
        <ElCard :title="$t('page.message.channelInfo')" shadow="never">
          <ElDescriptions :column="2" border>
            <ElDescriptionsItem :label="$t('page.message.channel')">
              {{ messageDetail.channel_name || '--' }}
            </ElDescriptionsItem>
            <ElDescriptionsItem label="ID">
              {{ messageDetail.channel_id }}
            </ElDescriptionsItem>
            <ElDescriptionsItem :label="$t('page.channel.type')" v-if="messageDetail.channel_type">
              <ElTag size="small">{{ getChannelTypeLabel(messageDetail.channel_type) }}</ElTag>
            </ElDescriptionsItem>
          </ElDescriptions>
        </ElCard>
      </template>
    </ElCard>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, onBeforeUnmount, watch, nextTick } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { marked } from 'marked';
import DOMPurify from 'dompurify';
import {
  ElCard,
  ElButton,
  ElDescriptions,
  ElDescriptionsItem,
  ElTag,
  ElMessage,
  ElRadioGroup,
  ElRadioButton,
  ElProgress,
} from 'element-plus';
import { IconifyIcon } from '@vben/icons';
import { $t } from '#/locales';
import {
  getMessageDetailApi,
  type MessageApi,
} from '#/api/modules/message';
import { getChannelTypesApi } from '#/api/modules/channel';
import { formatTimestamp } from '#/utils/time';

const route = useRoute();
const router = useRouter();

const loading = ref(false);
const messageDetail = ref<MessageApi.MessageDTO | null>(null);
const channelTypeMeta = ref<Record<string, string>>({});
const viewMode = ref<'rendered' | 'source'>('rendered');

// 富文本链接统一新窗口打开并解除 opener 关联（DOMPurify 官方推荐做法，模块级仅安装一次）
let linkHookInstalled = false;
function installLinkHook() {
  if (linkHookInstalled) return;
  linkHookInstalled = true;
  DOMPurify.addHook('afterSanitizeAttributes', (node) => {
    if (node.tagName === 'A') {
      node.setAttribute('target', '_blank');
      node.setAttribute('rel', 'noopener noreferrer nofollow');
    }
  });
}
installLinkHook();

// HTML 内容识别：以块级/常见内联标签开头才按 HTML 渲染，避免把 "< 3" 之类普通文本误判
function looksLikeHtml(content: string): boolean {
  const text = content.trim();
  if (!text.startsWith('<')) return false;
  return /<\/?(?:html|body|div|p|br|h[1-6]|ul|ol|li|table|thead|tbody|tr|td|th|blockquote|pre|section|article|header|footer|span|strong|em|b|i|u|s|a|img|font|center|code|hr)\b/i.test(
    text.slice(0, 512),
  );
}

const hasContent = computed(() => Boolean(messageDetail.value?.content?.trim()));

// ---------------------------------------------------------------------------
// 超长文本渐进渲染：
// marked 解析本身很快（200K 约 50ms），真正卡死页面的是
//   1) DOMPurify 对数百 KB 输出 HTML 的整棵 DOM 净化
//   2) 浏览器一次性 innerHTML 数万节点的解析与布局
// 三者同步串在一次主线程任务里会导致 UI 冻结数秒（移动端尤甚）。
// 方案：按空行分块（围栏感知，不切断代码块/表格），逐块「解析→净化→追加 DOM」，
// 每块间 setTimeout(0) 让出主线程；小文本（< CHUNK_THRESHOLD）保持一次性
// 同步路径。每次渲染可取消（route 变化/内容变化/组件卸载）。
// ---------------------------------------------------------------------------
const mdBox = ref<HTMLElement | null>(null);
const renderStage = ref<'idle' | 'rendering' | 'done'>('idle');
const renderProgress = ref(0);
let renderToken = 0; // 渲染代际令牌：内容/模式变化或卸载时自增以取消旧任务
let renderCache: { key: string; html: string } | null = null; // 视图来回切换时免重渲染

const CHUNK_THRESHOLD = 48 * 1024; // 超过 48K 才走分块渐进路径
const CHUNK_TARGET = 16 * 1024; // 单块目标大小
const CHUNK_HARD_LIMIT = 64 * 1024; // 单块硬上限（超长单行/巨型段落兜底强制切块）

/** 采样哈希：仅用于同页面实例内的渲染结果缓存键 */
function hashKey(s: string): string {
  let h = 5381;
  const step = Math.max(1, Math.floor(s.length / 512));
  for (let i = 0; i < s.length; i += step) {
    h = ((h * 33) ^ s.charCodeAt(i)) >>> 0;
  }
  return `${s.length}:${h.toString(36)}`;
}

/** 将 Markdown 源文按空行切成围栏感知的块（不切断 ```/~~~ 代码块与表格） */
function splitMarkdownChunks(content: string): string[] {
  const lines = content.split('\n');
  const chunks: string[] = [];
  let cur: string[] = [];
  let curLen = 0;
  let inFence = false;
  let fenceMark = '';
  for (const line of lines) {
    const m = /^(```|~~~)/.exec(line.trim());
    if (m) {
      if (!inFence) {
        inFence = true;
        fenceMark = m[1]![0]!;
      } else if (m[1]![0] === fenceMark) {
        inFence = false;
      }
    }
    cur.push(line);
    curLen += line.length + 1;
    // 在围栏外的空行处按目标大小切分；超过硬上限无条件切（防超长单行）
    if (!inFence && ((curLen >= CHUNK_TARGET && line.trim() === '') || curLen >= CHUNK_HARD_LIMIT)) {
      chunks.push(cur.join('\n'));
      cur = [];
      curLen = 0;
    }
  }
  if (cur.length) chunks.push(cur.join('\n'));
  return chunks;
}

function sanitizeChunk(rawHtml: string): string {
  return DOMPurify.sanitize(rawHtml, {
    FORBID_TAGS: ['iframe', 'object', 'embed', 'form', 'base', 'meta'],
  });
}

const nextFrame = (): Promise<void> =>
  new Promise((resolve) => setTimeout(resolve, 0));

/** 取消进行中的渲染任务 */
function cancelRender() {
  renderToken += 1;
  renderStage.value = 'idle';
  renderProgress.value = 0;
}

/** 分块渐进渲染（Markdown 长文）：逐块解析→净化→追加 DOM，每块间让出主线程，内容逐步流出。
 *  HTML 长文不做文本切分（切分会破坏标签配平风险），改为「先让出主线程显示进度条，
 *  再单次净化」，至少保证进度条先渲染、净化不与首帧挤在同一任务里。 */
async function renderProgressive(content: string, isHtml: boolean, token: number) {
  const box = mdBox.value;
  if (!box) return;
  renderStage.value = 'rendering';
  renderProgress.value = 0;
  box.innerHTML = '';
  if (isHtml) {
    await nextFrame(); // 让进度条先绘制
    if (token !== renderToken) return;
    box.innerHTML = sanitizeChunk(content);
    if (token !== renderToken) return;
    renderProgress.value = 100;
    renderCache = { key: hashKey(content), html: box.innerHTML };
    renderStage.value = 'done';
    return;
  }
  const parts = splitMarkdownChunks(content);
  const total = parts.length;
  for (let i = 0; i < total; i++) {
    if (token !== renderToken) return; // 已被新任务/取消/卸载取代
    let rawHtml = '';
    try {
      rawHtml = marked.parse(parts[i]!, { async: false, gfm: true, breaks: true });
    } catch {
      rawHtml = '';
    }
    if (token !== renderToken) return;
    box.insertAdjacentHTML('beforeend', sanitizeChunk(rawHtml));
    renderProgress.value = Math.round(((i + 1) / total) * 100);
    if (i < total - 1) await nextFrame();
  }
  if (token !== renderToken) return;
  renderCache = { key: hashKey(content), html: box.innerHTML };
  renderStage.value = 'done';
}

/** 一次性同步渲染（小文本路径） */
function renderImmediate(content: string, isHtml: boolean, token: number) {
  const box = mdBox.value;
  if (!box) return;
  let rawHtml = '';
  try {
    rawHtml = isHtml
      ? content
      : marked.parse(content, { async: false, gfm: true, breaks: true });
  } catch {
    rawHtml = '';
  }
  if (token !== renderToken) return;
  box.innerHTML = sanitizeChunk(rawHtml);
  renderCache = { key: hashKey(content), html: box.innerHTML };
  renderStage.value = 'done';
}

/** 根据当前内容与视图模式调度渲染（带缓存：视图来回切换瞬时完成） */
async function scheduleRender() {
  if (viewMode.value !== 'rendered') return;
  const content = messageDetail.value?.content ?? '';
  cancelRender();
  const token = renderToken;
  if (!content.trim()) {
    if (mdBox.value) mdBox.value.innerHTML = '';
    return;
  }
  await nextTick(); // 确保 mdBox 已挂载
  const box = mdBox.value;
  if (!box || token !== renderToken) return;
  const key = hashKey(content);
  if (renderCache && renderCache.key === key) {
    // 缓存命中：一次性回填，不再解析/净化
    box.innerHTML = renderCache.html;
    renderStage.value = 'done';
    return;
  }
  if (content.length <= CHUNK_THRESHOLD) {
    renderImmediate(content, looksLikeHtml(content), token);
  } else {
    renderProgressive(content, looksLikeHtml(content), token).catch(() => {});
  }
}

watch([messageDetail, viewMode], () => {
  scheduleRender();
});

onBeforeUnmount(() => {
  cancelRender();
});

// 加载详情
async function loadDetail() {
  const id = Number(route.params.id);
  if (!id) {
    ElMessage.error($t('page.message.invalidId'));
    return;
  }

  loading.value = true;
  try {
    const res = await getMessageDetailApi(id);
    messageDetail.value = res;
    viewMode.value = 'rendered';
  } catch {
    ElMessage.error($t('page.message.loadDetailFailed'));
  } finally {
    loading.value = false;
  }
}

// 渲染调度由 watch(messageDetail/viewMode) 驱动（见 scheduleRender）

// 加载渠道类型元数据
async function loadTypeMeta() {
  try {
    const types = await getChannelTypesApi();
    types.forEach((t) => {
      channelTypeMeta.value[t.type] = t.name;
    });
  } catch {
    // 不影响主流程
  }
}

// 状态映射
const statusMap: Record<number, { label: string; type: 'info' | 'success' | 'danger' }> = {
  100: { label: $t('page.message.statusPending'), type: 'info' },
  200: { label: $t('page.message.statusSuccess'), type: 'success' },
  300: { label: $t('page.message.statusFailed'), type: 'danger' },
};

function getStatusLabel(status: number): string {
  return statusMap[status]?.label ?? String(status);
}

function getStatusTagType(status: number): 'info' | 'success' | 'danger' {
  return statusMap[status]?.type ?? 'info';
}

// 渠道类型标签
function getChannelTypeLabel(type: string): string {
  return channelTypeMeta.value[type] || type;
}

function handleBack() {
  router.push('/message/list');
}

onMounted(() => {
  Promise.all([loadDetail(), loadTypeMeta()]);
});
</script>

<style scoped>
/* 渲染视图排版：中性半透明配色，亮色/暗色主题下均可读 */
.message-rich-body {
  color: inherit;
  line-height: 1.75;
  overflow-wrap: anywhere;
  word-break: break-word;
}

.message-rich-body :deep(h1),
.message-rich-body :deep(h2),
.message-rich-body :deep(h3),
.message-rich-body :deep(h4),
.message-rich-body :deep(h5),
.message-rich-body :deep(h6) {
  margin: 0.8em 0 0.4em;
  font-weight: 600;
  line-height: 1.4;
}

.message-rich-body :deep(h1:first-child),
.message-rich-body :deep(h2:first-child),
.message-rich-body :deep(h3:first-child),
.message-rich-body :deep(h4:first-child),
.message-rich-body :deep(h5:first-child),
.message-rich-body :deep(h6:first-child),
.message-rich-body :deep(p:first-child) {
  margin-top: 0;
}

.message-rich-body :deep(p) {
  margin: 0.5em 0;
}

.message-rich-body :deep(ul),
.message-rich-body :deep(ol) {
  margin: 0.5em 0;
  padding-left: 1.6em;
}

.message-rich-body :deep(li) {
  margin: 0.25em 0;
}

.message-rich-body :deep(a) {
  color: var(--el-color-primary);
  text-decoration: none;
}

.message-rich-body :deep(a:hover) {
  text-decoration: underline;
}

.message-rich-body :deep(code) {
  padding: 0.15em 0.45em;
  border-radius: 4px;
  background: rgb(128 128 128 / 15%);
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 0.9em;
}

.message-rich-body :deep(pre) {
  margin: 0.6em 0;
  padding: 0.8em 1em;
  overflow-x: auto;
  border: 1px solid rgb(128 128 128 / 25%);
  border-radius: 6px;
  background: rgb(128 128 128 / 12%);
}

.message-rich-body :deep(pre code) {
  padding: 0;
  background: transparent;
  font-size: 0.9em;
}

.message-rich-body :deep(blockquote) {
  margin: 0.6em 0;
  padding: 0.4em 1em;
  border-left: 3px solid rgb(128 128 128 / 40%);
  border-radius: 4px;
  background: rgb(128 128 128 / 10%);
  color: inherit;
  opacity: 0.85;
}

.message-rich-body :deep(table) {
  width: 100%;
  margin: 0.6em 0;
  border-collapse: collapse;
}

.message-rich-body :deep(th),
.message-rich-body :deep(td) {
  padding: 0.4em 0.8em;
  border: 1px solid rgb(128 128 128 / 30%);
  text-align: left;
}

.message-rich-body :deep(th) {
  background: rgb(128 128 128 / 12%);
  font-weight: 600;
}

.message-rich-body :deep(img) {
  max-width: 100%;
  height: auto;
  border-radius: 4px;
}

.message-rich-body :deep(hr) {
  margin: 1em 0;
  border: none;
  border-top: 1px solid rgb(128 128 128 / 30%);
}
</style>
