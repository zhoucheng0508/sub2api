import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import RecommendedPromptControl from '../RecommendedPromptControl.vue'
import ModerationPerformanceSettings from '../ModerationPerformanceSettings.vue'
import ModerationTestOutcome from '../ModerationTestOutcome.vue'
import type { ContentModerationTestAuditResult } from '@/api/admin/riskControl'

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string>) => (
        params?.version ? `${key}:${params.version}` : key
      ),
    }),
  }
})

describe('RecommendedPromptControl', () => {
  it('keeps a custom prompt until the administrator explicitly applies the backend recommendation', async () => {
    const wrapper = mount(RecommendedPromptControl, {
      props: {
        modelValue: 'custom prompt',
        recommendedSystemPrompt: 'backend recommended prompt',
        recommendedPromptVersion: 'vote-ai-2026-08-04',
        systemPromptVersion: 'custom',
        usesRecommendedSystemPrompt: false,
      },
    })

    expect(wrapper.get<HTMLTextAreaElement>('[data-test="ai-system-prompt"]').element.value).toBe('custom prompt')
    expect(wrapper.get('[data-test="custom-prompt-notice"]').exists()).toBe(true)
    expect(wrapper.get('[data-test="recommended-prompt-version"]').text()).toContain('vote-ai-2026-08-04')

    await wrapper.get('[data-test="apply-recommended-prompt"]').trigger('click')
    expect(wrapper.emitted('apply-recommended')).toEqual([[]])
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('marks the recommendation active by comparing against backend-provided content', () => {
    const wrapper = mount(RecommendedPromptControl, {
      props: {
        modelValue: 'backend recommended prompt',
        recommendedSystemPrompt: 'backend recommended prompt',
        recommendedPromptVersion: 'v2',
        systemPromptVersion: 'v2',
        usesRecommendedSystemPrompt: true,
      },
    })

    expect(wrapper.get('[data-test="apply-recommended-prompt"]').attributes('disabled')).toBeDefined()
    expect(wrapper.find('[data-test="custom-prompt-notice"]').exists()).toBe(false)
  })
})

describe('ModerationPerformanceSettings', () => {
  it('emits all three bounded performance controls independently', async () => {
    const wrapper = mount(ModerationPerformanceSettings, {
      props: {
        synchronousBudgetMs: 4800,
        fastInputChars: 12000,
        fallbackInputChars: 4000,
        maxInputChars: 200000,
      },
    })

    await wrapper.get('[data-test="ai-synchronous-budget-ms"]').setValue('4500')
    await wrapper.get('[data-test="ai-fast-input-chars"]').setValue('16000')
    await wrapper.get('[data-test="ai-fallback-input-chars"]').setValue('6000')

    expect(wrapper.emitted('update:synchronousBudgetMs')).toEqual([[4500]])
    expect(wrapper.emitted('update:fastInputChars')).toEqual([[16000]])
    expect(wrapper.emitted('update:fallbackInputChars')).toEqual([[6000]])
    expect(wrapper.get('label[for="vote-ai-moderation-synchronous-budget"]').exists()).toBe(true)
    expect(wrapper.get('#vote-ai-moderation-synchronous-budget').exists()).toBe(true)
  })
})

describe('ModerationTestOutcome', () => {
  const result: ContentModerationTestAuditResult = {
    flagged: true,
    risk_score: 0.82,
    risk_tier: 'high',
    categories: ['credential_theft'],
    signals: ['ownership_unverified', 'progressive_escalation'],
    highest_category: 'credential_theft',
    highest_score: 0.82,
    composite_score: 0.82,
    category_scores: { credential_theft: 0.82 },
    thresholds: { credential_theft: 0.7 },
    reason: '所有权无法核实',
    review_incomplete: true,
    review_error: 'supplemental review timeout',
  }

  it('renders risk tier, localized categories and signals, and incomplete review state', () => {
    const wrapper = mount(ModerationTestOutcome, {
      props: { result: { ...result, categories: ['ai_risk', 'self-harm/intent'] } },
    })

    expect(wrapper.attributes('role')).toBe('status')
    expect(wrapper.attributes('aria-live')).toBe('polite')
    expect(wrapper.get('[data-test="audit-test-risk-score"]').text()).toBe('82.0%')
    expect(wrapper.get('[data-test="audit-test-risk-tier"]').text()).toContain('auditRiskTier.high')
    expect(wrapper.get('[data-test="audit-test-categories"]').text()).toContain('auditCategoryLabels.ai_risk')
    expect(wrapper.get('[data-test="audit-test-categories"]').text()).toContain('auditCategoryLabels.self_harm_intent')
    expect(wrapper.get('[data-test="audit-test-signals"]').text()).toContain('auditSignalLabels.progressive_escalation')
    expect(wrapper.get('[data-test="audit-review-incomplete"]').text()).toContain('supplemental review timeout')
  })

  it('does not invent an AI Chat risk tier for OpenAI Moderations results', () => {
    const wrapper = mount(ModerationTestOutcome, {
      props: { result: { ...result, risk_tier: '' } },
    })

    expect(wrapper.find('[data-test="audit-test-risk-tier"]').exists()).toBe(false)
    expect(wrapper.get('[data-test="audit-test-risk-score"]').text()).toBe('82.0%')
  })

  it('renders observe as an amber review state instead of a green pass', () => {
    const wrapper = mount(ModerationTestOutcome, {
      props: { result: { ...result, flagged: false, risk_score: 0.45, risk_tier: 'observe' } },
    })

    const status = wrapper.get('[data-test="audit-test-flagged-status"]')
    expect(status.text()).toContain('auditRiskTier.observe')
    expect(status.classes()).toContain('bg-amber-50')
    expect(status.text()).not.toContain('auditTestPassed')
  })

  it('renders future unknown tiers neutrally', () => {
    const wrapper = mount(ModerationTestOutcome, {
      props: { result: { ...result, flagged: false, risk_tier: 'review' } },
    })

    expect(wrapper.get('[data-test="audit-test-risk-tier"]').text()).toContain('auditRiskTier.unknown')
    expect(wrapper.get('[data-test="audit-test-flagged-status"]').classes()).toContain('bg-gray-100')
  })
})
