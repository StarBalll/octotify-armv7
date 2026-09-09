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
          <!-- 原文视图：保留推送时的原始文本。
               超长原文用 content-visibility 跳过屏外渲染；双视图均常驻 DOM
               （v-show），tab 切换零重建开销 -->
          <div
            v-show="viewMode === 'source'"
            class="source-view whitespace-pre-wrap break-words leading-relaxed"
          >
            {{ messageDetail?.content || '--' }}
          </div>
          <!-- 渲染视图：Markdown / HTML 经 DOMPurify 净化后展示。
               超长文本分段按需渲染（首段 + 「加载更多」+ 滚动到底自动续载），
               已渲染段落缓存，切 tab / 往返滚动零重复解析 -->
          <div v-show="viewMode === 'rendered'">
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
            <!-- 加载更多：分段渲染的控制条（仅超长文出现） -->
            <div
              v-if="segmentState.visible"
              class="mt-3 flex justify-center"
            >
              <ElButton
                :loading="segmentState.loading"
                :type="segmentState.done ? 'default' : 'primary'"
                plain
                @click="loadMoreSegments"
              >
                {{
                  segmentState.done
                    ? $t('page.message.allLoaded')
                    : `${$t('page.message.loadMore')} (${segmentState.rendered}/${segmentState.total})`
                }}
              </ElButton>
            </div>
            <div v-if="!hasContent" class="leading-relaxed opacity-50">--</div>
          </div>
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
import { computed, ref, reactive, onMounted, onBeforeUnmount, watch, nextTick } from 'vue';
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
// 超长文本分段按需渲染（load-more）：
// 前版分块渐进渲染解决了"同步阻塞"，但所有段落最终仍全部插 DOM——
// 157KB 晚报约产生 3000+ 节点，打开瞬间与 tab 切换时一次性布局仍卡。
// 本版改为：
//   1) 源文按空行/围栏切分为段落组（一次性、纯字符串操作，约 10ms）
//   2) 首屏只渲染前 INITIAL_SEGMENTS 段；点击「加载更多」或滚动到
//      底部自动续载下一批（每批 parse+sanitize 仅毫秒级）
//   3) 每段渲染结果独立缓存（已净化 HTML），切 tab/往返滚动零重复解析；
//      已插入的 DOM 不再动，切换视图用 v-show 仅改 display
//   4) 原文视图加 content-visibility:auto，屏外行不参与布局
// ---------------------------------------------------------------------------
const mdBox = ref<HTMLElement | null>(null);
const renderStage = ref<'idle' | 'rendering' | 'done'>('idle');
const renderProgress = ref(0);
let renderToken = 0; // 渲染代际令牌：内容/模式变化或卸载时自增以取消旧任务

const SEGMENT_THRESHOLD = 24 * 1024; // 超过 24K 启用分段加载
const SEGMENT_TARGET = 24 * 1024; // 单段目标大小（约 1~2 屏）
const SEGMENT_HARD_LIMIT = 96 * 1024; // 单段硬上限（超长单行兜底）
const INITIAL_SEGMENTS = 2; // 首屏渲染段数
const LOAD_MORE_BATCH = 3; // 每次「加载更多」追加段数

interface SegmentState {
  visible: boolean; // 是否启用分段（超长文才显示控制条）
  total: number;
  rendered: number; // 已渲染段数
  loading: boolean;
  done: boolean;
}
const segmentState = reactive<SegmentState>({
  visible: false,
  total: 0,
  rendered: 0,
  loading: false,
  done: true,
});

// 分段数据（内容变化时重建）
let segments: string[] = [];
let segmentKind: 'md' | 'html' = 'md';
const segmentCache = new Map<number, string>(); // 段索引 → 已净化 HTML

/** 安全切点：块级结构起点行——空行 / 列表项 / 引用 / 标题 / 表格行。
 *  在这些行切开后各段独立渲染语义不变：无序列表拆成多个 <ul> 视觉一致，
 *  有序列表由 CommonMark start 属性保留编号；引用拆为相邻两个引用块。 */
const RE_SAFE_CUT = /^\s*(?:[-*+]\s+\S|\d{1,3}[.)]\s+\S|>|#{1,6}\s|\|)/;

/** 将 Markdown 源文切成围栏感知的段：优先在安全切点、达到目标大小时切段；
 *  无安全切点的超长块（巨型段落）超过硬上限后按任意行强切兜底 */
function splitMarkdownSegments(content: string): string[] {
  const lines = content.split('\n');
  const segs: string[] = [];
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
    if (!inFence) {
      const isBlank = line.trim() === '';
      const safeCut = isBlank || RE_SAFE_CUT.test(line);
      if ((safeCut && curLen >= SEGMENT_TARGET) || curLen >= SEGMENT_HARD_LIMIT) {
        segs.push(cur.join('\n'));
        cur = [];
        curLen = 0;
      }
    }
  }
  if (cur.length) segs.push(cur.join('\n'));
  return segs;
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
  segmentState.loading = false;
}

/** 解析+净化单个分段（带缓存） */
function renderSegmentHTML(idx: number): string {
  const cached = segmentCache.get(idx);
  if (cached !== undefined) return cached;
  let rawHtml = '';
  try {
    rawHtml =
      segmentKind === 'html'
        ? segments[idx]!
        : marked.parse(segments[idx]!, { async: false, gfm: true, breaks: true });
  } catch {
    rawHtml = '';
  }
  const clean = sanitizeChunk(rawHtml);
  segmentCache.set(idx, clean);
  return clean;
}

/** 将 [from, to) 段插入 DOM（to 不含），同步小批量，无感知卡顿 */
function appendSegments(from: number, to: number): void {
  const box = mdBox.value;
  if (!box) return;
  let html = '';
  for (let i = from; i < to && i < segments.length; i++) {
    html += renderSegmentHTML(i);
  }
  if (html) box.insertAdjacentHTML('beforeend', html);
  segmentState.rendered = Math.min(to, segments.length);
  segmentState.done = segmentState.rendered >= segments.length;
}

/** 「加载更多」按钮 / 滚动到底自动触发；一次一批，批间让出主线程 */
async function loadMoreSegments(): Promise<void> {
  if (segmentState.done || segmentState.loading) return;
  if (!segments.length) return;
  segmentState.loading = true;
  const token = renderToken;
  await nextFrame(); // 先让按钮 loading 态上屏
  if (token !== renderToken) {
    segmentState.loading = false;
    return;
  }
  const from = segmentState.rendered;
  appendSegments(from, from + LOAD_MORE_BATCH);
  segmentState.loading = false;
}

/** 滚动到底自动连续续填：每批一帧，直到离开底部区域/全部完成。
 *  每批毫秒级、批间让出一帧，浏览者无感知；用户向上滚动即自然停止 */
function handleScrollAutoLoad(): void {
  if (!segmentState.visible || segmentState.done || segmentState.loading) return;
  if (viewMode.value !== 'rendered') return;
  const scroller = document.scrollingElement;
  if (!scroller) return;
  const distance = scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight;
  if (distance >= 400) return;
  const token = renderToken;
  queueMicrotask(async () => {
    while (!segmentState.done && !segmentState.loading) {
      const sc = document.scrollingElement;
      if (!sc) break;
      const d = sc.scrollHeight - sc.scrollTop - sc.clientHeight;
      if (d >= 400) break; // 已离开底部（用户上滚/内容已够高）
      segmentState.loading = true;
      await nextFrame();
      if (token !== renderToken) {
        segmentState.loading = false;
        break;
      }
      const from = segmentState.rendered;
      appendSegments(from, from + LOAD_MORE_BATCH);
      segmentState.loading = false;
    }
  });
}

/** 渲染入口：小文一次性；超长文分段首屏 */
function startRender(content: string, isHtml: boolean) {
  const box = mdBox.value;
  if (!box) return;
  segmentCache.clear();
  box.innerHTML = '';
  if (content.length <= SEGMENT_THRESHOLD) {
    // 小文本：整体一段渲染
    segmentState.visible = false;
    segments = [content];
    segmentKind = isHtml ? 'html' : 'md';
    segmentState.total = 1;
    appendSegments(0, 1);
    renderStage.value = 'done';
    return;
  }
  // 超长文：切段（纯字符串操作）后仅渲染首屏段
  segmentKind = isHtml ? 'html' : 'md';
  segments = isHtml ? [content] : splitMarkdownSegments(content);
  segmentState.visible = true;
  segmentState.total = segments.length;
  segmentState.rendered = 0;
  segmentState.done = false;
  renderStage.value = 'rendering';
  renderProgress.value = 0;
  // 首屏段同步渲染（每段毫秒级，2 段 < 16ms，不闪进度条）
  appendSegments(0, Math.min(INITIAL_SEGMENTS, segments.length));
  renderStage.value = 'done';
}

/** 根据当前内容与视图模式调度渲染 */
async function scheduleRender() {
  if (viewMode.value !== 'rendered') return;
  const content = messageDetail.value?.content ?? '';
  cancelRender();
  if (!content.trim()) {
    if (mdBox.value) mdBox.value.innerHTML = '';
    segmentState.visible = false;
    return;
  }
  await nextTick(); // 确保 mdBox 已挂载
  if (mdBox.value) {
    startRender(content, looksLikeHtml(content));
  }
}

// 滚动到底自动续载：监听页面滚动，接近底部且还有未渲染段时自动加载
watch([messageDetail, viewMode], () => {
  scheduleRender();
});

onMounted(() => {
  window.addEventListener('scroll', handleScrollAutoLoad, { passive: true });
});

onBeforeUnmount(() => {
  cancelRender();
  window.removeEventListener('scroll', handleScrollAutoLoad);
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

/* 原文视图超长文本：跳过屏外行的布局与绘制（渐进增强，旧浏览器忽略） */
.source-view {
  content-visibility: auto;
  contain-intrinsic-size: auto 480px;
}
</style>
