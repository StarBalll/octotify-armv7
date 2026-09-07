<template>
  <div class="p-4">
    <ElCard v-loading="loading" shadow="never">
      <template #header>
        <div class="flex items-center justify-between">
          <span class="font-semibold">{{ $t('page.source.detail') }}</span>
          <ElButton type="primary" @click="handleBack">
            <IconifyIcon icon="mdi:arrow-left" class="mr-1" />
            {{ $t('common.back') }}
          </ElButton>
        </div>
      </template>

      <template v-if="sourceDetail">
        <!-- 基本信息 -->
        <ElDescriptions :column="2" border class="mb-6">
          <ElDescriptionsItem :label="$t('page.source.name')">
            {{ sourceDetail.source.name }}
          </ElDescriptionsItem>
          <ElDescriptionsItem :label="$t('page.source.status')">
            <ElTag :type="sourceDetail.source.status === 1 ? 'success' : 'danger'" size="small">
              {{ getStatusLabel(sourceDetail.source.status) }}
            </ElTag>
          </ElDescriptionsItem>
          <ElDescriptionsItem :label="$t('page.source.description')" :span="2">
            {{ sourceDetail.source.description || '--' }}
          </ElDescriptionsItem>
          <ElDescriptionsItem :label="$t('page.source.createTime')">
            {{ formatTimestamp(sourceDetail.source.created_at_ts) }}
          </ElDescriptionsItem>
          <ElDescriptionsItem :label="$t('page.source.lastUsedTime')">
            {{ sourceDetail.source.last_used_at_ts ? formatTimestamp(sourceDetail.source.last_used_at_ts) : $t('page.source.neverUsed') }}
          </ElDescriptionsItem>
        </ElDescriptions>

        <!-- Token 展示区 -->
        <ElCard :title="$t('page.source.token')" shadow="never" class="mb-6">
          <div class="token-section">
            <template v-if="tokenValue">
              <div class="token-display">
                <ElInput
                  :model-value="showTokenPlainText ? tokenValue : maskedToken"
                  readonly
                  size="large"
                  class="token-input"
                >
                  <template #append>
                    <ElTooltip :content="showTokenPlainText ? $t('page.source.hideToken') : $t('page.source.showToken')" placement="top">
                      <ElButton @click="showTokenPlainText = !showTokenPlainText">
                        <IconifyIcon :icon="showTokenPlainText ? 'mdi:eye-off' : 'mdi:eye'" />
                      </ElButton>
                    </ElTooltip>
                    <ElTooltip :content="$t('page.source.copyToken')" placement="top">
                      <ElButton @click="handleCopyToken">
                        <IconifyIcon icon="mdi:content-copy" />
                      </ElButton>
                    </ElTooltip>
                  </template>
                </ElInput>
              </div>
              <ElAlert
                v-if="showTokenPlainText"
                :title="$t('page.source.tokenWarning')"
                type="warning"
                :closable="false"
                show-icon
                class="mt-2"
              />
            </template>
            <template v-else>
              <ElButton type="primary" @click="handleViewToken">
                <IconifyIcon icon="mdi:key" class="mr-1" />
                {{ $t('page.source.viewToken') }}
              </ElButton>
            </template>
          </div>

          <div class="mt-4">
            <ElButton type="danger" plain @click="handleResetToken">
              <IconifyIcon icon="mdi:key-change" class="mr-1" />
              {{ $t('page.source.resetToken') }}
            </ElButton>
          </div>
        </ElCard>

        <!-- 已绑定渠道 -->
        <ElCard :title="$t('page.source.channelBindings')" shadow="never">
          <ElEmpty v-if="sourceDetail.channels.length === 0" :description="$t('page.source.noChannels')" />
          <ElTable v-else :data="sourceDetail.channels" stripe>
            <ElTableColumn prop="id" label="ID" width="80" align="center" />
            <ElTableColumn prop="name" :label="$t('page.channel.name')" min-width="150" />
            <ElTableColumn prop="type" :label="$t('page.channel.type')" width="150" align="center">
              <template #default="{ row }">
                <ElTag size="small">{{ getChannelTypeLabel(row.type) }}</ElTag>
              </template>
            </ElTableColumn>
          </ElTable>
        </ElCard>
      </template>
    </ElCard>

    <!-- 密码二次验证对话框 -->
    <StepUpAuthDialog
      v-model:visible="stepUpVisible"
      :title="stepUpTitle"
      :description="stepUpDescription"
      @confirm="handleStepUpConfirm"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import {
  ElCard,
  ElButton,
  ElDescriptions,
  ElDescriptionsItem,
  ElTag,
  ElInput,
  ElTooltip,
  ElAlert,
  ElTable,
  ElTableColumn,
  ElEmpty,
  ElMessage,
} from 'element-plus';
import { IconifyIcon } from '@vben/icons';
import { $t } from '#/locales';
import {
  getSourceDetailApi,
  getSourceTokenApi,
  resetSourceTokenApi,
  type SourceApi,
} from '#/api/modules/source';
import StepUpAuthDialog from '#/components/StepUpAuthDialog.vue';
import { formatTimestamp } from '#/utils/time';

const route = useRoute();
const router = useRouter();

const loading = ref(false);
const sourceDetail = ref<SourceApi.SourceDetailResponse | null>(null);
const tokenValue = ref('');
const showTokenPlainText = ref(false);

// 二次验证状态
const stepUpVisible = ref(false);
const stepUpTitle = ref('');
const stepUpDescription = ref('');
let stepUpResolve: ((password: string) => void) | null = null;

// 脱敏 Token
const maskedToken = computed(() => {
  if (!tokenValue.value) return '';
  const len = tokenValue.value.length;
  if (len <= 8) return '****';
  return tokenValue.value.slice(0, 4) + '****' + tokenValue.value.slice(-4);
});

// 加载详情
async function loadDetail() {
  const id = Number(route.params.id);
  if (!id) {
    ElMessage.error('无效的来源 ID');
    return;
  }

  loading.value = true;
  try {
    const res = await getSourceDetailApi(id);
    sourceDetail.value = res;
  } catch {
    ElMessage.error('加载来源详情失败');
  } finally {
    loading.value = false;
  }
}

// 状态映射
function getStatusLabel(status: number): string {
  return status === 1 ? $t('page.source.enable') : $t('page.source.disable');
}

// 渠道类型映射
const channelTypeMap: Record<string, string> = {
  wechat: '微信',
  telegram: 'Telegram',
  dingtalk: '钉钉',
  email: '邮件',
  webhook: 'Webhook',
  feishu: '飞书',
};

function getChannelTypeLabel(type: string): string {
  return channelTypeMap[type] || type;
}

// 显示二次验证对话框
async function showStepUp(title: string, description: string): Promise<string | null> {
  return new Promise((resolve) => {
    stepUpTitle.value = title;
    stepUpDescription.value = description;
    stepUpResolve = resolve;
    stepUpVisible.value = true;
  });
}

function handleStepUpConfirm(password: string) {
  stepUpVisible.value = false;
  stepUpResolve?.(password);
  stepUpResolve = null;
}

// 查看 Token
async function handleViewToken() {
  const id = Number(route.params.id);
  const password = await showStepUp(
    $t('page.source.confirmViewToken'),
    $t('page.source.confirmViewTokenDesc')
  );
  if (!password) return;

  try {
    const res = await getSourceTokenApi(id, { password });
    tokenValue.value = res.token;
    // 用户已通过密码二次验证并明确要求"查看令牌"，直接显示明文（眼睛图标可切回脱敏）
    showTokenPlainText.value = true;
    ElMessage.success($t('page.source.viewTokenSuccess'));
  } catch {
    ElMessage.error('查看令牌失败');
  }
}

// 复制 Token
async function handleCopyToken() {
  if (!tokenValue.value) return;
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(tokenValue.value);
    } else {
      // HTTP 环境降级：navigator.clipboard 仅在 HTTPS 安全上下文存在，
      // 局域网 HTTP 部署时改用临时 textarea + execCommand
      const textarea = document.createElement('textarea');
      textarea.value = tokenValue.value;
      textarea.style.position = 'fixed';
      textarea.style.opacity = '0';
      document.body.appendChild(textarea);
      textarea.select();
      const ok = document.execCommand('copy');
      document.body.removeChild(textarea);
      if (!ok) throw new Error('execCommand copy failed');
    }
    ElMessage.success($t('page.source.tokenCopied'));
  } catch {
    ElMessage.error('复制失败');
  }
}

// 重置 Token
async function handleResetToken() {
  const id = Number(route.params.id);
  const password = await showStepUp(
    $t('page.source.confirmResetToken'),
    $t('page.source.confirmResetTokenDesc')
  );
  if (!password) return;

  try {
    const res = await resetSourceTokenApi(id, { password });
    tokenValue.value = res.token;
    // 重置后立即展示新令牌明文（眼睛图标可切回脱敏）
    showTokenPlainText.value = true;
    ElMessage.success($t('page.source.resetTokenSuccess'));
  } catch {
    ElMessage.error('重置令牌失败');
  }
}

function handleBack() {
  router.push('/source/list');
}

onMounted(() => {
  loadDetail();
});
</script>

<style scoped>
.token-section {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.token-display {
  display: flex;
  align-items: center;
}

.token-input {
  flex: 1;
}

.token-input :deep(.el-input__inner) {
  font-family: 'Courier New', monospace;
  letter-spacing: 1px;
}
</style>
