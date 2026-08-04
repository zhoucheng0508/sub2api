<template>
  <BaseDialog
    :show="show"
    :title="t('admin.riskControl.unbanDialogTitle')"
    width="normal"
    :close-on-escape="!loading"
    :show-close-button="!loading"
    @close="handleClose"
  >
    <div v-if="row" class="space-y-4">
      <div class="rounded-md border border-gray-100 bg-gray-50 px-4 py-3 dark:border-dark-700 dark:bg-dark-800/70">
        <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ row.user_email || t('admin.riskControl.unknownUser') }}</p>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">UID {{ row.user_id }}</p>
      </div>

      <div
        v-if="warning"
        class="rounded-md border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-900/60 dark:bg-amber-900/20 dark:text-amber-200"
        data-test="unban-partial-warning"
      >
        <p class="font-semibold">{{ t('admin.riskControl.unbanPartialSuccessTitle') }}</p>
        <p class="mt-1 break-words text-xs leading-5">{{ warning }}</p>
      </div>

      <template v-else>
        <div
          class="rounded-md border px-4 py-3 text-sm"
          :class="canRestore
            ? 'border-emerald-200 bg-emerald-50 text-emerald-800 dark:border-emerald-900/60 dark:bg-emerald-900/20 dark:text-emerald-200'
            : 'border-amber-200 bg-amber-50 text-amber-800 dark:border-amber-900/60 dark:bg-amber-900/20 dark:text-amber-200'"
          data-test="unban-ownership-status"
        >
          {{ ownershipMessage }}
        </div>

        <fieldset :disabled="!canRestore || loading" class="space-y-2">
          <legend class="mb-2 text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.unbanModeLabel') }}</legend>
          <label
            v-for="option in options"
            :key="option.value"
            class="flex cursor-pointer items-start gap-3 rounded-md border p-3 transition-colors"
            :class="mode === option.value
              ? 'border-primary-300 bg-primary-50 dark:border-primary-700 dark:bg-primary-900/20'
              : 'border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800'"
          >
            <input
              v-model="mode"
              type="radio"
              name="moderation-unban-mode"
              :value="option.value"
              class="mt-1 h-4 w-4 border-gray-300 text-primary-600 focus:ring-primary-500"
              :data-test="`unban-mode-${option.value}`"
            />
            <span class="min-w-0">
              <span class="block text-sm font-medium text-gray-900 dark:text-white">{{ option.label }}</span>
              <span class="mt-1 block text-xs leading-5 text-gray-500 dark:text-gray-400">{{ option.description }}</span>
            </span>
          </label>
        </fieldset>
      </template>
    </div>

    <template #footer>
      <div class="flex justify-end gap-2">
        <button type="button" class="btn btn-secondary" :disabled="loading" @click="handleClose">
          {{ warning ? t('common.close') : t('common.cancel') }}
        </button>
        <button
          v-if="warning"
          type="button"
          class="btn btn-primary"
          :disabled="loading"
          data-test="retry-moderation-risk-clear"
          @click="emit('confirm', 'clear_risk_only')"
        >
          {{ loading ? t('common.processing') : t('admin.riskControl.retryRiskStateCleanup') }}
        </button>
        <button
          v-else
          type="button"
          class="btn btn-primary"
          :disabled="!canRestore || loading"
          data-test="confirm-moderation-unban"
          @click="emit('confirm', mode)"
        >
          {{ loading ? t('common.processing') : t('admin.riskControl.confirmUnban') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import type { ContentModerationLog, ContentModerationUnbanMode } from '@/api/admin/riskControl'

const props = withDefaults(defineProps<{
  show: boolean
  row: ContentModerationLog | null
  loading?: boolean
  warning?: string
}>(), {
  loading: false,
  warning: '',
})

const emit = defineEmits<{
  close: []
  confirm: [mode: ContentModerationUnbanMode]
}>()

const { t } = useI18n()
const mode = ref<ContentModerationUnbanMode>('restore_and_clear_risk')

watch(() => [props.show, props.row?.user_id], ([show]) => {
  if (show) mode.value = 'restore_and_clear_risk'
})

const canRestore = computed(() => Boolean(
  props.row?.moderation_ban_active && props.row.user_id && props.row.user_status === 'disabled'
))

const ownershipMessage = computed(() => {
  if (canRestore.value) return t('admin.riskControl.unbanModerationOwned')
  return props.row?.unban_block_reason || t('admin.riskControl.unbanNotModerationOwned')
})

const options = computed(() => [
  {
    value: 'restore_and_clear_risk' as const,
    label: t('admin.riskControl.unbanModeClearRisk'),
    description: t('admin.riskControl.unbanModeClearRiskHint'),
  },
  {
    value: 'restore_only' as const,
    label: t('admin.riskControl.unbanModeRestoreOnly'),
    description: t('admin.riskControl.unbanModeRestoreOnlyHint'),
  },
])

function handleClose() {
  if (!props.loading) emit('close')
}
</script>
