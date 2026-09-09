<template>
  <div class="p-4">
    <ElCard v-loading="loading" shadow="never">
      <template #header>
        <div class="flex items-center justify-between">
          <span class="font-semibold">{{ $t('page.channel.edit') }}</span>
          <ElButton @click="handleBack">
            <IconifyIcon icon="mdi:arrow-left" class="mr-1" />
            {{ $t('common.back') }}
          </ElButton>
        </div>
      </template>

      <template v-if="channelDetail && channelTypeMeta">
        <!-- 渠道名称 -->
        <ElForm
          ref="formRef"
          :model="formData"
          :rules="rules"
          label-width="140px"
          class="max-w-2xl mb-4"
        >
          <ElFormItem :label="$t('page.channel.name')" prop="name">
            <ElInput
              v-model="formData.name"
              :placeholder="$t('page.channel.namePlaceholder')"
              maxlength="128"
              show-word-limit
            />
          </ElFormItem>
        </ElForm>

        <!-- 配置表单 -->
        <ElForm
          ref="configFormRef"
          :model="configData"
          :rules="configRules"
          label-width="140px"
          class="max-w-2xl mb-4"
        >
          <ElFormItem
            v-for="field in channelTypeMeta.config_fields"
            :key="field.name"
            :label="field.label"
            :prop="field.name"
          >
            <!-- URL 类型 -->
            <ElInput
              v-if="field.type === 'url'"
              v-model="configData[field.name]"
              :placeholder="field.placeholder"
            />
            <!-- 密码类型 -->
            <ElInput
              v-else-if="field.type === 'password'"
              v-model="configData[field.name]"
              type="password"
              :placeholder="field.placeholder"
              show-password
            />
            <!-- 数字类型 -->
            <ElInputNumber
              v-else-if="field.type === 'number'"
              v-model="configData[field.name]"
              :placeholder="field.placeholder"
              style="width: 100%"
            />
            <!-- 多行文本类型（如自定义 Webhook 的请求体模板/请求头） -->
            <ElInput
              v-else-if="field.type === 'textarea'"
              v-model="configData[field.name]"
              type="textarea"
              :rows="4"
              :placeholder="field.placeholder"
            />
            <!-- 默认文本类型 -->
            <ElInput
              v-else
              v-model="configData[field.name]"
              :placeholder="field.placeholder"
            />
          </ElFormItem>
        </ElForm>


        <!-- 提交按钮 -->
        <ElFormItem class="mt-4">
          <ElButton type="primary" :loading="submitting" @click="handleSubmit">
            {{ $t('common.confirm') }}
          </ElButton>
          <ElButton @click="handleBack">
            {{ $t('common.cancel') }}
          </ElButton>
        </ElFormItem>
      </template>
    </ElCard>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import {
  ElCard,
  ElForm,
  ElFormItem,
  ElInput,
  ElInputNumber,
  ElButton,
  ElMessage,
  type FormInstance,
  type FormRules,
} from 'element-plus';
import { IconifyIcon } from '@vben/icons';
import { $t } from '#/locales';
import {
  getChannelDetailApi,
  updateChannelApi,
  getChannelTypesApi,
  type ChannelApi,
} from '#/api/modules/channel';

const route = useRoute();
const router = useRouter();

const loading = ref(false);
const submitting = ref(false);
const channelDetail = ref<ChannelApi.ChannelDTO | null>(null);
const channelTypeMeta = ref<ChannelApi.ChannelTypeMeta | null>(null);

const formData = reactive({ name: '' });
const configData = reactive<Record<string, any>>({});

// 动态校验规则
const configRules = computed<FormRules>(() => {
  if (!channelTypeMeta.value) return {};
  const rules: FormRules = {};
  channelTypeMeta.value.config_fields.forEach((field) => {
    if (field.required) {
      rules[field.name] = [
        { required: true, message: `${field.label}不能为空`, trigger: 'blur' },
      ];
    }
  });
  return rules;
});

const formRef = ref<FormInstance>();
const configFormRef = ref<FormInstance>();


const rules: FormRules = {
  name: [
    { required: true, message: $t('page.channel.nameRequired'), trigger: 'blur' },
    { max: 128, message: $t('page.channel.nameMaxLength'), trigger: 'blur' },
  ],
};

// 加载渠道详情
async function loadDetail() {
  const id = Number(route.params.id);
  if (!id) {
    ElMessage.error($t('page.channel.invalidId'));
    return;
  }

  loading.value = true;
  try {
    const res = await getChannelDetailApi(id);
    channelDetail.value = res;

    // 预填名称
    formData.name = res.name;
  } catch {
    ElMessage.error($t('page.channel.loadDetailFailed'));
  } finally {
    loading.value = false;
  }
}

// 加载渠道类型元数据并预填配置
async function loadTypeMeta() {
  const type = channelDetail.value?.type;
  if (!type) return;

  try {
    const types = await getChannelTypesApi();
    channelTypeMeta.value = types.find((t) => t.type === type) || null;

    // 预填配置（必须在 type meta 加载后执行）
    if (channelTypeMeta.value && channelDetail.value) {
      channelTypeMeta.value.config_fields.forEach((field) => {
        const existingValue = channelDetail.value!.config[field.name];
        if (field.type === 'number') {
          configData[field.name] = existingValue ? Number(existingValue) : undefined;
        } else {
          configData[field.name] = existingValue ?? '';
        }
      });
    }
  } catch {
    // 不影响主流程
  }
}

// 提交表单
async function handleSubmit() {
  const id = Number(route.params.id);

  // 校验名称
  const nameValid = await formRef.value?.validate().catch(() => false);
  if (!nameValid) return;

  // 校验配置
  const configValid = await configFormRef.value?.validate().catch(() => false);
  if (!configValid) return;

  submitting.value = true;
  try {
    const config: Record<string, any> = {};
    channelTypeMeta.value?.config_fields.forEach((field) => {
      config[field.name] = configData[field.name];
    });

    await updateChannelApi(id, {
      name: formData.name,
      config,
    });

    ElMessage.success($t('page.channel.editSuccess'));
    router.push('/channel/list');
  } catch {
    ElMessage.error($t('page.channel.editFailed'));
  } finally {
    submitting.value = false;
  }
}

function handleBack() {
  router.push('/channel/list');
}

onMounted(async () => {
  await loadDetail();
  await loadTypeMeta();
});
</script>
