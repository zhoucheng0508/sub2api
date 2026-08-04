import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import ModerationAuditStatusBadge from '../ModerationAuditStatusBadge.vue'
import ModerationSideEffectsStatus from '../ModerationSideEffectsStatus.vue'
import ModerationUnbanDialog from '../ModerationUnbanDialog.vue'
import type { ContentModerationLog } from '@/api/admin/riskControl'

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const BaseDialogStub = {
  props: ['show'],
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
}

const moderationLog = (overrides: Partial<ContentModerationLog> = {}): ContentModerationLog => ({
  id: 1,
  request_id: 'req-1',
  user_id: 7,
  user_email: 'user@example.com',
  api_key_id: 9,
  api_key_name: 'key',
  group_id: 11,
  group_name: 'mixed',
  endpoint: '/v1/responses',
  provider: 'openai',
  model: 'gpt-5.6-terra',
  mode: 'pre_block',
  action: 'block',
  flagged: true,
  highest_category: 'cyber_abuse',
  highest_score: 0.9,
  matched_keyword: '',
  category_scores: { cyber_abuse: 0.9 },
  threshold_snapshot: { cyber_abuse: 0.7 },
  input_excerpt: 'input',
  upstream_latency_ms: 100,
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

describe('ModerationAuditStatusBadge', () => {
  it('uses audit status instead of diagnostic error presence', () => {
    const wrapper = mount(ModerationAuditStatusBadge, {
      props: { status: 'skipped', action: 'skip', code: 'empty_content' },
    })

    expect(wrapper.get('[data-test="audit-status-skipped"]').text()).toBe('admin.riskControl.auditStatusSkipped')
    expect(wrapper.text()).toContain('empty_content')
    expect(wrapper.text()).not.toContain('admin.riskControl.auditStatusError')
  })

  it('shows retryability for a typed audit error', () => {
    const wrapper = mount(ModerationAuditStatusBadge, {
      props: { status: 'error', action: 'error', code: 'audit_timeout', retryable: true },
    })

    expect(wrapper.get('[data-test="audit-status-error"]').text()).toBe('admin.riskControl.auditStatusError')
    expect(wrapper.text()).toContain('audit_timeout')
    expect(wrapper.text()).toContain('admin.riskControl.auditRetryable')
  })
})

describe('ModerationSideEffectsStatus', () => {
  it('distinguishes deduplicated notifications from failed or unsent notifications', () => {
    const wrapper = mount(ModerationSideEffectsStatus, {
      props: {
        sideEffectStatus: 'completed',
        notificationStatus: 'deduplicated',
        moderationBanActive: true,
      },
    })

    expect(wrapper.text()).toContain('admin.riskControl.sideEffectStatus.completed')
    expect(wrapper.text()).toContain('admin.riskControl.notificationStatus.deduplicated')
    expect(wrapper.text()).toContain('admin.riskControl.moderationBanActive')
  })
})

describe('ModerationUnbanDialog', () => {
  it('defaults to restoring the account and clearing short-term risk', async () => {
    const wrapper = mount(ModerationUnbanDialog, {
      props: { show: true, row: moderationLog() },
      global: { stubs: { BaseDialog: BaseDialogStub } },
    })

    const clearRisk = wrapper.get<HTMLInputElement>('[data-test="unban-mode-restore_and_clear_risk"]')
    expect(clearRisk.element.checked).toBe(true)
    await wrapper.get('[data-test="confirm-moderation-unban"]').trigger('click')

    expect(wrapper.emitted('confirm')).toEqual([['restore_and_clear_risk']])
  })

  it('allows selecting restore-only explicitly', async () => {
    const wrapper = mount(ModerationUnbanDialog, {
      props: { show: true, row: moderationLog() },
      global: { stubs: { BaseDialog: BaseDialogStub } },
    })

    await wrapper.get('[data-test="unban-mode-restore_only"]').setValue(true)
    await wrapper.get('[data-test="confirm-moderation-unban"]').trigger('click')

    expect(wrapper.emitted('confirm')).toEqual([['restore_only']])
  })

  it('disables confirmation when the account is not under a moderation-owned ban', () => {
    const wrapper = mount(ModerationUnbanDialog, {
      props: {
        show: true,
        row: moderationLog({
          moderation_ban_active: false,
          unban_block_reason: 'manual suspension',
        }),
      },
      global: { stubs: { BaseDialog: BaseDialogStub } },
    })

    expect(wrapper.get('[data-test="unban-ownership-status"]').text()).toContain('manual suspension')
    expect(wrapper.get('[data-test="confirm-moderation-unban"]').attributes('disabled')).toBeDefined()
  })

  it('offers a risk-only cleanup retry after the account was restored', async () => {
    const wrapper = mount(ModerationUnbanDialog, {
      props: {
        show: true,
        row: moderationLog({ moderation_ban_active: false, user_status: 'active' }),
        warning: 'Redis cleanup unavailable',
      },
      global: { stubs: { BaseDialog: BaseDialogStub } },
    })

    expect(wrapper.get('[data-test="unban-partial-warning"]').text()).toContain('Redis cleanup unavailable')
    expect(wrapper.find('[data-test="confirm-moderation-unban"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="unban-ownership-status"]').exists()).toBe(false)
    await wrapper.get('[data-test="retry-moderation-risk-clear"]').trigger('click')

    expect(wrapper.emitted('confirm')).toEqual([['clear_risk_only']])
  })
})
