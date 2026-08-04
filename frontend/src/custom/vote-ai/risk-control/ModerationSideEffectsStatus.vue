<template>
  <div class="mt-1 flex max-w-56 flex-col gap-1" data-test="moderation-side-effects-status">
    <div class="flex flex-wrap gap-1">
      <span class="inline-flex rounded-md px-1.5 py-0.5 text-[11px] font-medium" :class="sideEffectClass">
        {{ sideEffectLabel }}
      </span>
      <span class="inline-flex rounded-md px-1.5 py-0.5 text-[11px] font-medium" :class="notificationClass">
        {{ notificationLabel }}
      </span>
      <span
        v-if="moderationBanActive"
        class="inline-flex rounded-md bg-red-100 px-1.5 py-0.5 text-[11px] font-medium text-red-700 dark:bg-red-900/30 dark:text-red-300"
      >
        {{ t('admin.riskControl.moderationBanActive') }}
      </span>
    </div>
    <p v-if="error" class="line-clamp-2 text-[11px] leading-4 text-red-600 dark:text-red-300" :title="error">
      {{ error }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type {
  ContentModerationNotificationStatus,
  ContentModerationSideEffectStatus,
} from '@/api/admin/riskControl'

const props = withDefaults(defineProps<{
  sideEffectStatus?: ContentModerationSideEffectStatus
  notificationStatus?: ContentModerationNotificationStatus
  error?: string
  moderationBanActive?: boolean
}>(), {
  sideEffectStatus: 'not_applicable',
  notificationStatus: 'not_required',
  error: '',
  moderationBanActive: false,
})

const { t } = useI18n()

const rawSideEffectStatus = computed(() => String(props.sideEffectStatus || '').trim())
const normalizedSideEffectStatus = computed<ContentModerationSideEffectStatus | null>(() => {
  const status = rawSideEffectStatus.value
  if (!status) return 'not_applicable'
  if (status === 'pending' || status === 'completed' || status === 'partial' || status === 'failed' || status === 'not_applicable') return status
  return null
})

const rawNotificationStatus = computed(() => String(props.notificationStatus || '').trim())
const normalizedNotificationStatus = computed<ContentModerationNotificationStatus | null>(() => {
  const status = rawNotificationStatus.value
  if (!status) return 'not_required'
  if (status === 'pending' || status === 'sent' || status === 'deduplicated' || status === 'failed' || status === 'not_required') return status
  return null
})

const sideEffectLabel = computed(() => normalizedSideEffectStatus.value
  ? t(`admin.riskControl.sideEffectStatus.${normalizedSideEffectStatus.value}`)
  : t('admin.riskControl.sideEffectStatus.unknown', { status: rawSideEffectStatus.value }))
const notificationLabel = computed(() => normalizedNotificationStatus.value
  ? t(`admin.riskControl.notificationStatus.${normalizedNotificationStatus.value}`)
  : t('admin.riskControl.notificationStatus.unknown', { status: rawNotificationStatus.value }))

const sideEffectClass = computed(() => {
  const status = normalizedSideEffectStatus.value
  if (!status) return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-200'
  return {
    pending: 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300',
    completed: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300',
    partial: 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300',
    failed: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300',
    not_applicable: 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400',
  }[status]
})

const notificationClass = computed(() => {
  const status = normalizedNotificationStatus.value
  if (!status) return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-200'
  return {
    pending: 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300',
    sent: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300',
    deduplicated: 'bg-sky-100 text-sky-700 dark:bg-sky-900/30 dark:text-sky-300',
    not_required: 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400',
    failed: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300',
  }[status]
})
</script>
