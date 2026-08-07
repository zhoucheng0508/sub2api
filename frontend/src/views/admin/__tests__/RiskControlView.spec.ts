import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import type { DOMWrapper, VueWrapper } from '@vue/test-utils'

import RiskControlView from '../RiskControlView.vue'
import type {
  ContentModerationAIChatProfile,
  ContentModerationConfig,
  ContentModerationLog,
  UpdateContentModerationConfig,
} from '@/api/admin/riskControl'

const {
  getConfig,
  updateConfig,
  getStatus,
  listLogs,
  getGroups,
  getProxies,
  listUsers,
  getUserById,
  listAccounts,
  getAccountById,
  testAPIKeys,
  deleteFlaggedHash,
  clearFlaggedHashes,
  unbanUser,
  showError,
  showSuccess,
  showWarning,
} = vi.hoisted(() => ({
  getConfig: vi.fn(),
  updateConfig: vi.fn(),
  getStatus: vi.fn(),
  listLogs: vi.fn(),
  getGroups: vi.fn(),
  getProxies: vi.fn(),
  listUsers: vi.fn(),
  getUserById: vi.fn(),
  listAccounts: vi.fn(),
  getAccountById: vi.fn(),
  testAPIKeys: vi.fn(),
  deleteFlaggedHash: vi.fn(),
  clearFlaggedHashes: vi.fn(),
  unbanUser: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  showWarning: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    riskControl: {
      getConfig,
      updateConfig,
      getStatus,
      listLogs,
      testAPIKeys,
      deleteFlaggedHash,
      clearFlaggedHashes,
      unbanUser,
    },
    groups: {
      getAll: getGroups,
    },
    proxies: {
      getAll: getProxies,
    },
    users: {
      list: listUsers,
      getById: getUserById,
    },
    accounts: {
      list: listAccounts,
      getById: getAccountById,
    },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
    showWarning,
  }),
}))

vi.mock('@/utils/apiError', () => ({
  extractApiErrorMessage: (_err: unknown, fallback: string) => fallback,
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string | number>) => {
        if (key === 'admin.riskControl.preBlockAPIKeyLoadSummary') {
          return `同步并发 ${params?.active} / 可用 Key ${params?.available}，累计 ${params?.total} 次，worker：${params?.workerActive} / ${params?.workerTotal}`
        }
        if (key === 'admin.riskControl.unbanPartialSuccess') {
          return `partial success: ${params?.warning}`
        }
        if (key === 'admin.riskControl.aiPromptCurrentVersion' || key === 'admin.riskControl.aiPromptRecommendedVersion') {
          return `${key}:${params?.version}`
        }
        if (key === 'admin.riskControl.auditTokenSummary') {
          return `input=${params?.prompt};cached=${params?.cached};uncached=${params?.uncached};output=${params?.output}`
        }
        return key.replace(/\{(\w+)\}/g, (_, token) => String(params?.[token] ?? `{${token}}`))
      },
    }),
  }
})

const baseConfig = (): ContentModerationConfig => ({
  enabled: true,
  mode: 'pre_block',
  base_url: 'https://api.openai.com',
  model: 'omni-moderation-latest',
  proxy_id: null,
  api_key_configured: false,
  api_key_masked: '',
  api_key_count: 0,
  api_key_masks: [],
  api_key_statuses: [],
  timeout_ms: 3000,
  sample_rate: 100,
  all_groups: true,
  group_ids: [],
  record_non_hits: false,
  worker_count: 4,
  queue_size: 32768,
  block_status: 403,
  block_message: '内容审计命中风险规则，请调整输入后重试',
  email_on_hit: true,
  auto_ban_enabled: true,
  ban_threshold: 10,
  violation_window_hours: 720,
  retry_count: 2,
  hit_retention_days: 180,
  non_hit_retention_days: 3,
  pre_hash_check_enabled: false,
  blocked_keywords: [],
  keyword_blocking_mode: 'keyword_and_api',
  thresholds: {
    harassment: 0.98,
    sexual: 0.65,
  },
  model_filter: {
    type: 'all',
    models: [],
  },
  user_filter: {
    type: 'all',
    user_ids: [],
  },
  account_filter: {
    type: 'all',
    account_ids: [],
  },
})

const aiChatConfig = (
  overrides: Partial<ContentModerationAIChatProfile> = {}
): ContentModerationConfig => ({
  ...baseConfig(),
  audit_provider: 'ai_chat',
  ai_chat: {
    base_url: 'https://api.deepseek.com',
    model: 'deepseek-v4-flash',
    proxy_id: null,
    api_key_configured: true,
    api_key_count: 1,
    api_key_masks: ['********test'],
    timeout_ms: 15000,
    retry_count: 1,
    confidence_threshold: 0.7,
    cache_enabled: true,
    cache_ttl_seconds: 300,
    system_prompt: 'custom prompt',
    recommended_system_prompt: 'backend recommended prompt',
    recommended_prompt_version: 'vote-ai-2026-08-04',
    system_prompt_version: 'custom',
    uses_recommended_system_prompt: false,
    failure_policy: 'allow',
    max_input_chars: 200000,
    synchronous_budget_ms: 4800,
    fast_input_chars: 12000,
    fallback_input_chars: 4000,
    thinking_mode: 'enabled',
    reasoning_effort: 'adaptive',
    risk_levels_enabled: true,
    observe_threshold: 0.35,
    session_risk_enabled: true,
    session_risk_ttl_minutes: 120,
    session_risk_half_life_minutes: 30,
    session_risk_block_cooldown_minutes: 30,
    actor_risk_enabled: true,
    incremental_audit_enabled: false,
    recent_user_turns: 2,
    summary_max_chars: 800,
    full_review_threshold: 0.4,
    full_review_risk_delta: 0.15,
    periodic_full_review_turns: 10,
    full_review_max_input_chars: 60000,
    fast_max_output_tokens: 256,
    full_max_output_tokens: 1024,
    max_review_max_output_tokens: 1536,
    audit_context_ttl_minutes: 120,
    ...overrides,
  },
})

const runtimeStatus = () => ({
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
  flagged_hash_count: 0,
  last_cleanup_deleted_hit: 0,
  last_cleanup_deleted_non_hit: 0,
})

const moderationLog = (overrides: Partial<ContentModerationLog> = {}): ContentModerationLog => ({
  id: 1,
  request_id: 'req-1',
  user_id: 7,
  user_email: 'user@example.com',
  api_key_id: 9,
  api_key_name: 'test-key',
  group_id: 11,
  group_name: 'mixed',
  endpoint: '/v1/responses',
  provider: 'openai',
  model: 'gpt-5.6-terra',
  mode: 'pre_block',
  action: 'block',
  flagged: true,
  highest_category: 'cyber_abuse',
  highest_score: 0.91,
  matched_keyword: '',
  category_scores: { cyber_abuse: 0.91 },
  threshold_snapshot: { cyber_abuse: 0.7 },
  input_excerpt: 'test input',
  upstream_latency_ms: 320,
  error: '',
  audit_status: 'success',
  audit_code: 'audited',
  audit_retryable: false,
  violation_count: 3,
  auto_banned: true,
  email_sent: true,
  side_effect_status: 'completed',
  notification_status: 'sent',
  side_effect_error: '',
  moderation_ban_active: true,
  user_status: 'disabled',
  queue_delay_ms: 0,
  created_at: '2026-08-04T00:00:00Z',
  ...overrides,
})

const AppLayoutStub = { template: '<div><slot /></div>' }
const BaseDialogStub = defineComponent({
  props: {
    show: {
      type: Boolean,
      default: false,
    },
  },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
})
const ModelWhitelistSelectorStub = defineComponent({
  props: {
    modelValue: {
      type: Array,
      default: () => [],
    },
  },
  emits: ['update:modelValue'],
  setup(props, { emit }) {
    const onInput = (event: Event) => {
      const value = (event.target as HTMLInputElement).value
      emit(
        'update:modelValue',
        value
          .split(/[,\n]/)
          .map((item) => item.trim())
          .filter(Boolean)
      )
    }
    return () =>
      h('input', {
        'data-test': 'model-filter-input',
        value: (props.modelValue as string[]).join('\n'),
        onInput,
      })
  },
})

function findButtonByText(wrapper: VueWrapper, text: string): DOMWrapper<HTMLButtonElement> {
  const button = wrapper.findAll<HTMLButtonElement>('button').find((item) => item.text().includes(text))
  if (!button) {
    throw new Error(`button not found: ${text}`)
  }
  return button
}

describe('admin RiskControlView', () => {
  beforeEach(() => {
    getConfig.mockReset()
    updateConfig.mockReset()
    getStatus.mockReset()
    listLogs.mockReset()
    getGroups.mockReset()
    getProxies.mockReset()
    listUsers.mockReset()
    getUserById.mockReset()
    listAccounts.mockReset()
    getAccountById.mockReset()
    testAPIKeys.mockReset()
    deleteFlaggedHash.mockReset()
    clearFlaggedHashes.mockReset()
    unbanUser.mockReset()
    showError.mockReset()
    showSuccess.mockReset()
    showWarning.mockReset()

    getConfig.mockResolvedValue(baseConfig())
    getStatus.mockResolvedValue(runtimeStatus())
    listLogs.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 1 })
    getGroups.mockResolvedValue([])
    getProxies.mockResolvedValue([])
    listUsers.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 1 })
    listAccounts.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 1 })
    unbanUser.mockResolvedValue({
      user_id: 7,
      status: 'active',
      mode: 'restore_and_clear_risk',
      restored: true,
      risk_state_cleared: true,
    })
    deleteFlaggedHash.mockResolvedValue({ deleted: true })
    clearFlaggedHashes.mockResolvedValue({ deleted: 0 })
    updateConfig.mockImplementation(async (payload: UpdateContentModerationConfig) => ({
      ...baseConfig(),
      ...payload,
      model_filter: payload.model_filter ?? baseConfig().model_filter,
      api_key_configured: false,
      api_key_masked: '',
      api_key_count: 0,
      api_key_masks: [],
      api_key_statuses: [],
    }))
  })

  it('saves the selected model filter mode and models', async () => {
    const wrapper = mount(RiskControlView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          BaseDialog: BaseDialogStub,
          Icon: true,
          Select: true,
          Toggle: true,
          Pagination: true,
          ModelWhitelistSelector: ModelWhitelistSelectorStub,
          ProxySelector: true,
        },
      },
    })

    await flushPromises()

    await findButtonByText(wrapper, 'admin.riskControl.openSettings').trigger('click')
    await findButtonByText(wrapper, 'admin.riskControl.tabs.scope').trigger('click')
    await findButtonByText(wrapper, 'admin.riskControl.modelFilterInclude').trigger('click')
    await wrapper.get('[data-test="model-filter-input"]').setValue('gpt-5.5, gpt-5.4')
    await findButtonByText(wrapper, 'admin.riskControl.saveConfig').trigger('click')
    await flushPromises()

    expect(updateConfig).toHaveBeenCalledWith(expect.objectContaining({
      model_filter: {
        type: 'include',
        models: ['gpt-5.5', 'gpt-5.4'],
      },
    }))
    expect(showError).not.toHaveBeenCalled()
  })

  it('keeps a custom audit prompt until the backend recommendation is explicitly applied', async () => {
    getConfig.mockResolvedValue(aiChatConfig())
    const wrapper = mount(RiskControlView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          BaseDialog: BaseDialogStub,
          Icon: true,
          Select: true,
          Toggle: true,
          Pagination: true,
          ModelWhitelistSelector: ModelWhitelistSelectorStub,
          ProxySelector: true,
        },
      },
    })
    await flushPromises()
    await findButtonByText(wrapper, 'admin.riskControl.openSettings').trigger('click')
    await flushPromises()

    const prompt = wrapper.get<HTMLTextAreaElement>('[data-test="ai-system-prompt"]')
    expect(prompt.element.value).toBe('custom prompt')
    expect(wrapper.get('[data-test="active-prompt-version"]').text()).toContain('custom')
    expect(wrapper.get('[data-test="recommended-prompt-version"]').text()).toContain('vote-ai-2026-08-04')

    await wrapper.get('[data-test="apply-recommended-prompt"]').trigger('click')
    await flushPromises()
    expect(prompt.element.value).toBe('backend recommended prompt')
    await findButtonByText(wrapper, 'admin.riskControl.saveConfig').trigger('click')
    await flushPromises()

    expect(updateConfig).toHaveBeenCalledWith(expect.objectContaining({
      ai_system_prompt: 'backend recommended prompt',
    }))
  })

  it('marks an edited legacy prompt as custom immediately', async () => {
    getConfig.mockResolvedValue(aiChatConfig({
      system_prompt: 'legacy prompt',
      system_prompt_version: 'legacy',
    }))
    const wrapper = mount(RiskControlView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          BaseDialog: BaseDialogStub,
          Icon: true,
          Select: true,
          Toggle: true,
          Pagination: true,
          ModelWhitelistSelector: ModelWhitelistSelectorStub,
          ProxySelector: true,
        },
      },
    })
    await flushPromises()
    await findButtonByText(wrapper, 'admin.riskControl.openSettings').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="active-prompt-version"]').text()).toContain('legacy')
    await wrapper.get('[data-test="ai-system-prompt"]').setValue('manually edited prompt')
    expect(wrapper.get('[data-test="active-prompt-version"]').text()).toContain('custom')
  })

  it('saves bounded synchronous audit performance settings and rejects invalid fallback sizing', async () => {
    getConfig.mockResolvedValue(aiChatConfig())
    const wrapper = mount(RiskControlView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          BaseDialog: BaseDialogStub,
          Icon: true,
          Select: true,
          Toggle: true,
          Pagination: true,
          ModelWhitelistSelector: ModelWhitelistSelectorStub,
          ProxySelector: true,
        },
      },
    })
    await flushPromises()
    await findButtonByText(wrapper, 'admin.riskControl.openSettings').trigger('click')
    await flushPromises()

    await wrapper.get('[data-test="ai-synchronous-budget-ms"]').setValue('4500')
    await wrapper.get('[data-test="ai-fast-stage-budget-ms"]').setValue('2200')
    await wrapper.get('[data-test="ai-fast-input-chars"]').setValue('16000')
    await wrapper.get('[data-test="ai-fallback-input-chars"]').setValue('17000')
    await findButtonByText(wrapper, 'admin.riskControl.saveConfig').trigger('click')
    await flushPromises()

    expect(updateConfig).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith('admin.riskControl.aiPerformanceFallbackInvalid')

    await wrapper.get('[data-test="ai-fallback-input-chars"]').setValue('6000')
    await findButtonByText(wrapper, 'admin.riskControl.saveConfig').trigger('click')
    await flushPromises()

    expect(updateConfig).toHaveBeenCalledWith(expect.objectContaining({
      ai_synchronous_budget_ms: 4500,
      ai_fast_stage_budget_ms: 2200,
      ai_fast_input_chars: 16000,
      ai_fallback_input_chars: 6000,
    }))
  })

  it('loads and saves incremental audit settings and blocks invalid advanced values', async () => {
    getConfig.mockResolvedValue(aiChatConfig({
      incremental_audit_enabled: true,
      input_provenance_v2_enabled: true,
      deterministic_risk_v2_enabled: true,
      recent_user_turns: 3,
      summary_max_chars: 1000,
      full_review_threshold: 0.45,
      full_review_risk_delta: 0.2,
      periodic_full_review_turns: 12,
      full_review_max_input_chars: 80000,
      fast_max_output_tokens: 320,
      full_max_output_tokens: 1280,
      max_review_max_output_tokens: 1792,
      audit_context_ttl_minutes: 180,
      pricing_configured: true,
      pricing_version: 'deepseek-2026-08',
      uncached_input_usd_per_million_tokens: 0.28,
      cached_input_usd_per_million_tokens: 0.028,
      output_usd_per_million_tokens: 0.42,
    }))
    getStatus.mockResolvedValue({
      ...runtimeStatus(),
      audit_fast_calls: 80,
      audit_full_calls: 15,
      audit_max_calls: 5,
      audit_result_cache_hits: 20,
      audit_prompt_tokens: 1000,
      audit_cached_input_tokens: 750,
      audit_uncached_input_tokens: 250,
      audit_estimated_cost_usd: 0.125,
      business_actual_cost_usd: 5,
      audit_cost_per_business_usd: 0.025,
      audit_cost_coverage: 'complete',
      audit_cost_priced_samples: 100,
      audit_cost_unpriced_samples: 0,
    })
    const wrapper = mount(RiskControlView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          BaseDialog: BaseDialogStub,
          Icon: true,
          Select: true,
          Toggle: true,
          Pagination: true,
          ModelWhitelistSelector: ModelWhitelistSelectorStub,
          ProxySelector: true,
        },
      },
    })
    await flushPromises()
    await findButtonByText(wrapper, 'admin.riskControl.openSettings').trigger('click')
    await flushPromises()

    expect(wrapper.get<HTMLInputElement>('[data-test="ai-recent-user-turns"]').element.value).toBe('3')
    expect(wrapper.get<HTMLInputElement>('[data-test="ai-full-review-max-input-chars"]').element.value).toBe('80000')
    expect(wrapper.get('[data-test="audit-fast-calls"]').text()).toBe('80')
    expect(wrapper.get('[data-test="audit-result-cache-rate"]').text()).toBe('16.7%')
    expect(wrapper.get('[data-test="audit-token-cache-rate"]').text()).toBe('75.0%')
    expect(wrapper.get<HTMLInputElement>('[data-test="ai-pricing-version"]').element.value).toBe('deepseek-2026-08')
    expect(wrapper.get('[data-test="audit-estimated-cost-usd"]').text()).toBe('USD 0.125000')
    expect(wrapper.get('[data-test="audit-cost-coverage"]').text()).toBe('admin.riskControl.aiRuntimeCostCoverage.complete')

    await wrapper.get('[data-test="ai-periodic-full-review-turns"]').setValue('101')
    await findButtonByText(wrapper, 'admin.riskControl.saveConfig').trigger('click')
    await flushPromises()

    expect(updateConfig).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith('admin.riskControl.aiPeriodicFullReviewTurnsInvalid')

    await wrapper.get('[data-test="ai-periodic-full-review-turns"]').setValue('15')
    await findButtonByText(wrapper, 'admin.riskControl.saveConfig').trigger('click')
    await flushPromises()

    expect(updateConfig).toHaveBeenCalledWith(expect.objectContaining({
      ai_incremental_audit_enabled: true,
      ai_input_provenance_v2_enabled: true,
      ai_deterministic_risk_v2_enabled: true,
      ai_recent_user_turns: 3,
      ai_summary_max_chars: 1000,
      ai_full_review_threshold: 0.45,
      ai_full_review_risk_delta: 0.2,
      ai_periodic_full_review_turns: 15,
      ai_full_review_max_input_chars: 80000,
      ai_fast_max_output_tokens: 320,
      ai_full_max_output_tokens: 1280,
      ai_max_review_max_output_tokens: 1792,
      ai_audit_context_ttl_minutes: 180,
      ai_pricing_configured: true,
      ai_pricing_version: 'deepseek-2026-08',
      ai_uncached_input_usd_per_million_tokens: 0.28,
      ai_cached_input_usd_per_million_tokens: 0.028,
      ai_output_usd_per_million_tokens: 0.42,
    }))
  })

  it('does not coerce empty AI pricing rates to zero after switching providers', async () => {
    getConfig.mockResolvedValue(aiChatConfig({
      pricing_configured: true,
      pricing_version: 'deepseek-2026-08',
      uncached_input_usd_per_million_tokens: null,
      cached_input_usd_per_million_tokens: null,
      output_usd_per_million_tokens: null,
    }))
    const wrapper = mount(RiskControlView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          BaseDialog: BaseDialogStub,
          Icon: true,
          Select: true,
          Toggle: true,
          Pagination: true,
          ModelWhitelistSelector: ModelWhitelistSelectorStub,
          ProxySelector: true,
        },
      },
    })
    await flushPromises()
    await findButtonByText(wrapper, 'admin.riskControl.openSettings').trigger('click')
    await flushPromises()
    await findButtonByText(wrapper, 'admin.riskControl.providerOpenAI').trigger('click')
    await flushPromises()
    await findButtonByText(wrapper, 'admin.riskControl.saveConfig').trigger('click')
    await flushPromises()

    expect(updateConfig).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith('admin.riskControl.aiPricingRateInvalid')
  })

  it('blocks saving when incremental auditing is enabled without provenance V2', async () => {
    getConfig.mockResolvedValue(aiChatConfig({
      incremental_audit_enabled: true,
      input_provenance_v2_enabled: false,
    }))

    const wrapper = mount(RiskControlView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          BaseDialog: BaseDialogStub,
          Icon: true,
          Select: true,
          Toggle: true,
          Pagination: true,
          ModelWhitelistSelector: ModelWhitelistSelectorStub,
          ProxySelector: true,
        },
      },
    })
    await flushPromises()
    await findButtonByText(wrapper, 'admin.riskControl.openSettings').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="incremental-provenance-warning"]').text()).toBe('admin.riskControl.aiIncrementalRequiresProvenance')
    await findButtonByText(wrapper, 'admin.riskControl.saveConfig').trigger('click')
    await flushPromises()

    expect(updateConfig).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith('admin.riskControl.aiIncrementalRequiresProvenance')
  })

  it('keeps incremental auditing disabled and supplies safe defaults for legacy config', async () => {
    const legacyConfig = aiChatConfig()
    delete legacyConfig.ai_chat!.incremental_audit_enabled
    delete legacyConfig.ai_chat!.input_provenance_v2_enabled
    delete legacyConfig.ai_chat!.deterministic_risk_v2_enabled
    delete legacyConfig.ai_chat!.recent_user_turns
    delete legacyConfig.ai_chat!.summary_max_chars
    delete legacyConfig.ai_chat!.full_review_threshold
    delete legacyConfig.ai_chat!.full_review_risk_delta
    delete legacyConfig.ai_chat!.periodic_full_review_turns
    delete legacyConfig.ai_chat!.full_review_max_input_chars
    delete legacyConfig.ai_chat!.fast_max_output_tokens
    delete legacyConfig.ai_chat!.full_max_output_tokens
    delete legacyConfig.ai_chat!.max_review_max_output_tokens
    delete legacyConfig.ai_chat!.audit_context_ttl_minutes
    getConfig.mockResolvedValue(legacyConfig)

    const wrapper = mount(RiskControlView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          BaseDialog: BaseDialogStub,
          Icon: true,
          Select: true,
          Toggle: true,
          Pagination: true,
          ModelWhitelistSelector: ModelWhitelistSelectorStub,
          ProxySelector: true,
        },
      },
    })
    await flushPromises()
    await findButtonByText(wrapper, 'admin.riskControl.openSettings').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="incremental-audit-status"]').text()).toBe('common.disabled')
    expect(wrapper.get<HTMLInputElement>('[data-test="ai-recent-user-turns"]').element.value).toBe('2')
    expect(wrapper.get<HTMLInputElement>('[data-test="ai-summary-max-chars"]').element.value).toBe('500')

    await findButtonByText(wrapper, 'admin.riskControl.saveConfig').trigger('click')
    await flushPromises()

    expect(updateConfig).toHaveBeenCalledWith(expect.objectContaining({
      ai_incremental_audit_enabled: false,
      ai_input_provenance_v2_enabled: true,
      ai_deterministic_risk_v2_enabled: true,
      ai_recent_user_turns: 2,
      ai_summary_max_chars: 500,
      ai_full_review_threshold: 0.4,
      ai_full_review_risk_delta: 0.15,
      ai_periodic_full_review_turns: 10,
      ai_full_review_max_input_chars: 60000,
      ai_fast_max_output_tokens: 128,
      ai_full_max_output_tokens: 1024,
      ai_max_review_max_output_tokens: 1536,
      ai_audit_context_ttl_minutes: 120,
    }))
  })

  it('renders structured trial risk fields and submits the active performance profile', async () => {
    getConfig.mockResolvedValue(aiChatConfig())
    testAPIKeys.mockResolvedValue({
      items: [],
      image_count: 0,
      audit_result: {
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
        scope: 'request',
      },
    })
    const wrapper = mount(RiskControlView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          BaseDialog: BaseDialogStub,
          Icon: true,
          Select: true,
          Toggle: true,
          Pagination: true,
          ModelWhitelistSelector: ModelWhitelistSelectorStub,
          ProxySelector: true,
        },
      },
    })
    await flushPromises()
    await findButtonByText(wrapper, 'admin.riskControl.openSettings').trigger('click')
    await flushPromises()
    await wrapper.get('textarea[placeholder="admin.riskControl.auditTestPromptPlaceholder"]').setValue('test prompt')
    await findButtonByText(wrapper, 'admin.riskControl.testContentWithStoredApiKey').trigger('click')
    await flushPromises()

    expect(testAPIKeys).toHaveBeenCalledWith(expect.objectContaining({
      ai_synchronous_budget_ms: 4800,
      ai_fast_input_chars: 12000,
      ai_fallback_input_chars: 4000,
      ai_risk_levels_enabled: true,
      ai_observe_threshold: 0.35,
    }))
    const trialPayload = testAPIKeys.mock.calls.at(-1)?.[0]
    expect(trialPayload).not.toHaveProperty('ai_incremental_audit_enabled')
    expect(trialPayload).not.toHaveProperty('ai_recent_user_turns')
    expect(trialPayload).not.toHaveProperty('ai_fast_max_output_tokens')
    expect(wrapper.get('[data-test="audit-test-risk-tier"]').text()).toContain('auditRiskTier.high')
    expect(wrapper.get('[data-test="audit-test-signals"]').text()).toContain('progressive_escalation')
    expect(wrapper.get('[data-test="audit-review-incomplete"]').text()).toContain('supplemental review timeout')
    expect(wrapper.get('[data-test="audit-test-scope"]').text()).toContain('auditTestScopeRequest')
    expect(wrapper.text()).toContain('82.0% / 70.0%')
  })

  it('reports a structured trial error instead of showing success without an audit result', async () => {
    getConfig.mockResolvedValue(aiChatConfig())
    testAPIKeys.mockResolvedValue({
      items: [],
      image_count: 0,
      audit_error: {
        code: 'audit_request_failed',
        message: 'AI audit API returned invalid JSON',
        http_status: 200,
      },
    })
    const wrapper = mount(RiskControlView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          BaseDialog: BaseDialogStub,
          Icon: true,
          Select: true,
          Toggle: true,
          Pagination: true,
          ModelWhitelistSelector: ModelWhitelistSelectorStub,
          ProxySelector: true,
        },
      },
    })
    await flushPromises()
    await findButtonByText(wrapper, 'admin.riskControl.openSettings').trigger('click')
    await flushPromises()
    await wrapper.get('textarea[placeholder="admin.riskControl.auditTestPromptPlaceholder"]').setValue('test prompt')
    await findButtonByText(wrapper, 'admin.riskControl.testContentWithStoredApiKey').trigger('click')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('AI audit API returned invalid JSON')
    expect(showSuccess).not.toHaveBeenCalled()
  })

  it('loads and saves user and upstream account audit scopes', async () => {
    getConfig.mockResolvedValue({
      ...baseConfig(),
      user_filter: { type: 'exclude', user_ids: [7] },
      account_filter: { type: 'include', account_ids: [11] },
    })
    getUserById.mockResolvedValue({ id: 7, email: 'admin@example.com', username: 'admin', role: 'admin' })
    getAccountById.mockResolvedValue({ id: 11, name: 'Protected Pro', platform: 'openai', type: 'oauth' })

    const wrapper = mount(RiskControlView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          BaseDialog: BaseDialogStub,
          Icon: true,
          Select: true,
          Toggle: true,
          Pagination: true,
          ModelWhitelistSelector: ModelWhitelistSelectorStub,
          ProxySelector: true,
        },
      },
    })

    await flushPromises()
    await findButtonByText(wrapper, 'admin.riskControl.openSettings').trigger('click')
    await findButtonByText(wrapper, 'admin.riskControl.tabs.scope').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="user-filter-exclude"]').classes()).toContain('border-primary-300')
    expect(wrapper.get('[data-test="account-filter-include"]').classes()).toContain('border-primary-300')
    expect(wrapper.text()).toContain('admin@example.com')
    expect(wrapper.text()).toContain('Protected Pro')

    await findButtonByText(wrapper, 'admin.riskControl.saveConfig').trigger('click')
    await flushPromises()

    expect(updateConfig).toHaveBeenCalledWith(expect.objectContaining({
      user_filter: { type: 'exclude', user_ids: [7] },
      account_filter: { type: 'include', account_ids: [11] },
    }))
  })

  it('saves a selected Spark shadow as its parent account scope', async () => {
    listAccounts.mockResolvedValue({
      items: [{
        id: 302,
        name: 'Protected Pro Spark',
        platform: 'openai',
        type: 'oauth',
        parent_account_id: 202,
        quota_dimension: 'spark',
      }],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })

    const wrapper = mount(RiskControlView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          BaseDialog: BaseDialogStub,
          Icon: true,
          Select: true,
          Toggle: true,
          Pagination: true,
          ModelWhitelistSelector: ModelWhitelistSelectorStub,
          ProxySelector: true,
        },
      },
    })

    await flushPromises()
    await findButtonByText(wrapper, 'admin.riskControl.openSettings').trigger('click')
    await findButtonByText(wrapper, 'admin.riskControl.tabs.scope').trigger('click')
    await wrapper.get('[data-test="account-filter-include"]').trigger('click')
    await wrapper.get('[data-test="account-scope-search"]').setValue('spark')
    await new Promise((resolve) => setTimeout(resolve, 350))
    await flushPromises()
    await wrapper.get('[data-test="account-scope-option-202"]').trigger('click')
    await findButtonByText(wrapper, 'admin.riskControl.saveConfig').trigger('click')
    await flushPromises()

    expect(updateConfig).toHaveBeenCalledWith(expect.objectContaining({
      account_filter: { type: 'include', account_ids: [202] },
    }))
  })

  it('warns when an include-only user or account scope is empty', async () => {
    const wrapper = mount(RiskControlView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          BaseDialog: BaseDialogStub,
          Icon: true,
          Select: true,
          Toggle: true,
          Pagination: true,
          ModelWhitelistSelector: ModelWhitelistSelectorStub,
          ProxySelector: true,
        },
      },
    })

    await flushPromises()
    await findButtonByText(wrapper, 'admin.riskControl.openSettings').trigger('click')
    await findButtonByText(wrapper, 'admin.riskControl.tabs.scope').trigger('click')
    await wrapper.get('[data-test="user-filter-include"]').trigger('click')
    await wrapper.get('[data-test="account-filter-include"]').trigger('click')

    expect(wrapper.get('[data-test="user-filter-empty-warning"]').text()).toContain('admin.riskControl.userFilterEmptyIncludeWarning')
    expect(wrapper.get('[data-test="account-filter-empty-warning"]').text()).toContain('admin.riskControl.accountFilterEmptyIncludeWarning')
  })

  it('submits edited risk control thresholds when saving moderation config', async () => {
    const wrapper = mount(RiskControlView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          BaseDialog: BaseDialogStub,
          Icon: true,
          Select: true,
          Toggle: true,
          Pagination: true,
          ModelWhitelistSelector: ModelWhitelistSelectorStub,
          ProxySelector: true,
        },
      },
    })

    await flushPromises()

    await findButtonByText(wrapper, 'admin.riskControl.openSettings').trigger('click')
    await findButtonByText(wrapper, 'admin.riskControl.tabs.riskThresholds').trigger('click')
    await wrapper.get('[data-test="risk-threshold-sexual"]').setValue('72')
    await wrapper.get('[data-test="risk-threshold-harassment"]').setValue('99')
    await findButtonByText(wrapper, 'admin.riskControl.saveConfig').trigger('click')
    await flushPromises()

    expect(updateConfig).toHaveBeenCalledWith(expect.objectContaining({
      thresholds: expect.objectContaining({
        sexual: 0.72,
        harassment: 0.99,
      }),
    }))
    expect(showError).not.toHaveBeenCalled()
  })

  it('describes worker runtime as async audit and pre-block record processing', async () => {
    getStatus.mockResolvedValue({
      ...runtimeStatus(),
      mode: 'observe',
      processed: 12,
      queue_length: 2,
    })

    const wrapper = mount(RiskControlView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          BaseDialog: BaseDialogStub,
          Icon: true,
          Select: true,
          Toggle: true,
          Pagination: true,
          ModelWhitelistSelector: ModelWhitelistSelectorStub,
          ProxySelector: true,
        },
      },
    })

    await flushPromises()

    expect(wrapper.text()).toContain('admin.riskControl.workerStatusHint')
    expect(wrapper.text()).not.toContain('admin.riskControl.preBlockSyncStatus')
    expect(wrapper.text()).toContain('admin.riskControl.records')
    expect(wrapper.text()).toContain('12')
    expect(wrapper.text()).toContain('2 / 32,768')
  })

  it('shows pre-block synchronous moderation metrics separately from worker queue', async () => {
    getStatus.mockResolvedValue({
      ...runtimeStatus(),
      pre_block_active: 2,
      pre_block_checked: 128,
      pre_block_allowed: 120,
      pre_block_blocked: 8,
      pre_block_errors: 1,
      pre_block_avg_latency_ms: 86,
      pre_block_api_key_active: 2,
      pre_block_api_key_available_count: 2,
      pre_block_api_key_total_calls: 128,
      active_workers: 3,
      worker_count: 7,
      pre_block_api_key_loads: [
        {
          index: 0,
          key_hash: 'hash-one',
          masked: 'sk-...one',
          status: 'ok',
          active: 1,
          total: 72,
          success: 70,
          errors: 2,
          avg_latency_ms: 84,
          last_latency_ms: 80,
          last_http_status: 200,
        },
        {
          index: 1,
          key_hash: 'hash-two',
          masked: 'sk-...two',
          status: 'ok',
          active: 1,
          total: 56,
          success: 56,
          errors: 0,
          avg_latency_ms: 90,
          last_latency_ms: 92,
          last_http_status: 200,
        },
      ],
    })

    const wrapper = mount(RiskControlView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          BaseDialog: BaseDialogStub,
          Icon: true,
          Select: true,
          Toggle: true,
          Pagination: true,
          ModelWhitelistSelector: ModelWhitelistSelectorStub,
          ProxySelector: true,
        },
      },
    })

    await flushPromises()

    expect(wrapper.text()).toContain('admin.riskControl.preBlockSyncStatus')
    expect(wrapper.text()).toContain('admin.riskControl.preBlockSyncHint')
    expect(wrapper.text()).not.toContain('admin.riskControl.workerStatus')
    expect(wrapper.text()).toContain('admin.riskControl.records')
    expect(wrapper.text()).toContain('128')
    expect(wrapper.text()).toContain('120')
    expect(wrapper.text()).toContain('8')
    expect(wrapper.text()).toContain('86 ms')
    expect(wrapper.text()).toContain('admin.riskControl.preBlockAPIKeyLoad')
    expect(wrapper.text()).toContain('sk-...one')
    expect(wrapper.text()).toContain('sk-...two')
    expect(wrapper.text()).toContain('72')
    expect(wrapper.text()).toContain('56')
    expect(wrapper.text()).toContain('同步并发 2 / 可用 Key 2，累计 128 次，worker：3 / 7')

    const runtimeCards = wrapper.get('[data-test="pre-block-runtime-cards"]')
    const syncCard = wrapper.get('[data-test="pre-block-sync-card"]')
    const apiKeyLoadCard = wrapper.get('[data-test="pre-block-api-key-load-card"]')

    expect(runtimeCards.classes()).toEqual(expect.arrayContaining([
      'grid',
      'grid-cols-1',
      'xl:grid-cols-[minmax(0,520px)_minmax(0,1fr)]',
    ]))
    expect(syncCard.element.parentElement).toBe(runtimeCards.element)
    expect(apiKeyLoadCard.element.parentElement).toBe(runtimeCards.element)
    expect(syncCard.classes()).toContain('card')
    expect(apiKeyLoadCard.classes()).toContain('card')
    expect(syncCard.get('h2').text()).toBe('admin.riskControl.preBlockSyncStatus')
    expect(syncCard.text()).toContain('admin.riskControl.preBlockSyncHint')
    expect(apiKeyLoadCard.get('h2').text()).toBe('admin.riskControl.preBlockAPIKeyLoad')
    expect(apiKeyLoadCard.text()).toContain('admin.riskControl.preBlockAPIKeyLoadHint')
    expect(wrapper.get('[data-test="pre-block-api-key-load-list"]').classes()).toEqual(expect.arrayContaining([
      'max-h-[280px]',
      'overflow-y-auto',
    ]))
  })

  it('renders a skipped audit as skipped even when diagnostic error text is present', async () => {
    listLogs.mockResolvedValue({
      items: [moderationLog({
        action: 'skip',
        flagged: false,
        audit_status: 'skipped',
        audit_code: 'empty_content',
        audit_retryable: false,
        error: 'request contains no auditable content',
        side_effect_status: 'not_applicable',
        notification_status: 'not_required',
        auto_banned: false,
        moderation_ban_active: false,
        user_status: 'active',
      })],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })

    const wrapper = mount(RiskControlView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          BaseDialog: BaseDialogStub,
          Icon: true,
          Select: true,
          Toggle: true,
          Pagination: true,
          ModelWhitelistSelector: ModelWhitelistSelectorStub,
          ProxySelector: true,
        },
      },
    })
    await flushPromises()

    expect(wrapper.get('[data-test="audit-status-skipped"]').text()).toBe('admin.riskControl.auditStatusSkipped')
    expect(wrapper.text()).not.toContain('admin.riskControl.auditStatusError')
    expect(wrapper.text()).toContain('admin.riskControl.sideEffectStatus.not_applicable')
    expect(wrapper.text()).toContain('admin.riskControl.notificationStatus.not_required')
  })

  it('shows complete per-request audit usage, provenance, and cache diagnostics', async () => {
    listLogs.mockResolvedValue({
      items: [moderationLog({
        audit_details: {
          total_latency_ms: 1234,
          extraction_latency_ms: 3,
          provenance_latency_ms: 2,
          deterministic_latency_ms: 4,
          verdict_cache_latency_ms: 5,
          context_load_latency_ms: 6,
          fast_build_latency_ms: 7,
          review_build_latency_ms: 8,
          provider_latency_ms: 905,
          postprocess_latency_ms: 12,
          audit_stage: 'full',
          escalation_reasons: ['periodic_review'],
          session_source: 'prompt_cache_key',
          turn_count: 8,
          input_chars: 12000,
          prompt_tokens: 4000,
          cached_input_tokens: 3000,
          uncached_input_tokens: 1000,
          output_tokens: 200,
          sub2api_result_cache_hit: false,
          provider_prefix_cache_ratio: 0.75,
          review_complete: true,
          audit_target_kind: 'user_request',
          audit_target_source: 'end_user',
          has_explicit_user_turn: true,
          trusted_client: false,
          audit_target_excerpt: 'redacted target',
          supporting_context_excerpt: 'redacted context',
          trusted_signals: ['strict_client_identity', 'originator_codex_cli'],
          ignored_metadata: ['ambient_ui'],
          audit_key_hash: 'audit-key-hash',
          input_hash: 'a'.repeat(64),
          hash_scope: 'policy:v2',
          hash_state: 'confirmed',
          hash_promotion_reason: 'full_review_confirmed',
          policy_version: 'vote-ai-risk-v2',
          prefix_epoch: 3,
          prefix_continuity: false,
          prefix_break_reason: 'history_rewritten',
          input_truncated: true,
          local_rule_level: 'confirmed',
          local_rule_match: {
            rule_id: 'auth-bypass-action',
            rule_version: 'v2.1',
            level: 'confirmed',
            target_kind: 'user_request',
            target_source: 'end_user',
            matched_intent: ['bypass'],
            matched_target: ['account'],
            matched_action: ['generate_script'],
            matched_excerpt: '脱敏规则摘要',
            lexical_types: ['intent', 'action'],
            negation_detected: false,
            defensive_detected: false,
            metadata_excluded: ['ambient_ui'],
          },
          stages: [
            {
              stage: 'fast',
              provider_called: true,
              result_cache_hit: false,
              usage_known: true,
              failed: false,
              input_chars: 12000,
              latency_ms: 850,
              prompt_tokens: 4000,
              cached_input_tokens: 3000,
              uncached_input_tokens: 1000,
              output_tokens: 200,
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
      })],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })

    const wrapper = mount(RiskControlView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          BaseDialog: BaseDialogStub,
          Icon: true,
          Select: true,
          Toggle: true,
          Pagination: true,
          ModelWhitelistSelector: ModelWhitelistSelectorStub,
          ProxySelector: true,
        },
      },
    })
    await flushPromises()
    expect(wrapper.text()).toContain('1,234 ms')
    await wrapper.get('[data-test="open-input-detail"]').trigger('click')

    const text = wrapper.text()
    expect(text).toContain('redacted target')
    expect(text).toContain('redacted context')
    expect(text).toContain('prompt_cache_key / 8')
    expect(text).toContain('12,000')
    expect(text).toContain('input=4,000;cached=3,000;uncached=1,000;output=200')
    expect(text).toContain('75.0%')
    expect(text).toContain('admin.riskControl.cacheMiss')
    expect(text).toContain('admin.riskControl.auditReviewComplete')
    expect(text).toContain('admin.riskControl.auditExplicitUserTurn')
    expect(text).toContain('admin.riskControl.auditTrustedClient')
    expect(wrapper.get('[data-test="audit-diagnostic-usage-completeness"]').text()).toContain('admin.riskControl.usageComplete')
    expect(wrapper.get('[data-test="audit-diagnostic-latency-total"]').text()).toContain('1,234 ms')
    expect(wrapper.get('[data-test="audit-diagnostic-latency-fast-build"]').text()).toContain('7 ms')
    expect(wrapper.get('[data-test="audit-diagnostic-latency-review-build"]').text()).toContain('8 ms')
    expect(wrapper.get('[data-test="audit-diagnostic-latency-provider"]').text()).toContain('905 ms')
    expect(wrapper.get('[data-test="audit-diagnostic-input-hash"]').text()).toContain('a'.repeat(64))
    expect(wrapper.get('[data-test="audit-diagnostic-policy-version"]').text()).toContain('vote-ai-risk-v2')
    expect(wrapper.get('[data-test="audit-diagnostic-audit-key-hash"]').text()).toContain('audit-key-hash')
    expect(wrapper.get('[data-test="audit-diagnostic-prefix"]').text()).toContain('3 / common.no')
    expect(wrapper.get('[data-test="audit-diagnostic-prefix-break-reason"]').text()).toContain('history_rewritten')
    expect(wrapper.get('[data-test="audit-diagnostic-input-truncated"]').text()).toContain('common.yes')
    expect(wrapper.get('[data-test="audit-diagnostic-explicit-user"]').text()).toContain('common.yes')
    expect(wrapper.get('[data-test="audit-diagnostic-trusted-client"]').text()).toContain('common.no')
    expect(wrapper.get('[data-test="audit-diagnostic-trusted-signals"]').text()).toContain('strict_client_identity')
    expect(wrapper.get('[data-test="audit-diagnostic-ignored-metadata"]').text()).toContain('ambient_ui')
    expect(wrapper.get('[data-test="audit-diagnostic-local-rule-identity"]').text()).toContain('auth-bypass-action / v2.1')
    expect(wrapper.get('[data-test="audit-diagnostic-local-rule-level"]').text()).toContain('confirmed')
    expect(wrapper.get('[data-test="audit-diagnostic-local-rule-intent"]').text()).toContain('bypass')
    expect(wrapper.get('[data-test="audit-diagnostic-local-rule-target"]').text()).toContain('account')
    expect(wrapper.get('[data-test="audit-diagnostic-local-rule-action"]').text()).toContain('generate_script')
    expect(wrapper.get('[data-test="audit-diagnostic-local-rule-excerpt"]').text()).toContain('脱敏规则摘要')
    expect(wrapper.get('[data-test="audit-diagnostic-lexical-types"]').text()).toContain('intent, action')
    expect(wrapper.get('[data-test="audit-diagnostic-negation-detected"]').text()).toContain('common.no')
    expect(wrapper.get('[data-test="audit-diagnostic-defensive-detected"]').text()).toContain('common.no')
    expect(wrapper.get('[data-test="audit-stage-fast"]').text()).toContain('admin.riskControl.usageComplete')
    expect(wrapper.get('[data-test="audit-stage-full"]').text()).toContain('admin.riskControl.cacheHit')
  })

  it('shows the first prefix sample as a baseline instead of a break', async () => {
    listLogs.mockResolvedValue({
      items: [moderationLog({
        audit_details: {
          audit_stage: 'full',
          audit_target_excerpt: 'baseline target',
          prefix_epoch: 1,
          prefix_baseline: true,
        },
      })],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })

    const wrapper = mount(RiskControlView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          BaseDialog: BaseDialogStub,
          Icon: true,
          Select: true,
          Toggle: true,
          Pagination: true,
          ModelWhitelistSelector: ModelWhitelistSelectorStub,
          ProxySelector: true,
        },
      },
    })
    await flushPromises()
    await wrapper.get('[data-test="open-input-detail"]').trigger('click')

    expect(wrapper.get('[data-test="audit-diagnostic-prefix"]').text()).toContain('1 / admin.riskControl.auditPrefixBaseline')
    expect(wrapper.get('[data-test="audit-diagnostic-prefix-break-reason"]').text()).toContain('admin.riskControl.auditPrefixBaseline')
    expect(wrapper.get('[data-test="audit-diagnostic-prefix"]').text()).not.toContain('common.no')
  })

  it('does not present partial audit token usage as zero or a cache percentage', async () => {
    listLogs.mockResolvedValue({
      items: [moderationLog({
        audit_details: {
          audit_stage: 'fast',
          session_source: 'none',
          prompt_tokens: 100,
          cached_input_tokens: 80,
          output_tokens: 10,
          usage_unknown: false,
          audit_target_excerpt: 'partial usage target',
        },
      })],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })

    const wrapper = mount(RiskControlView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          BaseDialog: BaseDialogStub,
          Icon: true,
          Select: true,
          Toggle: true,
          Pagination: true,
          ModelWhitelistSelector: ModelWhitelistSelectorStub,
          ProxySelector: true,
        },
      },
    })
    await flushPromises()
    await wrapper.get('[data-test="open-input-detail"]').trigger('click')

    const text = wrapper.text()
    expect(text).toContain('admin.riskControl.usageUnknown')
    expect(text).toContain('common.unknown')
    expect(text).not.toContain('input=100;cached=80')
    expect(text).not.toContain('80.0%')
  })

  it('shows legacy audit diagnostics as unknown instead of invented zero or false values', async () => {
    listLogs.mockResolvedValue({
      items: [moderationLog({
        audit_details: {
          audit_stage: 'fast',
          audit_target_excerpt: 'legacy audit target',
        },
      })],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })

    const wrapper = mount(RiskControlView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          BaseDialog: BaseDialogStub,
          Icon: true,
          Select: true,
          Toggle: true,
          Pagination: true,
          ModelWhitelistSelector: ModelWhitelistSelectorStub,
          ProxySelector: true,
        },
      },
    })
    await flushPromises()
    await wrapper.get('[data-test="open-input-detail"]').trigger('click')

    expect(wrapper.get('[data-test="audit-diagnostic-session"]').text()).toContain('common.unknown / common.unknown')
    expect(wrapper.get('[data-test="audit-diagnostic-prefix"]').text()).toContain('common.unknown / common.unknown')
    expect(wrapper.get('[data-test="audit-diagnostic-input-truncated"]').text()).toContain('common.unknown')
    expect(wrapper.get('[data-test="audit-diagnostic-explicit-user"]').text()).toContain('common.unknown')
    expect(wrapper.get('[data-test="audit-diagnostic-trusted-client"]').text()).toContain('common.unknown')
    expect(wrapper.get('[data-test="audit-diagnostic-usage-completeness"]').text()).toContain('common.unknown')
    expect(wrapper.get('[data-test="supporting-context"]').text()).toContain('common.unknown')
    expect(wrapper.get('[data-test="audit-diagnostic-session"]').text()).not.toContain('none / 0')
    expect(wrapper.get('[data-test="audit-diagnostic-prefix"]').text()).not.toContain('0 / common.no')
  })

  it('shows known false provenance and truncation diagnostics as no', async () => {
    listLogs.mockResolvedValue({
      items: [moderationLog({
        audit_details: {
          audit_stage: 'fast',
          audit_target_excerpt: 'current audit target',
          has_explicit_user_turn: false,
          trusted_client: false,
          input_truncated: false,
        },
      })],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })

    const wrapper = mount(RiskControlView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          BaseDialog: BaseDialogStub,
          Icon: true,
          Select: true,
          Toggle: true,
          Pagination: true,
          ModelWhitelistSelector: ModelWhitelistSelectorStub,
          ProxySelector: true,
        },
      },
    })
    await flushPromises()
    await wrapper.get('[data-test="open-input-detail"]').trigger('click')

    expect(wrapper.get('[data-test="audit-diagnostic-explicit-user"]').text()).toContain('common.no')
    expect(wrapper.get('[data-test="audit-diagnostic-trusted-client"]').text()).toContain('common.no')
    expect(wrapper.get('[data-test="audit-diagnostic-input-truncated"]').text()).toContain('common.no')
  })

  it('accepts complete legacy provider usage even when stage details are absent', async () => {
    listLogs.mockResolvedValue({
      items: [moderationLog({
        audit_details: {
          audit_stage: 'full',
          provider_applicable: true,
          result_cache_applicable: true,
          review_applicable: true,
          prompt_tokens: 100,
          cached_input_tokens: 80,
          uncached_input_tokens: 20,
          output_tokens: 5,
          usage_unknown: false,
          sub2api_result_cache_hit: false,
          review_complete: true,
          audit_target_excerpt: 'legacy provider usage',
        },
      })],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })
    const wrapper = mount(RiskControlView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          BaseDialog: BaseDialogStub,
          Icon: true,
          Select: true,
          Toggle: true,
          Pagination: true,
          ModelWhitelistSelector: ModelWhitelistSelectorStub,
          ProxySelector: true,
        },
      },
    })
    await flushPromises()
    await wrapper.get('[data-test="open-input-detail"]').trigger('click')

    expect(wrapper.get('[data-test="audit-diagnostic-tokens"]').text()).toContain('input=100;cached=80;uncached=20;output=5')
    expect(wrapper.get('[data-test="audit-diagnostic-usage-completeness"]').text()).toContain('admin.riskControl.usageComplete')
    expect(wrapper.get('[data-test="audit-diagnostic-cache"]').text()).toContain('admin.riskControl.cacheMiss')
  })

  it('shows provider, cache, and review diagnostics as not applicable for local skips', async () => {
    listLogs.mockResolvedValue({
      items: [moderationLog({
        action: 'skip',
        audit_status: 'skipped',
        audit_code: 'no_new_user_intent',
        audit_details: {
          provider_applicable: false,
          result_cache_applicable: false,
          review_applicable: false,
          sub2api_result_cache_hit: false,
          review_complete: false,
          audit_target_excerpt: 'metadata-only request',
        },
      })],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })
    const wrapper = mount(RiskControlView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          BaseDialog: BaseDialogStub,
          Icon: true,
          Select: true,
          Toggle: true,
          Pagination: true,
          ModelWhitelistSelector: ModelWhitelistSelectorStub,
          ProxySelector: true,
        },
      },
    })
    await flushPromises()
    await wrapper.get('[data-test="open-input-detail"]').trigger('click')

    expect(wrapper.get('[data-test="audit-diagnostic-tokens"]').text()).toContain('admin.riskControl.auditStageNoProviderUsage')
    expect(wrapper.get('[data-test="audit-diagnostic-usage-completeness"]').text()).toContain('admin.riskControl.auditStageNotApplicable')
    expect(wrapper.get('[data-test="audit-diagnostic-cache"]').text()).toContain('admin.riskControl.auditStageNotApplicable')
    expect(wrapper.get('[data-test="audit-diagnostic-review-complete"]').text()).toContain('admin.riskControl.auditStageNotApplicable')
  })

  it.each([
    {
      label: 'empty JSON',
      auditDetails: {},
    },
    {
      label: 'serialized zero-value booleans',
      auditDetails: {
        sub2api_result_cache_hit: false,
        review_complete: false,
        has_explicit_user_turn: false,
        trusted_client: false,
      },
    },
  ])('does not expand $label audit details from historical rows', async ({ auditDetails }) => {
    listLogs.mockResolvedValue({
      items: [moderationLog({ audit_details: auditDetails })],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })

    const wrapper = mount(RiskControlView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          BaseDialog: BaseDialogStub,
          Icon: true,
          Select: true,
          Toggle: true,
          Pagination: true,
          ModelWhitelistSelector: ModelWhitelistSelectorStub,
          ProxySelector: true,
        },
      },
    })
    await flushPromises()
    await wrapper.get('[data-test="open-input-detail"]').trigger('click')

    expect(wrapper.text()).toContain('test input')
    expect(wrapper.find('[data-test="supporting-context"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="audit-stage-details"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="audit-diagnostic-cache"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('admin.riskControl.auditDiagnostics')
  })

  it('deletes the current record hash only after confirmation', async () => {
    const inputHash = 'b'.repeat(64)
    listLogs.mockResolvedValue({
      items: [moderationLog({
        audit_details: {
          input_hash: inputHash,
          audit_target_excerpt: 'false positive target',
        },
      })],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })
    const confirmSpy = vi.spyOn(window, 'confirm')
      .mockReturnValueOnce(false)
      .mockReturnValueOnce(true)

    const wrapper = mount(RiskControlView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          BaseDialog: BaseDialogStub,
          Icon: true,
          Select: true,
          Toggle: true,
          Pagination: true,
          ModelWhitelistSelector: ModelWhitelistSelectorStub,
          ProxySelector: true,
        },
      },
    })
    await flushPromises()
    await wrapper.get('[data-test="open-input-detail"]').trigger('click')

    const deleteButton = wrapper.get('[data-test="input-detail-delete-flagged-hash"]')
    expect(deleteButton.attributes('disabled')).toBeUndefined()
    await deleteButton.trigger('click')
    expect(deleteFlaggedHash).not.toHaveBeenCalled()

    await deleteButton.trigger('click')
    await flushPromises()
    expect(confirmSpy).toHaveBeenCalledTimes(2)
    expect(deleteFlaggedHash).toHaveBeenCalledWith(inputHash)
    expect(showSuccess).toHaveBeenCalledWith('admin.riskControl.flaggedHashDeleted')

    confirmSpy.mockRestore()
  })

  it('uses restore and clear risk as the default moderation unban mode', async () => {
    listLogs.mockResolvedValue({
      items: [moderationLog()],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })

    const wrapper = mount(RiskControlView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          BaseDialog: BaseDialogStub,
          Icon: true,
          Select: true,
          Toggle: true,
          Pagination: true,
          ModelWhitelistSelector: ModelWhitelistSelectorStub,
          ProxySelector: true,
        },
      },
    })
    await flushPromises()
    await wrapper.get('[data-test="open-moderation-unban"]').trigger('click')
    await flushPromises()

    const clearRiskMode = wrapper.get<HTMLInputElement>('[data-test="unban-mode-restore_and_clear_risk"]')
    expect(clearRiskMode.element.checked).toBe(true)
    await wrapper.get('[data-test="confirm-moderation-unban"]').trigger('click')
    await flushPromises()

    expect(unbanUser).toHaveBeenCalledWith(7, { mode: 'restore_and_clear_risk' })
    expect(showSuccess).toHaveBeenCalledWith('admin.riskControl.unbanSuccessCleared')
  })

  it('retries risk-state cleanup after a partial unban and closes on success', async () => {
    listLogs.mockResolvedValue({
      items: [moderationLog()],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })
    unbanUser.mockResolvedValueOnce({
      user_id: 7,
      status: 'active',
      mode: 'restore_and_clear_risk',
      restored: true,
      risk_state_cleared: false,
      warning: 'Redis cleanup unavailable',
    }).mockResolvedValueOnce({
      user_id: 7,
      status: 'active',
      mode: 'clear_risk_only',
      restored: false,
      risk_state_cleared: true,
    })

    const wrapper = mount(RiskControlView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          BaseDialog: BaseDialogStub,
          Icon: true,
          Select: true,
          Toggle: true,
          Pagination: true,
          ModelWhitelistSelector: ModelWhitelistSelectorStub,
          ProxySelector: true,
        },
      },
    })
    await flushPromises()
    await wrapper.get('[data-test="open-moderation-unban"]').trigger('click')
    await wrapper.get('[data-test="confirm-moderation-unban"]').trigger('click')
    await flushPromises()

    expect(showWarning).toHaveBeenCalledWith('partial success: Redis cleanup unavailable')
    expect(showSuccess).not.toHaveBeenCalled()
    expect(wrapper.get('[data-test="unban-partial-warning"]').text()).toContain('Redis cleanup unavailable')
    expect(wrapper.find('[data-test="confirm-moderation-unban"]').exists()).toBe(false)
    await wrapper.get('[data-test="retry-moderation-risk-clear"]').trigger('click')
    await flushPromises()

    expect(unbanUser).toHaveBeenNthCalledWith(2, 7, { mode: 'clear_risk_only' })
    expect(showSuccess).toHaveBeenCalledWith('admin.riskControl.riskStateCleanupRetrySuccess')
    expect(wrapper.find('[data-test="unban-partial-warning"]').exists()).toBe(false)
  })

  it('does not offer moderation unban for an account disabled outside risk control', async () => {
    listLogs.mockResolvedValue({
      items: [moderationLog({
        moderation_ban_active: false,
        unban_block_reason: 'manual administrator suspension',
      })],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })

    const wrapper = mount(RiskControlView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          BaseDialog: BaseDialogStub,
          Icon: true,
          Select: true,
          Toggle: true,
          Pagination: true,
          ModelWhitelistSelector: ModelWhitelistSelectorStub,
          ProxySelector: true,
        },
      },
    })
    await flushPromises()

    expect(wrapper.find('[data-test="open-moderation-unban"]').exists()).toBe(false)
    expect(wrapper.get('[data-test="unban-unavailable-reason"]').text()).toContain('manual administrator suspension')
    expect(unbanUser).not.toHaveBeenCalled()
  })
})
