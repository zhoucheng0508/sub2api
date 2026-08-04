<template>
  <div class="flex min-w-0 flex-col items-start gap-1" data-test="moderation-audit-status">
    <span
      class="inline-flex rounded-md px-2 py-1 text-xs font-medium"
      :class="badgeClass"
      :data-test="`audit-status-${normalizedStatus}`"
    >
      {{ label }}
    </span>
    <span v-if="code" class="max-w-40 truncate font-mono text-[11px] text-gray-400" :title="code">
      {{ code }}
    </span>
    <span v-if="retryable" class="text-[11px] font-medium text-amber-600 dark:text-amber-300">
      {{ t('admin.riskControl.auditRetryable') }}
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ContentModerationAuditStatus } from '@/api/admin/riskControl'

const props = withDefaults(defineProps<{
  status?: ContentModerationAuditStatus
  action?: string
  flagged?: boolean
  code?: string
  retryable?: boolean
}>(), {
  status: undefined,
  action: '',
  flagged: false,
  code: '',
  retryable: false,
})

const { t } = useI18n()

const normalizedStatus = computed<ContentModerationAuditStatus>(() => {
  if (props.status === 'success' || props.status === 'skipped' || props.status === 'incomplete' || props.status === 'error') {
    return props.status
  }
  if (props.action === 'error') return 'error'
  if (props.action === 'skip') return 'skipped'
  return 'success'
})

const label = computed(() => {
  if (normalizedStatus.value === 'skipped') return t('admin.riskControl.auditStatusSkipped')
  if (normalizedStatus.value === 'incomplete') return t('admin.riskControl.auditStatusIncomplete')
  if (normalizedStatus.value === 'error') return t('admin.riskControl.auditStatusError')
  if (props.action === 'cyber_policy') return t('admin.riskControl.action.cyberPolicy')
  if (props.action === 'keyword_block') return t('admin.riskControl.action.keywordBlock')
  if (props.action === 'hash_block') return t('admin.riskControl.action.hashBlock')
  if (props.action === 'block') return t('admin.riskControl.action.block')
  if (props.action === 'observe') return t('admin.riskControl.action.observe')
  if (props.flagged) return t('admin.riskControl.result.hit')
  return t('admin.riskControl.result.pass')
})

const badgeClass = computed(() => {
  if (normalizedStatus.value === 'error') return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
  if (normalizedStatus.value === 'incomplete') return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
  if (normalizedStatus.value === 'skipped') return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
  if (props.action === 'block' || props.action === 'keyword_block' || props.action === 'hash_block' || props.action === 'cyber_policy') {
    return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
  }
  if (props.action === 'observe') return 'bg-sky-100 text-sky-700 dark:bg-sky-900/30 dark:text-sky-300'
  if (props.flagged) return 'bg-pink-100 text-pink-700 dark:bg-pink-900/30 dark:text-pink-300'
  return 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300'
})
</script>
