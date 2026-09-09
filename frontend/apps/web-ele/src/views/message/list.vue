<template>
  <div class="p-4">
    <!-- 筛选区 -->
    <ElCard shadow="never" class="mb-4">
      <ElForm :model="queryParams" inline>
        <ElFormItem :label="$t('page.message.source')">
          <ElSelect v-model="queryParams.source_id" clearable :placeholder="$t('page.message.allSources')" style="width: 160px">
            <ElOption
              v-for="s in sourceOptions"
              :key="s.id"
              :label="s.name"
              :value="s.id"
            />
          </ElSelect>
        </ElFormItem>
        <ElFormItem :label="$t('page.message.channel')">
          <ElSelect v-model="queryParams.channel_id" clearable :placeholder="$t('page.message.allChannels')" style="width: 160px">
            <ElOption
              v-for="c in channelOptions"
              :key="c.id"
              :label="c.name"
              :value="c.id"
            />
          </ElSelect>
        </ElFormItem>
        <ElFormItem :label="$t('page.message.status')">
          <ElSelect v-model="queryParams.status" clearable :placeholder="$t('page.message.allStatus')" style="width: 140px">
            <ElOption :label="$t('page.message.statusPending')" :value="100" />
            <ElOption :label="$t('page.message.statusSuccess')" :value="200" />
            <ElOption :label="$t('page.message.statusFailed')" :value="300" />
          </ElSelect>
        </ElFormItem>
        <ElFormItem :label="$t('page.message.dateRange')">
          <ElDatePicker
            v-model="dateRange"
            type="daterange"
            range-separator="-"
            value-format="x"
            style="width: 260px"
          />
        </ElFormItem>
        <ElFormItem :label="$t('page.message.keyword')">
          <ElInput
            v-model="queryParams.keyword"
            :placeholder="$t('page.message.keywordPlaceholder')"
            clearable
            @keyup.enter="handleSearch"
          />
        </ElFormItem>
        <ElFormItem>
          <ElButton type="primary" @click="handleSearch">
            <IconifyIcon icon="mdi:magnify" class="mr-1" />
            {{ $t('common.search') }}
          </ElButton>
          <ElButton @click="handleReset">
            {{ $t('common.reset') }}
          </ElButton>
        </ElFormItem>
      </ElForm>
    </ElCard>

    <!-- 表格 -->
    <ElCard shadow="never">
      <template #header>
        <div class="flex items-center justify-between">
          <span class="font-semibold">{{ $t('page.message.list') }}</span>
        </div>
      </template>

      <ElTable v-loading="loading" :data="messageList" stripe style="width: 100%">
        <ElTableColumn prop="id" label="ID" width="80" align="center" />
        <ElTableColumn :label="$t('page.dashboard.messageTitle')" prop="title" min-width="180">
          <template #default="{ row }">
            <a
              class="text-blue-500 hover:text-blue-700 cursor-pointer"
              @click="handleDetail(row.id)"
            >
              {{ row.title }}
            </a>
          </template>
        </ElTableColumn>
        <ElTableColumn :label="$t('page.dashboard.sourceName')" prop="source_name" min-width="120">
          <template #default="{ row }">
            {{ row.source_name || '--' }}
          </template>
        </ElTableColumn>
        <ElTableColumn :label="$t('page.dashboard.channelName')" prop="channel_name" min-width="120">
          <template #default="{ row }">
            {{ row.channel_name || '--' }}
            <ElTag v-if="row.channel_type" size="small" class="ml-1">
              {{ getChannelTypeLabel(row.channel_type) }}
            </ElTag>
          </template>
        </ElTableColumn>
        <ElTableColumn :label="$t('page.message.status')" prop="status" width="120" align="center">
          <template #default="{ row }">
            <ElTag :type="getStatusTagType(row.status)" size="small">
              {{ getStatusLabel(row.status) }}
            </ElTag>
          </template>
        </ElTableColumn>
        <ElTableColumn :label="$t('page.message.createTime')" prop="created_at_ts" width="180">
          <template #default="{ row }">
            {{ formatTimestamp(row.created_at_ts) }}
          </template>
        </ElTableColumn>
        <ElTableColumn :label="$t('page.source.actions')" width="180" align="center" fixed="right">
          <template #default="{ row }">
            <ElButton link type="primary" size="small" @click="handleDetail(row.id)">
              {{ $t('page.message.viewDetail') }}
            </ElButton>
            <ElButton link type="danger" size="small" @click="handleDelete(row)">
              {{ $t('page.message.delete') }}
            </ElButton>
          </template>
        </ElTableColumn>
      </ElTable>

      <!-- 分页 -->
      <div class="flex justify-end mt-4">
        <ElPagination
          v-model:current-page="queryParams.page"
          v-model:page-size="queryParams.page_size"
          :total="total"
          :page-sizes="[10, 20, 50, 100]"
          background
          layout="total, sizes, prev, pager, next, jumper"
          @current-change="loadData"
          @size-change="handleSizeChange"
        />
      </div>
    </ElCard>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import {
  ElCard,
  ElForm,
  ElFormItem,
  ElInput,
  ElSelect,
  ElOption,
  ElButton,
  ElTable,
  ElTableColumn,
  ElTag,
  ElPagination,
  ElMessage,
  ElMessageBox,
  ElDatePicker,
} from 'element-plus';
import { IconifyIcon } from '@vben/icons';
import { $t } from '#/locales';
import {
  filterMessagesApi,
  deleteMessageApi,
  type MessageApi,
} from '#/api/modules/message';
import { getAllSourcesApi, type SourceApi } from '#/api/modules/source';
import { getAllChannelsApi, getChannelTypesApi, type ChannelApi } from '#/api/modules/channel';
import { formatTimestamp } from '#/utils/time';

const router = useRouter();

// 来源/渠道选项
const sourceOptions = ref<SourceApi.SourceDTO[]>([]);
const channelOptions = ref<ChannelApi.ChannelDTO[]>([]);
const channelTypeMeta = ref<Record<string, string>>({});

// 日期范围
const dateRange = ref<[number, number] | null>(null);

// 查询参数
const queryParams = reactive({
  source_id: undefined as number | undefined,
  channel_id: undefined as number | undefined,
  status: undefined as number | undefined,
  start_date: undefined as number | undefined,
  end_date: undefined as number | undefined,
  keyword: '',
  page: 1,
  page_size: 20,
});

// 列表数据
const messageList = ref<MessageApi.MessageDTO[]>([]);
const total = ref(0);
const loading = ref(false);

// 加载数据
async function loadData() {
  loading.value = true;
  try {
    const params: MessageApi.FilterParams = {
      page: queryParams.page,
      page_size: queryParams.page_size,
    };

    if (queryParams.source_id) params.source_id = queryParams.source_id;
    if (queryParams.channel_id) params.channel_id = queryParams.channel_id;
    if (queryParams.status !== undefined) params.status = queryParams.status;
    if (queryParams.start_date) params.start_date = queryParams.start_date;
    if (queryParams.end_date) params.end_date = queryParams.end_date;
    if (queryParams.keyword) params.keyword = queryParams.keyword;

    const res = await filterMessagesApi(params);
    messageList.value = res?.list ?? [];
    total.value = res?.total ?? 0;
  } catch {
    ElMessage.error($t('page.message.loadFailed'));
  } finally {
    loading.value = false;
  }
}

function handleSearch() {
  // 处理日期范围 - value-format="x" 返回字符串，需转为数字
  if (dateRange.value) {
    queryParams.start_date = Number(dateRange.value[0]);
    queryParams.end_date = Number(dateRange.value[1]);
  } else {
    queryParams.start_date = undefined;
    queryParams.end_date = undefined;
  }

  queryParams.page = 1;
  loadData();
}

function handleReset() {
  queryParams.source_id = undefined;
  queryParams.channel_id = undefined;
  queryParams.status = undefined;
  queryParams.start_date = undefined;
  queryParams.end_date = undefined;
  queryParams.keyword = '';
  queryParams.page = 1;
  dateRange.value = null;
  loadData();
}

function handleSizeChange() {
  queryParams.page = 1;
  loadData();
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

// 查看详情
function handleDetail(id: number) {
  router.push(`/message/detail/${id}`);
}

// 删除消息
async function handleDelete(row: MessageApi.MessageDTO) {
  try {
    await ElMessageBox.confirm(
      $t('page.message.confirmDeleteDesc'),
      $t('page.message.confirmDelete'),
      { type: 'warning' }
    );
    await deleteMessageApi(row.id);
    ElMessage.success($t('page.message.deleteSuccess'));
    loadData();
  } catch {
    // 用户取消或请求失败
  }
}

onMounted(async () => {
  // 并行加载来源、渠道、渠道类型
  try {
    const [sourcesRes, channelsRes, typesRes] = await Promise.allSettled([
      getAllSourcesApi(),
      getAllChannelsApi(),
      getChannelTypesApi(),
    ]);

    if (sourcesRes.status === 'fulfilled') {
      sourceOptions.value = sourcesRes.value?.list ?? [];
    }
    if (channelsRes.status === 'fulfilled') {
      channelOptions.value = channelsRes.value?.list ?? [];
    }
    if (typesRes.status === 'fulfilled') {
      typesRes.value.forEach((t) => {
        channelTypeMeta.value[t.type] = t.name;
      });
    }
  } catch {
    // 加载失败不影响列表展示
  }

  loadData();
});
</script>
