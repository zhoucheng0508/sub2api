import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import IncrementalAuditSettings from '../IncrementalAuditSettings.vue'
import Toggle from '@/components/common/Toggle.vue'
import type { ContentModerationRuntimeStatus } from '@/api/admin/riskControl'

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string | number>) => {
        if (key === 'admin.riskControl.aiRuntimeUsageCoverage') {
          return `${key}:${params?.complete}/${params?.unknown}`
        }
        if (key === 'admin.riskControl.aiRuntimeLatencySummary') {
          return `${key}:${params?.average}/${params?.p95}/${params?.count}`
        }
        return key
      },
    }),
  }
})

const defaultProps = () => ({
  incrementalAuditEnabled: true,
  inputProvenanceV2Enabled: true,
  deterministicRiskV2Enabled: true,
  recentUserTurns: 2,
  summaryMaxChars: 800,
  fullReviewThreshold: 0.4,
  fullReviewRiskDelta: 0.15,
  periodicFullReviewTurns: 10,
  fullReviewMaxInputChars: 60000,
  fastMaxOutputTokens: 256,
  fullMaxOutputTokens: 1024,
  maxReviewMaxOutputTokens: 1536,
  auditContextTtlMinutes: 120,
  pricingConfigured: false,
  pricingVersion: '',
  uncachedInputUsdPerMillionTokens: null,
  cachedInputUsdPerMillionTokens: null,
  outputUsdPerMillionTokens: null,
  maxInputChars: 200000,
  blockThreshold: 0.8,
})

const runtimeStatus = (
  overrides: Partial<ContentModerationRuntimeStatus> = {}
): ContentModerationRuntimeStatus => ({
  enabled: true,
  risk_control_enabled: true,
  mode: 'pre_block',
  worker_count: 4,
  max_workers: 32,
  active_workers: 0,
  idle_workers: 4,
  queue_size: 32768,
  queue_length: 0,
  queue_usage_percent: 0,
  enqueued: 0,
  dropped: 0,
  processed: 0,
  errors: 0,
  pre_block_active: 0,
  pre_block_checked: 0,
  pre_block_allowed: 0,
  pre_block_blocked: 0,
  pre_block_errors: 0,
  pre_block_avg_latency_ms: 0,
  pre_block_api_key_active: 0,
  pre_block_api_key_available_count: 0,
  pre_block_api_key_total_calls: 0,
  pre_block_api_key_loads: [],
  api_key_statuses: [],
  audit_fast_calls: 0,
  audit_full_calls: 0,
  audit_max_calls: 0,
  audit_result_cache_hits: 0,
  audit_prompt_tokens: 0,
  audit_cached_input_tokens: 0,
  audit_uncached_input_tokens: 0,
  audit_output_tokens: 0,
  audit_usage_unknown: 0,
  audit_input_chars: 0,
  metrics_started_at: '2026-08-05T01:02:03Z',
  audit_estimated_cost_usd: null,
  audit_cost_coverage: 'no_samples',
  audit_cost_complete: false,
  audit_cost_partial: false,
  audit_cost_priced_samples: 0,
  audit_cost_unpriced_samples: 0,
  audit_cost_by_pricing_version_usd: {},
  business_actual_cost_usd: null,
  audit_cost_per_business_usd: null,
  flagged_hash_count: 0,
  last_cleanup_deleted_hit: 0,
  last_cleanup_deleted_non_hit: 0,
  ...overrides,
})

function validator(wrapper: ReturnType<typeof mount>): { validate: () => string | null } {
  return wrapper.vm as unknown as { validate: () => string | null }
}

describe('IncrementalAuditSettings', () => {
  it('renders the advanced section with coverage, session identity, and privacy notices', () => {
    const wrapper = mount(IncrementalAuditSettings, { props: defaultProps() })

    expect(wrapper.get('details').attributes('open')).toBeUndefined()
    expect(wrapper.text()).toContain('admin.riskControl.aiIncrementalCoverageNotice')
    expect(wrapper.text()).toContain('admin.riskControl.aiIncrementalSessionNotice')
    expect(wrapper.text()).toContain('admin.riskControl.aiIncrementalPrivacyNotice')
    expect(wrapper.get('[data-test="incremental-audit-status"]').text()).toBe('common.enabled')
  })

  it('renders process-lifetime audit counters and calculates cache rates from provider calls plus hits', () => {
    const wrapper = mount(IncrementalAuditSettings, {
      props: {
        ...defaultProps(),
        runtimeStatus: runtimeStatus({
          audit_fast_calls: 70,
          audit_full_calls: 20,
          audit_max_calls: 10,
          audit_result_cache_hits: 25,
          audit_prompt_tokens: 800,
          audit_cached_input_tokens: 600,
          audit_uncached_input_tokens: 200,
          audit_output_tokens: 44,
          audit_usage_unknown: 0,
          audit_input_chars: 505,
        }),
      },
    })

    expect(wrapper.get('[data-test="audit-fast-calls"]').text()).toBe('70')
    expect(wrapper.get('[data-test="audit-full-calls"]').text()).toBe('20')
    expect(wrapper.get('[data-test="audit-max-calls"]').text()).toBe('10')
    expect(wrapper.get('[data-test="audit-result-cache-hits"]').text()).toBe('25')
    expect(wrapper.get('[data-test="audit-result-cache-rate"]').text()).toBe('20.0%')
    expect(wrapper.get('[data-test="audit-prompt-tokens"]').text()).toBe('800')
    expect(wrapper.get('[data-test="audit-cached-input-tokens"]').text()).toBe('600')
    expect(wrapper.get('[data-test="audit-uncached-input-tokens"]').text()).toBe('200')
    expect(wrapper.get('[data-test="audit-token-cache-rate"]').text()).toBe('75.0%')
    expect(wrapper.get('[data-test="audit-output-tokens"]').text()).toBe('44')
    expect(wrapper.get('[data-test="audit-input-chars"]').text()).toBe('505')
    expect(wrapper.get('[data-test="audit-usage-unknown"]').text()).toBe('0')
    expect(wrapper.get('[data-test="audit-usage-coverage"]').text()).toContain('100/0')
  })

  it('shows unknown counters and rates when runtime metrics are unavailable', () => {
    const wrapper = mount(IncrementalAuditSettings, { props: defaultProps() })

    expect(wrapper.get('[data-test="audit-fast-calls"]').text()).toBe('common.unknown')
    expect(wrapper.get('[data-test="audit-result-cache-rate"]').text()).toBe('common.unknown')
    expect(wrapper.get('[data-test="audit-token-cache-rate"]').text()).toBe('common.unknown')
    expect(wrapper.get('[data-test="audit-usage-coverage"]').text()).toContain('common.unknown/common.unknown')
  })

  it('shows unknown rates for a legacy runtime payload without audit counters', () => {
    const legacyStatus: Partial<ContentModerationRuntimeStatus> = { ...runtimeStatus() }
    delete legacyStatus.audit_fast_calls
    delete legacyStatus.audit_full_calls
    delete legacyStatus.audit_max_calls
    delete legacyStatus.audit_result_cache_hits
    delete legacyStatus.audit_prompt_tokens
    delete legacyStatus.audit_cached_input_tokens
    delete legacyStatus.audit_uncached_input_tokens
    delete legacyStatus.audit_usage_unknown

    const wrapper = mount(IncrementalAuditSettings, {
      props: {
        ...defaultProps(),
        runtimeStatus: legacyStatus as ContentModerationRuntimeStatus,
      },
    })

    expect(wrapper.get('[data-test="audit-result-cache-rate"]').text()).toBe('common.unknown')
    expect(wrapper.get('[data-test="audit-token-cache-rate"]').text()).toBe('common.unknown')
    expect(wrapper.get('[data-test="audit-usage-coverage"]').text()).toContain('common.unknown/common.unknown')
  })

  it('shows zero rates when a current runtime payload explicitly reports zero calls', () => {
    const wrapper = mount(IncrementalAuditSettings, {
      props: { ...defaultProps(), runtimeStatus: runtimeStatus() },
    })

    expect(wrapper.get('[data-test="audit-result-cache-rate"]').text()).toBe('0.0%')
    expect(wrapper.get('[data-test="audit-token-cache-rate"]').text()).toBe('0.0%')
  })

  it('shows an unknown provider cache rate when any upstream usage was unknown', () => {
    const wrapper = mount(IncrementalAuditSettings, {
      props: {
        ...defaultProps(),
        runtimeStatus: runtimeStatus({
          audit_prompt_tokens: 800,
          audit_cached_input_tokens: 600,
          audit_uncached_input_tokens: 200,
          audit_usage_unknown: 1,
        }),
      },
    })

    expect(wrapper.get('[data-test="audit-token-cache-rate"]').text()).toBe('common.unknown')
  })

  it('keeps the provider cache ratio available when complete samples exist alongside unknown samples', () => {
    const wrapper = mount(IncrementalAuditSettings, {
      props: {
        ...defaultProps(),
        runtimeStatus: runtimeStatus({
          audit_fast_calls: 2,
          audit_usage_complete: 1,
          audit_usage_unknown: 1,
          audit_prompt_tokens: 800,
          audit_cached_input_tokens: 600,
          audit_uncached_input_tokens: 200,
        }),
      },
    })

    expect(wrapper.get('[data-test="audit-token-cache-rate"]').text()).toBe('75.0%')
    expect(wrapper.get('[data-test="audit-usage-coverage"]').text()).toContain('1/1')
  })

  it('renders stage latency, session sources, and prefix continuity diagnostics', () => {
    const wrapper = mount(IncrementalAuditSettings, {
      props: {
        ...defaultProps(),
        runtimeStatus: runtimeStatus({
          audit_stage_latency: {
            fast: { count: 4, average_ms: 120, p95_upper_ms: 180 },
            full: { count: 1, average_ms: 900, p95_upper_ms: 900 },
          },
          audit_session_sources: { header: 3, prompt_cache_key: 1, none: 2 },
          audit_prefix_continuity: { continuous: 3, breaks: 2, history_rewritten: 1 },
        }),
      },
    })

    expect(wrapper.get('[data-test="audit-stage-latency"]').text()).toContain('admin.riskControl.aiRuntimeStage.fast')
    expect(wrapper.get('[data-test="audit-stage-latency"]').text()).toContain('120/180/4')
    expect(wrapper.get('[data-test="audit-session-sources"]').text()).toContain('admin.riskControl.aiRuntimeSessionSource.prompt_cache_key')
    expect(wrapper.get('[data-test="audit-prefix-continuity"]').text()).toContain('admin.riskControl.aiRuntimePrefixReason.history_rewritten')
  })

  it('renders USD cost values and complete coverage without hiding the unit', () => {
    const wrapper = mount(IncrementalAuditSettings, {
      props: {
        ...defaultProps(),
        runtimeStatus: runtimeStatus({
          audit_estimated_cost_usd: 0.25,
          business_actual_cost_usd: 10,
          audit_cost_per_business_usd: 0.025,
          audit_cost_coverage: 'complete',
          audit_cost_complete: true,
          audit_cost_priced_samples: 8,
          audit_cost_unpriced_samples: 0,
        }),
      },
    })

    expect(wrapper.get('[data-test="audit-estimated-cost-usd"]').text()).toBe('USD 0.250000')
    expect(wrapper.get('[data-test="business-actual-cost-usd"]').text()).toBe('USD 10.000000')
    expect(wrapper.get('[data-test="audit-cost-per-business-usd"]').text()).toBe('USD 0.025000')
    expect(wrapper.get('[data-test="audit-cost-coverage"]').text()).toBe('admin.riskControl.aiRuntimeCostCoverage.complete')
  })

  it('keeps known positive sub-micro-dollar costs distinct from zero', () => {
    const wrapper = mount(IncrementalAuditSettings, {
      props: {
        ...defaultProps(),
        runtimeStatus: runtimeStatus({
          audit_estimated_cost_usd: 0.0000004,
          business_actual_cost_usd: 0,
          audit_cost_per_business_usd: 0.0000009,
        }),
      },
    })

    expect(wrapper.get('[data-test="audit-estimated-cost-usd"]').text()).toBe('< USD 0.000001')
    expect(wrapper.get('[data-test="business-actual-cost-usd"]').text()).toBe('USD 0.000000')
    expect(wrapper.get('[data-test="audit-cost-per-business-usd"]').text()).toBe('< USD 0.000001')
  })

  it('shows partial coverage and never renders missing cost as zero', () => {
    const wrapper = mount(IncrementalAuditSettings, {
      props: {
        ...defaultProps(),
        runtimeStatus: runtimeStatus({
          audit_estimated_cost_usd: 0.1,
          audit_cost_coverage: 'partial',
          audit_cost_partial: true,
          audit_cost_priced_samples: 2,
          audit_cost_unpriced_samples: 1,
          audit_usage_unknown: 1,
        }),
      },
    })

    expect(wrapper.get('[data-test="audit-estimated-cost-usd"]').text()).toBe('USD 0.100000')
    expect(wrapper.get('[data-test="business-actual-cost-usd"]').text()).toBe('common.unknown')
    expect(wrapper.get('[data-test="audit-cost-per-business-usd"]').text()).toBe('common.unknown')
    expect(wrapper.get('[data-test="audit-cost-coverage"]').text()).toBe('admin.riskControl.aiRuntimeCostCoverage.partial')
  })

  it('shows an unknown provider cache rate when token usage does not balance', () => {
    const wrapper = mount(IncrementalAuditSettings, {
      props: {
        ...defaultProps(),
        runtimeStatus: runtimeStatus({
          audit_prompt_tokens: 900,
          audit_cached_input_tokens: 600,
          audit_uncached_input_tokens: 200,
        }),
      },
    })

    expect(wrapper.get('[data-test="audit-token-cache-rate"]').text()).toBe('common.unknown')
  })

  it.each([
    'audit_prompt_tokens',
    'audit_cached_input_tokens',
    'audit_uncached_input_tokens',
  ] as const)('shows an unknown provider cache rate when %s is unavailable', (field) => {
    const status = runtimeStatus({
      audit_prompt_tokens: 800,
      audit_cached_input_tokens: 600,
      audit_uncached_input_tokens: 200,
    }) as ContentModerationRuntimeStatus & Record<typeof field, number | undefined>
    status[field] = undefined

    const wrapper = mount(IncrementalAuditSettings, {
      props: { ...defaultProps(), runtimeStatus: status },
    })

    expect(wrapper.get('[data-test="audit-token-cache-rate"]').text()).toBe('common.unknown')
  })

  it('emits the feature switch and every advanced numeric control independently', async () => {
    const wrapper = mount(IncrementalAuditSettings, { props: defaultProps() })

    await wrapper.get('[data-test="ai-incremental-audit-enabled"]').trigger('click')
    await wrapper.get('[data-test="ai-deterministic-risk-v2-enabled"]').trigger('click')
    await wrapper.get('[data-test="ai-recent-user-turns"]').setValue('3')
    await wrapper.get('[data-test="ai-summary-max-chars"]').setValue('1000')
    await wrapper.get('[data-test="ai-full-review-threshold"]').setValue('0.45')
    await wrapper.get('[data-test="ai-full-review-risk-delta"]').setValue('0.2')
    await wrapper.get('[data-test="ai-periodic-full-review-turns"]').setValue('12')
    await wrapper.get('[data-test="ai-full-review-max-input-chars"]').setValue('80000')
    await wrapper.get('[data-test="ai-fast-max-output-tokens"]').setValue('320')
    await wrapper.get('[data-test="ai-full-max-output-tokens"]').setValue('1280')
    await wrapper.get('[data-test="ai-max-review-max-output-tokens"]').setValue('1792')
    await wrapper.get('[data-test="ai-audit-context-ttl-minutes"]').setValue('180')

    expect(wrapper.emitted('update:incrementalAuditEnabled')).toEqual([[false]])
    expect(wrapper.emitted('update:deterministicRiskV2Enabled')).toEqual([[false]])
    expect(wrapper.emitted('update:recentUserTurns')).toEqual([[3]])
    expect(wrapper.emitted('update:summaryMaxChars')).toEqual([[1000]])
    expect(wrapper.emitted('update:fullReviewThreshold')).toEqual([[0.45]])
    expect(wrapper.emitted('update:fullReviewRiskDelta')).toEqual([[0.2]])
    expect(wrapper.emitted('update:periodicFullReviewTurns')).toEqual([[12]])
    expect(wrapper.emitted('update:fullReviewMaxInputChars')).toEqual([[80000]])
    expect(wrapper.emitted('update:fastMaxOutputTokens')).toEqual([[320]])
    expect(wrapper.emitted('update:fullMaxOutputTokens')).toEqual([[1280]])
    expect(wrapper.emitted('update:maxReviewMaxOutputTokens')).toEqual([[1792]])
    expect(wrapper.emitted('update:auditContextTtlMinutes')).toEqual([[180]])

    await wrapper.setProps({ incrementalAuditEnabled: false })
    await wrapper.get('[data-test="ai-input-provenance-v2-enabled"]').trigger('click')
    expect(wrapper.emitted('update:inputProvenanceV2Enabled')).toEqual([[false]])
  })

  it('maintains a versioned USD pricing table and validates complete rates', async () => {
    const wrapper = mount(IncrementalAuditSettings, {
      props: {
        ...defaultProps(),
        pricingConfigured: true,
        pricingVersion: 'deepseek-2026-08',
        uncachedInputUsdPerMillionTokens: 0.28,
        cachedInputUsdPerMillionTokens: 0.028,
        outputUsdPerMillionTokens: 0.42,
      },
    })

    expect(validator(wrapper).validate()).toBeNull()
    await wrapper.get('[data-test="ai-pricing-version"]').setValue('deepseek-2026-09')
    await wrapper.get('[data-test="ai-uncached-input-price"]').setValue('0.3')
    await wrapper.get('[data-test="ai-cached-input-price"]').setValue('0.03')
    await wrapper.get('[data-test="ai-output-price"]').setValue('0.5')

    expect(wrapper.emitted('update:pricingVersion')).toEqual([['deepseek-2026-09']])
    expect(wrapper.emitted('update:uncachedInputUsdPerMillionTokens')).toEqual([[0.3]])
    expect(wrapper.emitted('update:cachedInputUsdPerMillionTokens')).toEqual([[0.03]])
    expect(wrapper.emitted('update:outputUsdPerMillionTokens')).toEqual([[0.5]])

    await wrapper.setProps({ cachedInputUsdPerMillionTokens: null })
    expect(validator(wrapper).validate()).toBe('admin.riskControl.aiPricingRateInvalid')
  })

  it('does not allow provenance V2 to be disabled while incremental auditing is active', async () => {
    const wrapper = mount(IncrementalAuditSettings, { props: defaultProps() })
    const provenanceToggle = wrapper.findAllComponents(Toggle)[1]

    expect(provenanceToggle.attributes('disabled')).toBeDefined()
    provenanceToggle.vm.$emit('update:modelValue', false)
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('update:inputProvenanceV2Enabled')).toBeUndefined()

    await wrapper.setProps({ incrementalAuditEnabled: false })
    provenanceToggle.vm.$emit('update:modelValue', false)
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('update:inputProvenanceV2Enabled')).toEqual([[false]])
  })

  it('blocks an invalid persisted incremental and provenance combination', async () => {
    const wrapper = mount(IncrementalAuditSettings, {
      props: {
        ...defaultProps(),
        inputProvenanceV2Enabled: false,
      },
    })

    expect(wrapper.get('[data-test="incremental-provenance-warning"]').text()).toBe('admin.riskControl.aiIncrementalRequiresProvenance')
    expect(validator(wrapper).validate()).toBe('admin.riskControl.aiIncrementalRequiresProvenance')
  })

  it('validates active ranges but ignores disabled incremental-only settings', async () => {
    const wrapper = mount(IncrementalAuditSettings, { props: defaultProps() })

    expect(validator(wrapper).validate()).toBeNull()

    await wrapper.setProps({ periodicFullReviewTurns: 101 })
    expect(validator(wrapper).validate()).toBe('admin.riskControl.aiPeriodicFullReviewTurnsInvalid')

    await wrapper.setProps({ periodicFullReviewTurns: 10, fullReviewThreshold: 0.8 })
    expect(validator(wrapper).validate()).toBe('admin.riskControl.aiFullReviewThresholdInvalid')

    await wrapper.setProps({
      incrementalAuditEnabled: false,
      recentUserTurns: 9,
      fullReviewThreshold: 0.8,
      fullReviewMaxInputChars: 60000,
      maxInputChars: 50000,
    })
    expect(validator(wrapper).validate()).toBeNull()
    expect(wrapper.get<HTMLInputElement>('[data-test="ai-recent-user-turns"]').element.closest('fieldset')?.disabled).toBe(true)

    await wrapper.setProps({ pricingConfigured: true, pricingVersion: '' })
    expect(validator(wrapper).validate()).toBe('admin.riskControl.aiPricingVersionInvalid')

    await wrapper.setProps({ pricingVersion: 'deepseek-2026-08' })
    expect(validator(wrapper).validate()).toBe('admin.riskControl.aiPricingRateInvalid')
  })
})
