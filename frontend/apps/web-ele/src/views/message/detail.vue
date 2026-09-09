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
          <!-- 渲染视图：Markdown / HTML 经 DOMPurify 净化后展示 -->
          <div
            v-else-if="renderedHtml"
            class="message-rich-body"
            v-html="renderedHtml"
          ></div>
          <div v-else class="leading-relaxed opacity-50">--</div>
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
import { computed, ref, onMounted } from 'vue';
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

// 渲染视图 HTML：HTML 直接净化；其余内容一律按 GFM Markdown 解析
// （纯文本消息也能获得换行/链接/加粗等基础排版），输出统一经 DOMPurify 净化防 XSS
const renderedHtml = computed(() => {
  const content = messageDetail.value?.content ?? '';
  if (!content.trim()) return '';
  try {
    const rawHtml = looksLikeHtml(content)
      ? content
      : marked.parse(content, { async: false, gfm: true, breaks: true });
    return DOMPurify.sanitize(rawHtml, {
      FORBID_TAGS: ['iframe', 'object', 'embed', 'form', 'base', 'meta'],
    });
  } catch {
    return '';
  }
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
