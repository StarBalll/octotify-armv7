<template>
  <div class="p-4">
    <!-- 统计卡片行 -->
    <ElRow :gutter="16" class="mb-4">
      <ElCol :xs="24" :sm="12" :md="6" v-for="card in statCards" :key="card.key">
        <ElCard shadow="hover" class="stat-card">
          <div class="flex items-center gap-4">
            <div
              class="flex items-center justify-center w-12 h-12 rounded-lg"
              :class="card.bgColor"
            >
              <IconifyIcon :icon="card.icon" class="text-2xl" :class="card.color" />
            </div>
            <div>
              <p class="text-sm text-gray-500">{{ card.label }}</p>
              <p class="text-2xl font-semibold">
                <template v-if="card.key === 'successRate'">
                  {{ statData[card.key] }}%
                </template>
                <template v-else>
                  {{ statData[card.key] ?? '--' }}
                </template>
              </p>
            </div>
          </div>
        </ElCard>
      </ElCol>
    </ElRow>

    <!-- 最近推送记录列表 -->
    <ElCard :title="$t('page.dashboard.recentPush')" shadow="hover">
      <ElTable
        v-loading="tableLoading"
        :data="recentMessages"
        stripe
        style="width: 100%"
        :empty-text="$t('page.dashboard.noRecords')"
        @row-click="handleRowClick"
      >
        <ElTableColumn
          prop="title"
          :label="$t('page.dashboard.messageTitle')"
          min-width="160"
          show-overflow-tooltip
        >
          <template #default="{ row }">
            <a
              class="text-blue-500 hover:text-blue-700 cursor-pointer"
              @click.stop="handleViewDetail(row.id)"
            >
              {{ row.title }}
            </a>
          </template>
        </ElTableColumn>

        <ElTableColumn
          prop="source_name"
          :label="$t('page.dashboard.sourceName')"
          min-width="120"
          show-overflow-tooltip
        >
          <template #default="{ row }">
            {{ row.source_name || '--' }}
          </template>
        </ElTableColumn>

        <ElTableColumn
          prop="channel_name"
          :label="$t('page.dashboard.channelName')"
          min-width="120"
          show-overflow-tooltip
        >
          <template #default="{ row }">
            {{ row.channel_name || '--' }}
          </template>
        </ElTableColumn>

        <ElTableColumn prop="status" :label="$t('page.dashboard.status')" width="100" align="center">
          <template #default="{ row }">
            <ElTag :type="getStatusTagType(row.status)" size="small">
              {{ getStatusLabel(row.status) }}
            </ElTag>
          </template>
        </ElTableColumn>

        <ElTableColumn prop="created_at_ts" :label="$t('page.dashboard.createTime')" width="180">
          <template #default="{ row }">
            {{ formatTimestamp(row.created_at_ts) }}
          </template>
        </ElTableColumn>
      </ElTable>
    </ElCard>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue';
import { useRouter } from 'vue-router';
import { ElCard, ElRow, ElCol, ElTable, ElTableColumn, ElTag, ElMessage } from 'element-plus';
import { IconifyIcon } from '@vben/icons';
import { $t } from '#/locales';
import { getSourceListApi } from '#/api/modules/source';
import { getChannelListApi } from '#/api/modules/channel';
import { getMessageListApi, filterMessagesApi, type MessageApi } from '#/api/modules/message';
import { formatTimestamp } from '#/utils/time';
import { getMessageStatusLabel, getMessageStatusTagType } from '#/utils/status';

const router = useRouter();

// 统计数据
const statData = ref({
  sourceTotal: 0,
  channelTotal: 0,
  todayPush: 0,
  successRate: 0,
});

// 最近推送记录
const recentMessages = ref<MessageApi.MessageDTO[]>([]);
const tableLoading = ref(false);

// 统计卡片配置
const statCards = computed(() => [
  {
    key: 'sourceTotal' as const,
    label: $t('page.dashboard.sourceTotal'),
    icon: 'mdi:source-branch',
    bgColor: 'bg-blue-50',
    color: 'text-blue-500',
  },
  {
    key: 'channelTotal' as const,
    label: $t('page.dashboard.channelTotal'),
    icon: 'mdi:pipe',
    bgColor: 'bg-green-50',
    color: 'text-green-500',
  },
  {
    key: 'todayPush' as const,
    label: $t('page.dashboard.todayPush'),
    icon: 'mdi:send',
    bgColor: 'bg-orange-50',
    color: 'text-orange-500',
  },
  {
    key: 'successRate' as const,
    label: $t('page.dashboard.successRate'),
    icon: 'mdi:chart-line',
    bgColor: 'bg-purple-50',
    color: 'text-purple-500',
  },
]);

// 状态映射
// 已改用导入的函数
const getStatusLabel = getMessageStatusLabel;
const getStatusTagType = getMessageStatusTagType;

// 获取今日 0 点的时间戳（毫秒）
function getTodayStartTimestamp(): number {
  const now = new Date();
  const todayStart = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  return todayStart.getTime();
}

// 获取当前时间戳（毫秒）
function getCurrentTimestamp(): number {
  return Date.now();
}

// 加载统计数据
async function loadStats() {
  try {
    // 并行请求来源总数和渠道总数
    const [sourceRes, channelRes] = await Promise.all([
      getSourceListApi({ page: 1, page_size: 1 }),
      getChannelListApi({ page: 1, page_size: 1 }),
    ]);

    statData.value.sourceTotal = sourceRes?.total ?? 0;
    statData.value.channelTotal = channelRes?.total ?? 0;
  } catch (error) {
    console.error('Failed to load stats:', error);
  }

  try {
    // 今日推送数据
    const todayRes = await filterMessagesApi({
      page: 1,
      page_size: 100,
      start_date: getTodayStartTimestamp(),
      end_date: getCurrentTimestamp(),
    });

    const todayList = todayRes?.list ?? [];
    statData.value.todayPush = todayList.length;

    // 计算今日成功率
    if (todayList.length > 0) {
      const successCount = todayList.filter((m) => m.status === 200).length;
      // 只统计已推送的（成功或失败），排除待推送
      const pushedCount = todayList.filter((m) => m.status === 200 || m.status === 300).length;
      statData.value.successRate = pushedCount > 0
        ? Math.round((successCount / pushedCount) * 100)
        : 0;
    } else {
      statData.value.successRate = 0;
    }
  } catch (error) {
    console.error('Failed to load today stats:', error);
  }
}

// 加载最近推送记录
async function loadRecentMessages() {
  tableLoading.value = true;
  try {
    const res = await getMessageListApi({ page: 1, page_size: 10 });
    recentMessages.value = res?.list ?? [];
  } catch (error) {
    ElMessage.error($t('page.dashboard.loadRecentFailed'));
    console.error('Failed to load recent messages:', error);
  } finally {
    tableLoading.value = false;
  }
}

// 查看消息详情：点击行或标题跳转至消息详情页
function handleRowClick(row: MessageApi.MessageDTO) {
  handleViewDetail(row.id);
}

function handleViewDetail(messageId: number) {
  if (!messageId) return;
  router.push(`/message/detail/${messageId}`);
}

// 初始化加载
onMounted(() => {
  loadStats();
  loadRecentMessages();
});
</script>

<style scoped>
.stat-card {
  transition: all 0.3s;
}

.stat-card:hover {
  transform: translateY(-2px);
}
</style>
