<template>
  <details
    class="overflow-hidden rounded-lg border border-gray-100 bg-gray-50/60 dark:border-dark-700 dark:bg-dark-900/20"
    data-test="incremental-audit-settings"
  >
    <summary class="flex cursor-pointer list-none items-center justify-between gap-4 px-4 py-3 marker:hidden">
      <div class="min-w-0">
        <p class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('admin.riskControl.aiIncrementalAuditSettings') }}
        </p>
        <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">
          {{ t('admin.riskControl.aiIncrementalAuditSettingsHint') }}
        </p>
      </div>
      <span
        class="inline-flex flex-shrink-0 rounded-md px-2 py-1 text-xs font-medium"
        :class="incrementalAuditEnabled
          ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300'
          : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-300'"
        data-test="incremental-audit-status"
      >
        {{ incrementalAuditEnabled ? t('common.enabled') : t('common.disabled') }}
      </span>
    </summary>

    <div class="space-y-4 border-t border-gray-100 p-4 dark:border-dark-700">
      <div class="flex items-center justify-between gap-4 rounded-lg border border-gray-100 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
        <div class="min-w-0 pr-4">
          <p class="text-sm font-medium text-gray-900 dark:text-white">
            {{ t('admin.riskControl.aiIncrementalAuditEnabled') }}
          </p>
          <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">
            {{ t('admin.riskControl.aiIncrementalAuditEnabledHint') }}
          </p>
        </div>
        <Toggle
          :model-value="incrementalAuditEnabled"
          :aria-label="t('admin.riskControl.aiIncrementalAuditEnabled')"
          data-test="ai-incremental-audit-enabled"
          @update:model-value="emit('update:incrementalAuditEnabled', $event)"
        />
      </div>

      <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
        <div class="flex items-center justify-between gap-4 rounded-lg border border-gray-100 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
          <div class="min-w-0 pr-4">
            <p class="text-sm font-medium text-gray-900 dark:text-white">
              {{ t('admin.riskControl.aiInputProvenanceV2Enabled') }}
            </p>
            <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">
              {{ t('admin.riskControl.aiInputProvenanceV2EnabledHint') }}
            </p>
          </div>
          <Toggle
            :model-value="inputProvenanceV2Enabled"
            :aria-label="t('admin.riskControl.aiInputProvenanceV2Enabled')"
            :aria-disabled="provenanceToggleLocked"
            :disabled="provenanceToggleLocked"
            :class="{ 'cursor-not-allowed opacity-60': provenanceToggleLocked }"
            data-test="ai-input-provenance-v2-enabled"
            @update:model-value="updateInputProvenance"
          />
        </div>

        <div class="flex items-center justify-between gap-4 rounded-lg border border-gray-100 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
          <div class="min-w-0 pr-4">
            <p class="text-sm font-medium text-gray-900 dark:text-white">
              {{ t('admin.riskControl.aiDeterministicRiskV2Enabled') }}
            </p>
            <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">
              {{ t('admin.riskControl.aiDeterministicRiskV2EnabledHint') }}
            </p>
          </div>
          <Toggle
            :model-value="deterministicRiskV2Enabled"
            :aria-label="t('admin.riskControl.aiDeterministicRiskV2Enabled')"
            data-test="ai-deterministic-risk-v2-enabled"
            @update:model-value="emit('update:deterministicRiskV2Enabled', $event)"
          />
        </div>
      </div>

      <p
        v-if="incrementalAuditEnabled && !inputProvenanceV2Enabled"
        class="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-xs leading-5 text-red-700 dark:border-red-900/60 dark:bg-red-900/20 dark:text-red-300"
        data-test="incremental-provenance-warning"
      >
        {{ t('admin.riskControl.aiIncrementalRequiresProvenance') }}
      </p>
      <p
        v-else-if="provenanceToggleLocked"
        class="text-xs leading-5 text-gray-500 dark:text-gray-400"
      >
        {{ t('admin.riskControl.aiInputProvenanceRequiredHint') }}
      </p>

      <div class="rounded-lg border border-sky-100 bg-sky-50/70 px-4 py-3 text-xs leading-5 text-sky-800 dark:border-sky-900/50 dark:bg-sky-900/20 dark:text-sky-200">
        <p>{{ t('admin.riskControl.aiIncrementalCoverageNotice') }}</p>
        <p class="mt-1">{{ t('admin.riskControl.aiIncrementalSessionNotice') }}</p>
        <p class="mt-1">{{ t('admin.riskControl.aiIncrementalPrivacyNotice') }}</p>
      </div>

      <section
        class="border-y border-gray-100 py-4 dark:border-dark-700"
        data-test="audit-runtime-metrics"
      >
        <div class="mb-3">
          <h4 class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('admin.riskControl.aiRuntimeMetricsTitle') }}
          </h4>
          <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">
            {{ t('admin.riskControl.aiRuntimeMetricsHint') }}
          </p>
        </div>
        <dl class="grid grid-cols-2 gap-x-4 gap-y-3 md:grid-cols-4">
          <div class="min-w-0">
            <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.aiRuntimeFastCalls') }}</dt>
            <dd class="mt-1 text-sm font-semibold text-gray-900 dark:text-white" data-test="audit-fast-calls">
              {{ formatCounter(runtimeStatus?.audit_fast_calls) }}
            </dd>
          </div>
          <div class="min-w-0">
            <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.aiRuntimeFullCalls') }}</dt>
            <dd class="mt-1 text-sm font-semibold text-gray-900 dark:text-white" data-test="audit-full-calls">
              {{ formatCounter(runtimeStatus?.audit_full_calls) }}
            </dd>
          </div>
          <div class="min-w-0">
            <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.aiRuntimeMaxCalls') }}</dt>
            <dd class="mt-1 text-sm font-semibold text-gray-900 dark:text-white" data-test="audit-max-calls">
              {{ formatCounter(runtimeStatus?.audit_max_calls) }}
            </dd>
          </div>
          <div class="min-w-0">
            <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.aiRuntimeResultCacheHits') }}</dt>
            <dd class="mt-1 text-sm font-semibold text-gray-900 dark:text-white" data-test="audit-result-cache-hits">
              {{ formatCounter(runtimeStatus?.audit_result_cache_hits) }}
            </dd>
          </div>
          <div class="min-w-0">
            <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.aiRuntimeResultCacheRate') }}</dt>
            <dd class="mt-1 text-sm font-semibold text-gray-900 dark:text-white" data-test="audit-result-cache-rate">
              {{ resultCacheRate }}
            </dd>
          </div>
          <div class="min-w-0">
            <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.aiRuntimePromptTokens') }}</dt>
            <dd class="mt-1 text-sm font-semibold text-gray-900 dark:text-white" data-test="audit-prompt-tokens">
              {{ formatCounter(runtimeStatus?.audit_prompt_tokens) }}
            </dd>
          </div>
          <div class="min-w-0">
            <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.aiRuntimeCachedInputTokens') }}</dt>
            <dd class="mt-1 text-sm font-semibold text-gray-900 dark:text-white" data-test="audit-cached-input-tokens">
              {{ formatCounter(runtimeStatus?.audit_cached_input_tokens) }}
            </dd>
          </div>
          <div class="min-w-0">
            <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.aiRuntimeUncachedInputTokens') }}</dt>
            <dd class="mt-1 text-sm font-semibold text-gray-900 dark:text-white" data-test="audit-uncached-input-tokens">
              {{ formatCounter(runtimeStatus?.audit_uncached_input_tokens) }}
            </dd>
          </div>
          <div class="min-w-0">
            <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.aiRuntimeTokenCacheRate') }}</dt>
            <dd class="mt-1 text-sm font-semibold text-gray-900 dark:text-white" data-test="audit-token-cache-rate">
              {{ tokenCacheRate }}
            </dd>
          </div>
          <div class="min-w-0">
            <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.aiRuntimeOutputTokens') }}</dt>
            <dd class="mt-1 text-sm font-semibold text-gray-900 dark:text-white" data-test="audit-output-tokens">
              {{ formatCounter(runtimeStatus?.audit_output_tokens) }}
            </dd>
          </div>
          <div class="min-w-0">
            <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.aiRuntimeInputChars') }}</dt>
            <dd class="mt-1 text-sm font-semibold text-gray-900 dark:text-white" data-test="audit-input-chars">
              {{ formatCounter(runtimeStatus?.audit_input_chars) }}
            </dd>
          </div>
          <div class="min-w-0">
            <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.aiRuntimeUsageUnknown') }}</dt>
            <dd class="mt-1 text-sm font-semibold text-gray-900 dark:text-white" data-test="audit-usage-unknown">
              {{ formatCounter(runtimeStatus?.audit_usage_unknown) }}
            </dd>
          </div>
          <div class="min-w-0">
            <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.aiRuntimeEstimatedCostUSD') }}</dt>
            <dd class="mt-1 text-sm font-semibold text-gray-900 dark:text-white" data-test="audit-estimated-cost-usd">
              {{ formatUSD(runtimeStatus?.audit_estimated_cost_usd) }}
            </dd>
          </div>
          <div class="min-w-0">
            <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.aiRuntimeBusinessCostUSD') }}</dt>
            <dd class="mt-1 text-sm font-semibold text-gray-900 dark:text-white" data-test="business-actual-cost-usd">
              {{ formatUSD(runtimeStatus?.business_actual_cost_usd) }}
            </dd>
          </div>
          <div class="min-w-0">
            <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.aiRuntimeCostPerBusinessUSD') }}</dt>
            <dd class="mt-1 text-sm font-semibold text-gray-900 dark:text-white" data-test="audit-cost-per-business-usd">
              {{ formatUSD(runtimeStatus?.audit_cost_per_business_usd) }}
            </dd>
          </div>
          <div class="min-w-0">
            <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.aiRuntimeCostCoverageLabel') }}</dt>
            <dd class="mt-1 text-sm font-semibold text-gray-900 dark:text-white" data-test="audit-cost-coverage">
              {{ costCoverageLabel }}
            </dd>
          </div>
        </dl>

        <p class="mt-3 text-xs leading-5 text-gray-500 dark:text-gray-400" data-test="audit-usage-coverage">
          {{ t('admin.riskControl.aiRuntimeUsageCoverage', {
            complete: formatCounter(completeUsageSamples),
            unknown: formatCounter(runtimeStatus?.audit_usage_unknown),
          }) }}
        </p>
        <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400" data-test="audit-cost-samples">
          {{ t('admin.riskControl.aiRuntimeCostSamples', {
            priced: formatCounter(runtimeStatus?.audit_cost_priced_samples),
            unpriced: formatCounter(runtimeStatus?.audit_cost_unpriced_samples),
            unknown: formatCounter(runtimeStatus?.audit_usage_unknown),
          }) }}
        </p>
        <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">
          {{ t('admin.riskControl.aiRuntimeCostUnitHint') }}
        </p>

        <div class="mt-4 grid grid-cols-1 gap-4 border-t border-gray-100 pt-4 md:grid-cols-3 dark:border-dark-700">
          <section class="min-w-0" data-test="audit-stage-latency">
            <h5 class="text-xs font-semibold text-gray-700 dark:text-gray-200">{{ t('admin.riskControl.aiRuntimeStageLatency') }}</h5>
            <p v-if="stageLatencyItems.length === 0" class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ t('common.unknown') }}</p>
            <dl v-else class="mt-2 space-y-2">
              <div v-for="item in stageLatencyItems" :key="item.key" class="text-xs">
                <dt class="text-gray-500 dark:text-gray-400">{{ item.label }}</dt>
                <dd class="mt-0.5 font-medium text-gray-900 dark:text-white">
                  {{ t('admin.riskControl.aiRuntimeLatencySummary', { average: item.average, p95: item.p95, count: item.count }) }}
                </dd>
              </div>
            </dl>
          </section>

          <section class="min-w-0" data-test="audit-session-sources">
            <h5 class="text-xs font-semibold text-gray-700 dark:text-gray-200">{{ t('admin.riskControl.aiRuntimeSessionSources') }}</h5>
            <p v-if="sessionSourceItems.length === 0" class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ t('common.unknown') }}</p>
            <dl v-else class="mt-2 space-y-1.5 text-xs">
              <div v-for="item in sessionSourceItems" :key="item.key" class="flex items-center justify-between gap-3">
                <dt class="truncate text-gray-500 dark:text-gray-400">{{ item.label }}</dt>
                <dd class="font-medium text-gray-900 dark:text-white">{{ item.count }}</dd>
              </div>
            </dl>
          </section>

          <section class="min-w-0" data-test="audit-prefix-continuity">
            <h5 class="text-xs font-semibold text-gray-700 dark:text-gray-200">{{ t('admin.riskControl.aiRuntimePrefixContinuity') }}</h5>
            <p v-if="prefixContinuityItems.length === 0" class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ t('common.unknown') }}</p>
            <dl v-else class="mt-2 space-y-1.5 text-xs">
              <div v-for="item in prefixContinuityItems" :key="item.key" class="flex items-center justify-between gap-3">
                <dt class="truncate text-gray-500 dark:text-gray-400">{{ item.label }}</dt>
                <dd class="font-medium text-gray-900 dark:text-white">{{ item.count }}</dd>
              </div>
            </dl>
          </section>
        </div>
      </section>

      <section class="space-y-4 border-b border-gray-100 pb-4 dark:border-dark-700" data-test="audit-pricing-settings">
        <div class="flex items-center justify-between gap-4">
          <div class="min-w-0 pr-4">
            <h4 class="text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('admin.riskControl.aiPricingTitle') }}
            </h4>
            <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">
              {{ t('admin.riskControl.aiPricingHint') }}
            </p>
          </div>
          <Toggle
            :model-value="pricingConfigured"
            :aria-label="t('admin.riskControl.aiPricingEnabled')"
            data-test="ai-pricing-configured"
            @update:model-value="emit('update:pricingConfigured', $event)"
          />
        </div>

        <fieldset :disabled="!pricingConfigured" :class="{ 'opacity-50': !pricingConfigured }">
          <div class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
            <div>
              <label for="vote-ai-pricing-version" class="input-label">{{ t('admin.riskControl.aiPricingVersion') }}</label>
              <input
                id="vote-ai-pricing-version"
                :value="pricingVersion"
                type="text"
                maxlength="100"
                class="input"
                data-test="ai-pricing-version"
                @input="emitText('update:pricingVersion', $event)"
              />
            </div>
            <div>
              <label for="vote-ai-uncached-input-price" class="input-label">{{ t('admin.riskControl.aiUncachedInputPrice') }}</label>
              <input
                id="vote-ai-uncached-input-price"
                :value="uncachedInputUsdPerMillionTokens ?? ''"
                type="number"
                min="0"
                max="1000000"
                step="0.000001"
                class="input"
                data-test="ai-uncached-input-price"
                @input="emitNullableNumber('update:uncachedInputUsdPerMillionTokens', $event)"
              />
            </div>
            <div>
              <label for="vote-ai-cached-input-price" class="input-label">{{ t('admin.riskControl.aiCachedInputPrice') }}</label>
              <input
                id="vote-ai-cached-input-price"
                :value="cachedInputUsdPerMillionTokens ?? ''"
                type="number"
                min="0"
                max="1000000"
                step="0.000001"
                class="input"
                data-test="ai-cached-input-price"
                @input="emitNullableNumber('update:cachedInputUsdPerMillionTokens', $event)"
              />
            </div>
            <div>
              <label for="vote-ai-output-price" class="input-label">{{ t('admin.riskControl.aiOutputPrice') }}</label>
              <input
                id="vote-ai-output-price"
                :value="outputUsdPerMillionTokens ?? ''"
                type="number"
                min="0"
                max="1000000"
                step="0.000001"
                class="input"
                data-test="ai-output-price"
                @input="emitNullableNumber('update:outputUsdPerMillionTokens', $event)"
              />
            </div>
          </div>
          <p class="mt-2 text-xs leading-5 text-gray-500 dark:text-gray-400">
            {{ t('admin.riskControl.aiPricingUnitHint') }}
          </p>
        </fieldset>
      </section>

      <fieldset :disabled="!incrementalAuditEnabled" :class="{ 'opacity-50': !incrementalAuditEnabled }">
        <div class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
          <div>
            <label for="vote-ai-recent-user-turns" class="input-label">{{ t('admin.riskControl.aiRecentUserTurns') }}</label>
            <input
              id="vote-ai-recent-user-turns"
              :value="recentUserTurns"
              type="number"
              min="1"
              max="8"
              step="1"
              class="input"
              data-test="ai-recent-user-turns"
              @input="emitNumber('update:recentUserTurns', $event)"
            />
            <p class="mt-2 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.aiRecentUserTurnsHint') }}</p>
          </div>

          <div>
            <label for="vote-ai-summary-max-chars" class="input-label">{{ t('admin.riskControl.aiSummaryMaxChars') }}</label>
            <input
              id="vote-ai-summary-max-chars"
              :value="summaryMaxChars"
              type="number"
              min="1"
              max="4000"
              step="1"
              class="input"
              data-test="ai-summary-max-chars"
              @input="emitNumber('update:summaryMaxChars', $event)"
            />
            <p class="mt-2 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.aiSummaryMaxCharsHint') }}</p>
          </div>

          <div>
            <label for="vote-ai-audit-context-ttl" class="input-label">{{ t('admin.riskControl.aiAuditContextTTL') }}</label>
            <input
              id="vote-ai-audit-context-ttl"
              :value="auditContextTtlMinutes"
              type="number"
              min="1"
              max="1440"
              step="1"
              class="input"
              data-test="ai-audit-context-ttl-minutes"
              @input="emitNumber('update:auditContextTtlMinutes', $event)"
            />
            <p class="mt-2 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.aiAuditContextTTLHint') }}</p>
          </div>

          <div>
            <label for="vote-ai-full-review-threshold" class="input-label">{{ t('admin.riskControl.aiFullReviewThreshold') }}</label>
            <input
              id="vote-ai-full-review-threshold"
              :value="fullReviewThreshold"
              type="number"
              min="0.01"
              :max="Math.max(0.01, blockThreshold - 0.01)"
              step="0.01"
              class="input"
              data-test="ai-full-review-threshold"
              @input="emitNumber('update:fullReviewThreshold', $event)"
            />
            <p class="mt-2 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.aiFullReviewThresholdHint') }}</p>
          </div>

          <div>
            <label for="vote-ai-full-review-risk-delta" class="input-label">{{ t('admin.riskControl.aiFullReviewRiskDelta') }}</label>
            <input
              id="vote-ai-full-review-risk-delta"
              :value="fullReviewRiskDelta"
              type="number"
              min="0.01"
              max="1"
              step="0.01"
              class="input"
              data-test="ai-full-review-risk-delta"
              @input="emitNumber('update:fullReviewRiskDelta', $event)"
            />
            <p class="mt-2 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.aiFullReviewRiskDeltaHint') }}</p>
          </div>

          <div>
            <label for="vote-ai-periodic-full-review-turns" class="input-label">{{ t('admin.riskControl.aiPeriodicFullReviewTurns') }}</label>
            <input
              id="vote-ai-periodic-full-review-turns"
              :value="periodicFullReviewTurns"
              type="number"
              min="1"
              max="100"
              step="1"
              class="input"
              data-test="ai-periodic-full-review-turns"
              @input="emitNumber('update:periodicFullReviewTurns', $event)"
            />
            <p class="mt-2 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.aiPeriodicFullReviewTurnsHint') }}</p>
          </div>

          <div>
            <label for="vote-ai-full-review-max-input" class="input-label">{{ t('admin.riskControl.aiFullReviewMaxInputChars') }}</label>
            <input
              id="vote-ai-full-review-max-input"
              :value="fullReviewMaxInputChars"
              type="number"
              min="1000"
              :max="maxInputChars"
              step="1000"
              class="input"
              data-test="ai-full-review-max-input-chars"
              @input="emitNumber('update:fullReviewMaxInputChars', $event)"
            />
            <p class="mt-2 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.aiFullReviewMaxInputCharsHint') }}</p>
          </div>

          <div>
            <label for="vote-ai-fast-output-tokens" class="input-label">{{ t('admin.riskControl.aiFastMaxOutputTokens') }}</label>
            <input
              id="vote-ai-fast-output-tokens"
              :value="fastMaxOutputTokens"
              type="number"
              min="1"
              max="8192"
              step="1"
              class="input"
              data-test="ai-fast-max-output-tokens"
              @input="emitNumber('update:fastMaxOutputTokens', $event)"
            />
          </div>

          <div>
            <label for="vote-ai-full-output-tokens" class="input-label">{{ t('admin.riskControl.aiFullMaxOutputTokens') }}</label>
            <input
              id="vote-ai-full-output-tokens"
              :value="fullMaxOutputTokens"
              type="number"
              min="1"
              max="8192"
              step="1"
              class="input"
              data-test="ai-full-max-output-tokens"
              @input="emitNumber('update:fullMaxOutputTokens', $event)"
            />
          </div>

          <div>
            <label for="vote-ai-max-review-output-tokens" class="input-label">{{ t('admin.riskControl.aiMaxReviewMaxOutputTokens') }}</label>
            <input
              id="vote-ai-max-review-output-tokens"
              :value="maxReviewMaxOutputTokens"
              type="number"
              min="1"
              max="8192"
              step="1"
              class="input"
              data-test="ai-max-review-max-output-tokens"
              @input="emitNumber('update:maxReviewMaxOutputTokens', $event)"
            />
            <p class="mt-2 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.aiOutputTokenLimitsHint') }}</p>
          </div>
        </div>
      </fieldset>
    </div>
  </details>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import Toggle from '@/components/common/Toggle.vue'
import type { ContentModerationRuntimeStatus } from '@/api/admin/riskControl'

type NumericUpdateEvent =
  | 'update:recentUserTurns'
  | 'update:summaryMaxChars'
  | 'update:fullReviewThreshold'
  | 'update:fullReviewRiskDelta'
  | 'update:periodicFullReviewTurns'
  | 'update:fullReviewMaxInputChars'
  | 'update:fastMaxOutputTokens'
  | 'update:fullMaxOutputTokens'
  | 'update:maxReviewMaxOutputTokens'
  | 'update:auditContextTtlMinutes'

type PricingNumericUpdateEvent =
  | 'update:uncachedInputUsdPerMillionTokens'
  | 'update:cachedInputUsdPerMillionTokens'
  | 'update:outputUsdPerMillionTokens'

const props = defineProps<{
  incrementalAuditEnabled: boolean
  inputProvenanceV2Enabled: boolean
  deterministicRiskV2Enabled: boolean
  recentUserTurns: number
  summaryMaxChars: number
  fullReviewThreshold: number
  fullReviewRiskDelta: number
  periodicFullReviewTurns: number
  fullReviewMaxInputChars: number
  fastMaxOutputTokens: number
  fullMaxOutputTokens: number
  maxReviewMaxOutputTokens: number
  auditContextTtlMinutes: number
  pricingConfigured: boolean
  pricingVersion: string
  uncachedInputUsdPerMillionTokens: number | null
  cachedInputUsdPerMillionTokens: number | null
  outputUsdPerMillionTokens: number | null
  maxInputChars: number
  blockThreshold: number
  runtimeStatus?: ContentModerationRuntimeStatus | null
}>()

const emit = defineEmits<{
  (event: 'update:incrementalAuditEnabled', value: boolean): void
  (event: 'update:inputProvenanceV2Enabled', value: boolean): void
  (event: 'update:deterministicRiskV2Enabled', value: boolean): void
  (event: 'update:pricingConfigured', value: boolean): void
  (event: 'update:pricingVersion', value: string): void
  (event: PricingNumericUpdateEvent, value: number | null): void
  (event: NumericUpdateEvent, value: number): void
}>()

const { t } = useI18n()

const provenanceToggleLocked = computed(() => props.incrementalAuditEnabled && props.inputProvenanceV2Enabled)

const auditStageCount = computed(() =>
  normalizeCounter(props.runtimeStatus?.audit_fast_calls)
  + normalizeCounter(props.runtimeStatus?.audit_full_calls)
  + normalizeCounter(props.runtimeStatus?.audit_max_calls)
)
const hasAuditStageCounters = computed(() => [
  props.runtimeStatus?.audit_fast_calls,
  props.runtimeStatus?.audit_full_calls,
  props.runtimeStatus?.audit_max_calls,
].every((value) => availableCounter(value) !== null))
const resultCacheRate = computed(() => {
  if (
    !props.runtimeStatus
    || !hasAuditStageCounters.value
    || availableCounter(props.runtimeStatus.audit_result_cache_hits) === null
  ) return t('common.unknown')
  return formatRate(
    normalizeCounter(props.runtimeStatus.audit_result_cache_hits),
    auditStageCount.value + normalizeCounter(props.runtimeStatus.audit_result_cache_hits)
  )
})
const completeUsageSamples = computed(() => {
  const explicit = availableCounter(props.runtimeStatus?.audit_usage_complete)
  if (explicit !== null) return explicit
  if (
    !props.runtimeStatus
    || !hasAuditStageCounters.value
    || availableCounter(props.runtimeStatus.audit_usage_unknown) === null
  ) return null
  return Math.max(
    0,
    auditStageCount.value
      - normalizeCounter(props.runtimeStatus.audit_usage_unknown)
  )
})
const tokenCacheRate = computed(() => {
  const runtimeStatus = props.runtimeStatus
  if (!runtimeStatus) {
    return t('common.unknown')
  }

  const unknownSamples = normalizeCounter(runtimeStatus.audit_usage_unknown)
  if (unknownSamples > 0 && normalizeCounter(completeUsageSamples.value) === 0) return t('common.unknown')

  const promptTokens = availableCounter(runtimeStatus.audit_prompt_tokens)
  const cachedTokens = availableCounter(runtimeStatus.audit_cached_input_tokens)
  const uncachedTokens = availableCounter(runtimeStatus.audit_uncached_input_tokens)
  if (
    promptTokens === null
    || cachedTokens === null
    || uncachedTokens === null
    || promptTokens !== cachedTokens + uncachedTokens
  ) {
    return t('common.unknown')
  }

  return formatRate(cachedTokens, promptTokens)
})

const costCoverageLabel = computed(() => {
  const coverage = props.runtimeStatus?.audit_cost_coverage
  if (coverage === 'complete' || coverage === 'partial' || coverage === 'unknown' || coverage === 'no_samples') {
    return t('admin.riskControl.aiRuntimeCostCoverage.' + coverage)
  }
  return t('common.unknown')
})

const stageLatencyItems = computed(() => {
  const values = props.runtimeStatus?.audit_stage_latency
  if (!values) return []
  return ['fast', 'full', 'max'].flatMap((key) => {
    const summary = values[key]
    if (!summary || !isNonNegativeInteger(summary.count)) return []
    return [{
      key,
      label: t(`admin.riskControl.aiRuntimeStage.${key}`),
      count: formatCounter(summary.count),
      average: summary.count > 0 ? formatCounter(summary.average_ms) : t('common.unknown'),
      p95: summary.count > 0 ? formatCounter(summary.p95_upper_ms) : t('common.unknown'),
    }]
  })
})

const sessionSourceItems = computed(() => mapMetricEntries(
  props.runtimeStatus?.audit_session_sources,
  'admin.riskControl.aiRuntimeSessionSource'
))

const prefixContinuityItems = computed(() => mapMetricEntries(
  props.runtimeStatus?.audit_prefix_continuity,
  'admin.riskControl.aiRuntimePrefixReason'
))

function availableCounter(value: number | null | undefined): number | null {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0
    ? Math.floor(value)
    : null
}

function normalizeCounter(value: number | null | undefined): number {
  return availableCounter(value) ?? 0
}

function formatCounter(value: number | null | undefined): string {
  const normalized = availableCounter(value)
  return normalized === null ? t('common.unknown') : new Intl.NumberFormat().format(normalized)
}

function formatRate(numerator: number, denominator: number): string {
  if (denominator <= 0) return '0.0%'
  const ratio = Math.min(1, Math.max(0, numerator / denominator))
  return `${(ratio * 100).toFixed(1)}%`
}

function formatUSD(value: number | null | undefined): string {
  if (typeof value !== 'number' || !Number.isFinite(value) || value < 0) return t('common.unknown')
  if (value > 0 && value < 0.000001) return '< USD 0.000001'
  return 'USD ' + value.toFixed(6)
}

function emitNumber(eventName: NumericUpdateEvent, event: Event) {
  emit(eventName, Number((event.target as HTMLInputElement).value))
}

function emitNullableNumber(eventName: PricingNumericUpdateEvent, event: Event) {
  const raw = (event.target as HTMLInputElement).value.trim()
  emit(eventName, raw === '' ? null : Number(raw))
}

function emitText(eventName: 'update:pricingVersion', event: Event) {
  emit(eventName, (event.target as HTMLInputElement).value)
}

function updateInputProvenance(value: boolean) {
  if (!value && provenanceToggleLocked.value) return
  emit('update:inputProvenanceV2Enabled', value)
}

function isNonNegativeInteger(value: unknown): value is number {
  return typeof value === 'number' && Number.isInteger(value) && value >= 0
}

function mapMetricEntries(values: Record<string, number> | undefined, translationPrefix: string) {
  if (!values) return []
  return Object.entries(values)
    .filter(([, count]) => isNonNegativeInteger(count))
    .map(([key, count]) => ({
      key,
      label: t(`${translationPrefix}.${key}`),
      count: formatCounter(count),
    }))
}

function isIntegerInRange(value: number, min: number, max: number): boolean {
  return Number.isInteger(value) && value >= min && value <= max
}

function isNumberInRange(value: number, min: number, max: number): boolean {
  return Number.isFinite(value) && value >= min && value <= max
}

function validate(): string | null {
  if (props.incrementalAuditEnabled) {
    if (!props.inputProvenanceV2Enabled) return 'admin.riskControl.aiIncrementalRequiresProvenance'
    if (!isIntegerInRange(props.recentUserTurns, 1, 8)) return 'admin.riskControl.aiRecentUserTurnsInvalid'
    if (!isIntegerInRange(props.summaryMaxChars, 1, 4000)) return 'admin.riskControl.aiSummaryMaxCharsInvalid'
    if (!isNumberInRange(props.fullReviewThreshold, 0.01, 1) || props.fullReviewThreshold >= props.blockThreshold) return 'admin.riskControl.aiFullReviewThresholdInvalid'
    if (!isNumberInRange(props.fullReviewRiskDelta, 0.01, 1)) return 'admin.riskControl.aiFullReviewRiskDeltaInvalid'
    if (!isIntegerInRange(props.periodicFullReviewTurns, 1, 100)) return 'admin.riskControl.aiPeriodicFullReviewTurnsInvalid'
    if (!isIntegerInRange(props.fullReviewMaxInputChars, 1000, props.maxInputChars)) return 'admin.riskControl.aiFullReviewMaxInputCharsInvalid'
    if (!isIntegerInRange(props.fastMaxOutputTokens, 1, 8192)) return 'admin.riskControl.aiFastMaxOutputTokensInvalid'
    if (!isIntegerInRange(props.fullMaxOutputTokens, 1, 8192)) return 'admin.riskControl.aiFullMaxOutputTokensInvalid'
    if (!isIntegerInRange(props.maxReviewMaxOutputTokens, 1, 8192)) return 'admin.riskControl.aiMaxReviewMaxOutputTokensInvalid'
    if (!isIntegerInRange(props.auditContextTtlMinutes, 1, 1440)) return 'admin.riskControl.aiAuditContextTTLInvalid'
  }
  if (props.pricingConfigured) {
    if (props.pricingVersion.trim().length < 1 || props.pricingVersion.trim().length > 100) return 'admin.riskControl.aiPricingVersionInvalid'
    const rates = [
      props.uncachedInputUsdPerMillionTokens,
      props.cachedInputUsdPerMillionTokens,
      props.outputUsdPerMillionTokens,
    ]
    if (rates.some((value) => value === null || !isNumberInRange(value, 0, 1000000))) return 'admin.riskControl.aiPricingRateInvalid'
  }
  return null
}

defineExpose({ validate })
</script>
