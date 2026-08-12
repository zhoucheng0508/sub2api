<template>
  <section
    v-if="stageRows.length > 0"
    class="border-t border-gray-100 pt-4 dark:border-dark-700"
    data-test="audit-stage-details"
  >
    <p class="text-sm font-semibold text-gray-900 dark:text-white">
      {{ t('admin.riskControl.auditStageDetailsTitle') }}
    </p>
    <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">
      {{ t('admin.riskControl.auditStageDetailsHint') }}
    </p>

    <div class="mt-3 overflow-x-auto">
      <table class="min-w-[920px] table-fixed text-left text-xs">
        <thead class="border-b border-gray-100 text-gray-500 dark:border-dark-700 dark:text-gray-400">
          <tr>
            <th scope="col" class="w-28 px-2 py-2 font-medium">{{ t('admin.riskControl.auditStageColumn') }}</th>
            <th scope="col" class="w-24 px-2 py-2 font-medium">{{ t('admin.riskControl.auditStageOutcome') }}</th>
            <th scope="col" class="w-24 px-2 py-2 font-medium">{{ t('admin.riskControl.auditStageProviderCalled') }}</th>
            <th scope="col" class="w-28 px-2 py-2 font-medium">{{ t('admin.riskControl.auditStageCache') }}</th>
            <th scope="col" class="w-32 px-2 py-2 font-medium">{{ t('admin.riskControl.auditStageUsage') }}</th>
            <th scope="col" class="w-24 px-2 py-2 font-medium">{{ t('admin.riskControl.auditStageInputChars') }}</th>
            <th scope="col" class="w-24 px-2 py-2 font-medium">{{ t('admin.riskControl.auditStageLatency') }}</th>
            <th scope="col" class="px-2 py-2 font-medium">{{ t('admin.riskControl.auditStageTokens') }}</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-100 text-gray-800 dark:divide-dark-700 dark:text-gray-200">
          <tr
            v-for="row in stageRows"
            :key="row.key"
            :data-test="`audit-stage-${row.stageKey}`"
          >
            <td class="px-2 py-2.5 font-semibold text-gray-900 dark:text-white">{{ row.stage }}</td>
            <td class="px-2 py-2.5">{{ row.outcome }}</td>
            <td class="px-2 py-2.5">{{ row.providerCalled }}</td>
            <td class="px-2 py-2.5">{{ row.resultCache }}</td>
            <td class="px-2 py-2.5">{{ row.usage }}</td>
            <td class="px-2 py-2.5 tabular-nums">{{ row.inputChars }}</td>
            <td class="px-2 py-2.5 tabular-nums">{{ row.latency }}</td>
            <td class="break-words px-2 py-2.5">{{ row.tokens }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import type { ContentModerationAuditStageDetails } from '@/api/admin/riskControl'

const props = defineProps<{
  stages?: ContentModerationAuditStageDetails[]
}>()

const { t } = useI18n()

const stageRows = computed(() => (props.stages ?? []).map((stage, index) => ({
  key: `${stage.stage || 'unknown'}-${index}`,
  stageKey: stage.stage || `unknown-${index}`,
  stage: stageLabel(stage.stage),
  outcome: stage.failed
    ? t('admin.riskControl.auditStageFailed')
    : t('admin.riskControl.auditStageSucceeded'),
  providerCalled: booleanText(stage.provider_called),
  resultCache: typeof stage.result_cache_hit !== 'boolean'
    ? t('common.unknown')
    : stage.result_cache_hit
      ? t('admin.riskControl.cacheHit')
      : t('admin.riskControl.cacheMiss'),
  usage: stageUsageStatus(stage),
  inputChars: formatStageCounter(stage.input_chars),
  latency: formatStageCounter(stage.latency_ms),
  tokens: stageTokenText(stage),
})))

function stageLabel(stage: string): string {
  const normalized = String(stage || '').trim()
  if (normalized === 'fast' || normalized === 'full' || normalized === 'max') {
    return t(`admin.riskControl.aiRuntimeStage.${normalized}`)
  }
  return normalized || t('common.unknown')
}

function booleanText(value: boolean | undefined): string {
  if (typeof value !== 'boolean') return t('common.unknown')
  return value ? t('common.yes') : t('common.no')
}

function stageUsageStatus(stage: ContentModerationAuditStageDetails): string {
  if (!stage.provider_called) return t('admin.riskControl.auditStageNotApplicable')
  return stage.usage_known
    ? t('admin.riskControl.usageComplete')
    : t('admin.riskControl.usageUnknown')
}

function stageTokenText(stage: ContentModerationAuditStageDetails): string {
  if (!stage.provider_called) return t('admin.riskControl.auditStageNoProviderUsage')
  if (!stage.usage_known) return t('admin.riskControl.usageUnknown')

  const prompt = knownCount(stage.prompt_tokens)
  const cached = knownCount(stage.cached_input_tokens)
  const uncached = knownCount(stage.uncached_input_tokens)
  const output = knownCount(stage.output_tokens)
  if (prompt === null || cached === null || uncached === null || output === null || prompt !== cached + uncached) {
    return t('common.unknown')
  }
  return t('admin.riskControl.auditTokenSummary', {
    prompt: formatNumber(prompt),
    cached: formatNumber(cached),
    uncached: formatNumber(uncached),
    output: formatNumber(output),
  })
}

function knownCount(value: number | undefined): number | null {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0
    ? Math.floor(value)
    : null
}

function formatStageCounter(value: number | undefined): string {
  if (value === undefined) return t('common.unknown')
  const normalized = knownCount(value)
  return normalized === null ? t('common.unknown') : formatNumber(normalized)
}

function formatNumber(value: number): string {
  return new Intl.NumberFormat().format(value)
}
</script>
