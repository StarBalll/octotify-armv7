<template>
  <div class="p-4">
    <ElCard shadow="never">
      <template #header>
        <div class="flex items-center justify-between">
          <span class="font-semibold">{{ $t('page.source.create') }}</span>
          <ElButton @click="handleBack">
            <IconifyIcon icon="mdi:arrow-left" class="mr-1" />
            {{ $t('common.back') }}
          </ElButton>
        </div>
      </template>

      <ElForm
        v-if="!createdToken"
        ref="formRef"
        :model="formData"
        :rules="rules"
        label-width="120px"
        class="max-w-2xl"
      >
        <ElFormItem :label="$t('page.source.name')" prop="name">
          <ElInput
            v-model="formData.name"
            :placeholder="$t('page.source.namePlaceholder')"
            maxlength="128"
            show-word-limit
          />
        </ElFormItem>

        <ElFormItem :label="$t('page.source.description')" prop="description">
          <ElInput
            v-model="formData.description"
            type="textarea"
            :placeholder="$t('page.source.descriptionPlaceholder')"
            :rows="3"
            maxlength="512"
            show-word-limit
          />
        </ElFormItem>

        <ElFormItem :label="$t('page.source.channelBindings')">
          <ElSelect
            v-model="formData.channel_ids"
            multiple
            filterable
            :placeholder="$t('page.source.selectChannels')"
            style="width: 100%"
          >
            <ElOption
              v-for="channel in channels"
              :key="channel.id"
              :label="channel.name"
              :value="channel.id"
            >
              <span>{{ channel.name }}</span>
              <ElTag size="small" class="ml-2">{{ getChannelTypeLabel(channel.type) }}</ElTag>
            </ElOption>
          </ElSelect>
        </ElFormItem>

        <ElFormItem>
          <ElButton type="primary" :loading="submitting" @click="handleSubmit">
            {{ $t('common.create') }}
          </ElButton>
          <ElButton @click="handleBack">
            {{ $t('common.cancel') }}
          </ElButton>
        </ElFormItem>
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
    </ElCard>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import {
  ElAlert,
  ElCard,
  ElForm,
  ElFormItem,
  ElInput,
  ElSelect,
  ElOption,
  ElButton,
  ElTag,
  ElMessage,
  type FormInstance,
  type FormRules,
} from 'element-plus';
import { IconifyIcon } from '@vben/icons';
import { $t } from '#/locales';
import { createSourceApi, type SourceApi } from '#/api/modules/source';
import { getAllChannelsApi, type ChannelApi } from '#/api/modules/channel';

const router = useRouter();
const formRef = ref<FormInstance>();
const submitting = ref(false);

const formData = reactive<SourceApi.CreateSourceReq>({
  name: '',
  description: '',
  channel_ids: [],
});

const channels = ref<ChannelApi.ChannelDTO[]>([]);
// 创建成功后返回的推送令牌（仅创建时返回一次）
const createdToken = ref('');

const rules: FormRules = {
  name: [
    { required: true, message: $t('page.source.nameRequired'), trigger: 'blur' },
    { max: 128, message: $t('page.source.nameMaxLength'), trigger: 'blur' },
  ],
  description: [
    { max: 512, message: $t('page.source.descriptionMaxLength'), trigger: 'blur' },
  ],
};

// 加载渠道列表
async function loadChannels() {
  try {
    const res = await getAllChannelsApi();
    channels.value = res?.list ?? [];
  } catch {
    console.error('加载渠道列表失败');
  }
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

// 提交表单
async function handleSubmit() {
  if (!formRef.value) return;

  const valid = await formRef.value.validate().catch(() => false);
  if (!valid) return;

  submitting.value = true;
  try {
    // 清理空 channel_ids
    const data: SourceApi.CreateSourceReq = {
      name: formData.name,
      description: formData.description,
    };
    if (formData.channel_ids && formData.channel_ids.length > 0) {
      data.channel_ids = formData.channel_ids;
    }

    const res = await createSourceApi(data);
    createdToken.value = res?.token ?? '';
    ElMessage.success($t('page.source.createSuccess'));
    if (!createdToken.value) {
      // 异常兜底：后端未返回令牌时保持旧行为返回列表
      router.push('/source/list');
    }
  } catch {
    ElMessage.error('创建来源失败');
  } finally {
    submitting.value = false;
  }
}

// 复制新建来源的令牌（HTTP 环境降级：clipboard API 需安全上下文）
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

function handleBack() {
  router.push('/source/list');
}

onMounted(() => {
  loadChannels();
});
</script>
