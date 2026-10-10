<template>
  <div class="min-w-32 text-xs" :title="t('admin.accounts.upstreamBalance.hint')">
    <div class="flex items-center gap-2">
      <span v-if="data?.status === 'ok'" class="font-medium tabular-nums" :class="state?.failed ? 'text-amber-600' : 'text-gray-900 dark:text-gray-100'">
        {{ data.unlimited ? t('admin.accounts.upstreamBalance.unlimited') : amount }}
      </span>
      <span v-else class="text-gray-400">{{ t(state?.loading ? 'admin.accounts.upstreamBalance.loading' : 'admin.accounts.upstreamBalance.unavailable') }}</span>
      <button v-if="eligible" class="rounded p-1 text-gray-500 hover:text-primary-600 disabled:opacity-40" :disabled="state?.loading" :aria-label="t('admin.accounts.upstreamBalance.refresh')" @click="$emit('refresh')">
        <Icon name="refresh" size="sm" :class="state?.loading ? 'animate-spin' : ''" />
      </button>
    </div>
    <div v-if="data?.status === 'ok'" class="mt-1 text-gray-500">
      {{ t(`admin.accounts.upstreamBalance.${data.scope || 'upstream'}`) }}
    </div>
    <div v-if="data?.status === 'ok'" class="mt-1 text-gray-400">
      {{ t('admin.accounts.upstreamBalance.updated', { time: new Date(data.checked_at).toLocaleString() }) }}
    </div>
    <div v-if="state?.failed && data?.status === 'ok'" class="mt-1 text-amber-600">{{ t('admin.accounts.upstreamBalance.stale') }}</div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { BalanceState } from '@/composables/useUpstreamBalances'

const props = defineProps<{ state?: BalanceState; eligible: boolean }>()
defineEmits<{ refresh: [] }>()
const { t } = useI18n()
const data = computed(() => props.state?.data)
const amount = computed(() => {
  const value = data.value?.amount
  if (value == null || !Number.isFinite(value)) return t('admin.accounts.upstreamBalance.unavailable')
  const unit = data.value?.unit
  const label = unit === 'TOKENS' || unit === 'QUOTA' ? t('admin.accounts.upstreamBalance.quotaUnit') : unit
  return `${value.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 4 })} ${label || ''}`
})
</script>
