<template>
  <AppLayout>
    <div class="space-y-6">
      <div v-if="loading" class="flex items-center justify-center py-12"><LoadingSpinner /></div>
      <template v-else-if="stats">
        <section
          v-if="statsLoaded && stats.total_requests === 0 && !authStore.isAdmin"
          data-testid="newcomer-guide"
          class="space-y-4"
        >
          <FirstUseJourney context="dashboard" :phase="fundingPhase" :balance="availableBalance" :loading="fundingLoading"
            :simple-mode="authStore.isSimpleMode"
            :online-available="onlineAvailable" :can-redeem="canRedeem" :subscription-only="subscriptionOnly"
            :key-ready="stats.active_api_keys > 0" :request-count="stats.total_requests" :contact-info="appStore.cachedPublicSettings?.contact_info" @refresh="refreshAll" />
        </section>
        <UserDashboardStats :stats="stats" :balance="user?.balance || 0" :is-simple="authStore.isSimpleMode" :platform-quotas="platformQuotas" />
        <UserDashboardCharts v-model:startDate="startDate" v-model:endDate="endDate" v-model:granularity="granularity" :loading="loadingCharts" :trend="trendData" :models="modelStats" @dateRangeChange="loadCharts" @granularityChange="loadCharts" @refresh="refreshAll" />
        <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
          <div class="lg:col-span-2"><UserDashboardRecentUsage :data="recentUsage" :loading="loadingUsage" /></div>
          <div class="lg:col-span-1"><UserDashboardQuickActions /></div>
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, watch } from 'vue'; import { useAuthStore } from '@/stores/auth'; import { usageAPI, type UserDashboardStats as UserStatsType } from '@/api/usage'
import AppLayout from '@/components/layout/AppLayout.vue'; import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import UserDashboardStats from '@/components/user/dashboard/UserDashboardStats.vue'; import UserDashboardCharts from '@/components/user/dashboard/UserDashboardCharts.vue'
import UserDashboardRecentUsage from '@/components/user/dashboard/UserDashboardRecentUsage.vue'; import UserDashboardQuickActions from '@/components/user/dashboard/UserDashboardQuickActions.vue'
import type { UsageLog, TrendDataPoint, ModelStat, PlatformQuotaItem } from '@/types'
import { getMyPlatformQuotas } from '@/api/user'
import { formatDateLocalInput } from '@/utils/format'
import { useAppStore } from '@/stores/app'
import FirstUseJourney from '@/custom/vote-ai/quick-connect/FirstUseJourney.vue'
import { useFundingReadiness } from '@/custom/vote-ai/quick-connect/useFundingReadiness'

const appStore = useAppStore()
const { phase: fundingPhase, balance: availableBalance, loading: fundingLoading, onlineAvailable, canRedeem, subscriptionOnly, refresh: refreshFunding } = useFundingReadiness()
const authStore = useAuthStore(); const user = computed(() => authStore.user)
const stats = ref<UserStatsType | null>(null); const loading = ref(false); const loadingUsage = ref(false); const loadingCharts = ref(false)
const statsLoaded = ref(false)
const trendData = ref<TrendDataPoint[]>([]); const modelStats = ref<ModelStat[]>([]); const recentUsage = ref<UsageLog[]>([])
const platformQuotas = ref<PlatformQuotaItem[] | null>(null)

const startDate = ref(formatDateLocalInput(new Date(Date.now() - 6 * 86400000))); const endDate = ref(formatDateLocalInput(new Date())); const granularity = ref('day')

let disposed = false, statsRequest = 0, chartsRequest = 0, recentRequest = 0, quotaRequest = 0
const identity = () => `${authStore.user?.id ?? ''}:${Boolean(authStore.token)}`
const guard = () => { const owner = identity(); return () => !disposed && owner === identity() }
const loadStats = async () => {
  const request = ++statsRequest, currentOwner = guard()
  const current = () => request === statsRequest && currentOwner()
  loading.value = true; statsLoaded.value = false
  try {
    await authStore.refreshUser()
    if (!current()) return
    const result = await usageAPI.getDashboardStats()
    if (current()) { stats.value = result; statsLoaded.value = true }
  } catch (error) { if (current()) console.error('Failed to load dashboard stats:', error) }
  finally { if (current()) loading.value = false }
}
const loadCharts = async () => {
  const request = ++chartsRequest, currentOwner = guard()
  const current = () => request === chartsRequest && currentOwner()
  loadingCharts.value = true
  try {
    const res = await Promise.all([usageAPI.getDashboardTrend({ start_date: startDate.value, end_date: endDate.value, granularity: granularity.value as any }), usageAPI.getDashboardModels({ start_date: startDate.value, end_date: endDate.value })])
    if (current()) { trendData.value = res[0].trend || []; modelStats.value = res[1].models || [] }
  } catch (error) { if (current()) console.error('Failed to load charts:', error) }
  finally { if (current()) loadingCharts.value = false }
}
const loadRecent = async () => {
  const request = ++recentRequest, currentOwner = guard()
  const current = () => request === recentRequest && currentOwner()
  loadingUsage.value = true
  try { const res = await usageAPI.getByDateRange(startDate.value, endDate.value); if (current()) recentUsage.value = res.items.slice(0, 5) }
  catch (error) { if (current()) console.error('Failed to load recent usage:', error) }
  finally { if (current()) loadingUsage.value = false }
}
const loadPlatformQuotas = async () => {
  const request = ++quotaRequest, currentOwner = guard()
  const current = () => request === quotaRequest && currentOwner()
  try { const data = await getMyPlatformQuotas(); if (current()) platformQuotas.value = data.platform_quotas ?? [] }
  catch (error) { if (current()) { console.warn('Failed to load platform quotas:', error); platformQuotas.value = [] } }
}
const refreshAll = () => { loadStats(); loadCharts(); loadRecent(); loadPlatformQuotas(); void refreshFunding() }

onMounted(() => { refreshAll() })
watch(() => [authStore.user?.id, Boolean(authStore.token)], (next, previous) => {
  if (next[0] === previous[0] && next[1] === previous[1]) return
  ++statsRequest; ++chartsRequest; ++recentRequest; ++quotaRequest
  statsLoaded.value = false; stats.value = null; trendData.value = []; modelStats.value = []; recentUsage.value = []; platformQuotas.value = null
  loading.value = false; loadingUsage.value = false; loadingCharts.value = false
  if (authStore.token && authStore.user?.id) refreshAll()
})
onBeforeUnmount(() => { disposed = true; ++statsRequest; ++chartsRequest; ++recentRequest; ++quotaRequest })
</script>
