import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import AuditStageDiagnostics from '../AuditStageDiagnostics.vue'

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string | number>) => {
        if (key === 'admin.riskControl.auditTokenSummary') {
          return `input=${params?.prompt};cached=${params?.cached};uncached=${params?.uncached};output=${params?.output}`
        }
        return key
      },
    }),
  }
})

describe('AuditStageDiagnostics', () => {
  it('separates a real fast provider call from a cached full review', () => {
    const wrapper = mount(AuditStageDiagnostics, {
      props: {
        stages: [
          {
            stage: 'fast',
            provider_called: true,
            result_cache_hit: false,
            usage_known: true,
            failed: false,
            input_chars: 900,
            latency_ms: 120,
            prompt_tokens: 100,
            cached_input_tokens: 80,
            uncached_input_tokens: 20,
            output_tokens: 5,
          },
          {
            stage: 'full',
            provider_called: false,
            result_cache_hit: true,
            usage_known: false,
            failed: false,
          },
        ],
      },
    })

    const fast = wrapper.get('[data-test="audit-stage-fast"]')
    expect(fast.text()).toContain('admin.riskControl.aiRuntimeStage.fast')
    expect(fast.text()).toContain('admin.riskControl.cacheMiss')
    expect(fast.text()).toContain('admin.riskControl.usageComplete')
    expect(fast.text()).toContain('900')
    expect(fast.text()).toContain('120')
    expect(fast.text()).toContain('input=100;cached=80;uncached=20;output=5')

    const full = wrapper.get('[data-test="audit-stage-full"]')
    expect(full.text()).toContain('admin.riskControl.aiRuntimeStage.full')
    expect(full.text()).toContain('admin.riskControl.cacheHit')
    expect(full.text()).toContain('admin.riskControl.auditStageNotApplicable')
    expect(full.text()).toContain('admin.riskControl.auditStageNoProviderUsage')
  })

  it('shows a failed provider stage with unknown usage and measured latency', () => {
    const wrapper = mount(AuditStageDiagnostics, {
      props: {
        stages: [{
          stage: 'full',
          provider_called: true,
          result_cache_hit: false,
          usage_known: false,
          failed: true,
          input_chars: 1200,
          latency_ms: 3500,
        }],
      },
    })

    const full = wrapper.get('[data-test="audit-stage-full"]')
    expect(full.text()).toContain('admin.riskControl.auditStageFailed')
    expect(full.text()).toContain('admin.riskControl.usageUnknown')
    expect(full.text()).toContain('1,200')
    expect(full.text()).toContain('3,500')
  })

  it('distinguishes missing stage counters from measured zero values', () => {
    const wrapper = mount(AuditStageDiagnostics, {
      props: {
        stages: [
          {
            stage: 'fast',
            provider_called: false,
            result_cache_hit: true,
            usage_known: false,
            failed: false,
          },
          {
            stage: 'full',
            provider_called: true,
            result_cache_hit: false,
            usage_known: false,
            failed: false,
            input_chars: 0,
            latency_ms: 0,
          },
        ],
      },
    })

    const missingCells = wrapper.get('[data-test="audit-stage-fast"]').findAll('td')
    expect(missingCells[5].text()).toBe('common.unknown')
    expect(missingCells[6].text()).toBe('common.unknown')

    const zeroCells = wrapper.get('[data-test="audit-stage-full"]').findAll('td')
    expect(zeroCells[5].text()).toBe('0')
    expect(zeroCells[6].text()).toBe('0')
  })

  it('does not render an empty legacy stage list', () => {
    const wrapper = mount(AuditStageDiagnostics, { props: {} })

    expect(wrapper.find('[data-test="audit-stage-details"]').exists()).toBe(false)
  })
})
