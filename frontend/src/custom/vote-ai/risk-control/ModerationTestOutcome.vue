<template>
  <div role="status" aria-live="polite">
    <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
      <div>
        <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.auditTestResult') }}</p>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.riskControl.auditTestHighest', { category: categoryLabel(result.highest_category), score: percent(result.highest_score) }) }}
        </p>
        <p v-if="result.scope === 'request'" class="mt-1 text-xs text-gray-500 dark:text-gray-400" data-test="audit-test-scope">
          {{ t('admin.riskControl.auditTestScopeRequest') }}
        </p>
      </div>
      <span class="inline-flex self-start rounded-md px-2 py-1 text-xs font-medium" :class="primaryStatusClass" data-test="audit-test-flagged-status">
        {{ primaryStatusLabel }}
      </span>
    </div>

    <div class="mt-3 grid gap-3" :class="hasRiskTier ? 'grid-cols-2' : 'grid-cols-1'">
      <div class="border-l-2 border-primary-400 pl-3">
        <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.auditTestRiskScore') }}</p>
        <p class="mt-1 text-sm font-semibold text-gray-900 dark:text-white" data-test="audit-test-risk-score">{{ percent(riskScore) }}</p>
      </div>
      <div v-if="hasRiskTier" class="border-l-2 pl-3" :class="riskTierBorderClass">
        <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.auditTestRiskTier') }}</p>
        <p class="mt-1 text-sm font-semibold" :class="riskTierTextClass" data-test="audit-test-risk-tier">{{ riskTierLabel }}</p>
      </div>
    </div>

    <div class="mt-3">
      <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.auditTestCategories') }}</p>
      <div class="mt-2 flex flex-wrap gap-2" data-test="audit-test-categories">
        <span v-for="category in categories" :key="category" class="inline-flex rounded-md bg-red-50 px-2 py-1 text-xs text-red-700 dark:bg-red-900/20 dark:text-red-300">
          {{ categoryLabel(category) }}
        </span>
        <span v-if="categories.length === 0" class="text-xs text-gray-400">{{ t('admin.riskControl.auditTestNone') }}</span>
      </div>
    </div>

    <div class="mt-3">
      <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.auditTestSignals') }}</p>
      <div class="mt-2 flex flex-wrap gap-2" data-test="audit-test-signals">
        <span v-for="signal in signals" :key="signal" class="inline-flex rounded-md bg-amber-50 px-2 py-1 text-xs text-amber-700 dark:bg-amber-900/20 dark:text-amber-300">
          {{ signalLabel(signal) }}
        </span>
        <span v-if="signals.length === 0" class="text-xs text-gray-400">{{ t('admin.riskControl.auditTestNone') }}</span>
      </div>
    </div>

    <div v-if="result.review_incomplete" class="mt-3 border-l-2 border-amber-400 bg-amber-50 px-3 py-2 text-xs leading-5 text-amber-800 dark:bg-amber-900/20 dark:text-amber-200" data-test="audit-review-incomplete">
      <p class="font-semibold">{{ t('admin.riskControl.auditTestReviewIncomplete') }}</p>
      <p v-if="result.review_error" class="mt-1 break-words">{{ result.review_error }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ContentModerationTestAuditResult } from '@/api/admin/riskControl'

const props = defineProps<{
  result: ContentModerationTestAuditResult
}>()

const { t } = useI18n()

const categoryTranslationKeys: Record<string, string> = {
  ai_risk: 'admin.riskControl.auditCategoryLabels.ai_risk',
  ai_current_risk: 'admin.riskControl.auditCategoryLabels.ai_current_risk',
  ai_session_risk: 'admin.riskControl.auditCategoryLabels.ai_session_risk',
  ai_actor_bonus: 'admin.riskControl.auditCategoryLabels.ai_actor_bonus',
  cyber_abuse: 'admin.riskControl.auditCategoryLabels.cyber_abuse',
  credential_theft: 'admin.riskControl.auditCategoryLabels.credential_theft',
  malware: 'admin.riskControl.auditCategoryLabels.malware',
  phishing: 'admin.riskControl.auditCategoryLabels.phishing',
  fraud: 'admin.riskControl.auditCategoryLabels.fraud',
  spam: 'admin.riskControl.auditCategoryLabels.spam',
  policy_evasion: 'admin.riskControl.auditCategoryLabels.policy_evasion',
  illicit: 'admin.riskControl.auditCategoryLabels.illicit',
  'illicit/violent': 'admin.riskControl.auditCategoryLabels.illicit_violent',
  hate: 'admin.riskControl.auditCategoryLabels.hate',
  'hate/threatening': 'admin.riskControl.auditCategoryLabels.hate_threatening',
  harassment: 'admin.riskControl.auditCategoryLabels.harassment',
  'harassment/threatening': 'admin.riskControl.auditCategoryLabels.harassment_threatening',
  sexual: 'admin.riskControl.auditCategoryLabels.sexual',
  'sexual/minors': 'admin.riskControl.auditCategoryLabels.sexual_minors',
  sexual_minors: 'admin.riskControl.auditCategoryLabels.sexual_minors',
  violence: 'admin.riskControl.auditCategoryLabels.violence',
  'violence/graphic': 'admin.riskControl.auditCategoryLabels.violence_graphic',
  self_harm: 'admin.riskControl.auditCategoryLabels.self_harm',
  'self-harm': 'admin.riskControl.auditCategoryLabels.self_harm',
  'self-harm/intent': 'admin.riskControl.auditCategoryLabels.self_harm_intent',
  'self-harm/instructions': 'admin.riskControl.auditCategoryLabels.self_harm_instructions',
  other: 'admin.riskControl.auditCategoryLabels.other',
}

const signalTranslationKeys: Record<string, string> = {
  defensive_context: 'admin.riskControl.auditSignalLabels.defensive_context',
  ownership_unverified: 'admin.riskControl.auditSignalLabels.ownership_unverified',
  credential_access: 'admin.riskControl.auditSignalLabels.credential_access',
  auth_bypass: 'admin.riskControl.auditSignalLabels.auth_bypass',
  secret_extraction: 'admin.riskControl.auditSignalLabels.secret_extraction',
  malware_delivery: 'admin.riskControl.auditSignalLabels.malware_delivery',
  policy_evasion: 'admin.riskControl.auditSignalLabels.policy_evasion',
  progressive_escalation: 'admin.riskControl.auditSignalLabels.progressive_escalation',
}

const categories = computed(() => Array.from(new Set((props.result.categories || []).filter(Boolean))))
const signals = computed(() => Array.from(new Set((props.result.signals || []).filter(Boolean))))
const riskScore = computed(() => Number.isFinite(props.result.risk_score) ? props.result.risk_score : props.result.composite_score)
const rawRiskTier = computed(() => String(props.result.risk_tier || '').trim().toLowerCase())
const normalizedRiskTier = computed<'low' | 'observe' | 'high' | null>(() => {
  const tier = rawRiskTier.value
  return tier === 'low' || tier === 'observe' || tier === 'high' ? tier : null
})
const hasRiskTier = computed(() => rawRiskTier.value !== '')
const riskTierLabel = computed(() => normalizedRiskTier.value
  ? t(`admin.riskControl.auditRiskTier.${normalizedRiskTier.value}`)
  : t('admin.riskControl.auditRiskTier.unknown', { tier: rawRiskTier.value }))

const primaryStatusLabel = computed(() => normalizedRiskTier.value
  ? t(`admin.riskControl.auditRiskTier.${normalizedRiskTier.value}`)
  : props.result.flagged
    ? t('admin.riskControl.auditTestFlagged')
    : hasRiskTier.value
      ? t('admin.riskControl.auditRiskTier.unknown', { tier: rawRiskTier.value })
      : t('admin.riskControl.auditTestPassed'))

const primaryStatusClass = computed(() => {
  if (normalizedRiskTier.value === 'high') return 'bg-red-50 text-red-700 dark:bg-red-900/20 dark:text-red-300'
  if (normalizedRiskTier.value === 'observe') return 'bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300'
  if (normalizedRiskTier.value === 'low') return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300'
  if (hasRiskTier.value) return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-200'
  return props.result.flagged
    ? 'bg-red-50 text-red-700 dark:bg-red-900/20 dark:text-red-300'
    : 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300'
})

const riskTierBorderClass = computed(() => {
  if (normalizedRiskTier.value === 'high') return 'border-red-400'
  if (normalizedRiskTier.value === 'observe') return 'border-amber-400'
  if (normalizedRiskTier.value === 'low') return 'border-emerald-400'
  return 'border-gray-400'
})

const riskTierTextClass = computed(() => {
  if (normalizedRiskTier.value === 'high') return 'text-red-700 dark:text-red-300'
  if (normalizedRiskTier.value === 'observe') return 'text-amber-700 dark:text-amber-300'
  if (normalizedRiskTier.value === 'low') return 'text-emerald-700 dark:text-emerald-300'
  return 'text-gray-700 dark:text-gray-200'
})

function percent(value: number): string {
  if (!Number.isFinite(value)) return '-'
  return `${(value * 100).toFixed(1)}%`
}

function categoryLabel(category?: string): string {
  const code = String(category || '').trim()
  if (!code) return '-'
  const key = categoryTranslationKeys[code]
  return key ? `${t(key)} (${code})` : code
}

function signalLabel(signal: string): string {
  const key = signalTranslationKeys[signal]
  return key ? `${t(key)} (${signal})` : signal
}
</script>
