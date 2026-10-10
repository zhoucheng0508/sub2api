import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AccountBalanceCell from '../AccountBalanceCell.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('AccountBalanceCell', () => {
  it('distinguishes zero from unavailable and exposes refresh', async () => {
    const wrapper = mount(AccountBalanceCell, { props: { eligible: true, state: {
      data: { status: 'ok', amount: 0, unit: 'USD', scope: 'wallet', checked_at: '2026-10-10T00:00:00Z' },
      loading: false, failed: false, fetchedAt: 0, identity: 'a'
    } } })
    expect(wrapper.text()).toContain('0.00 USD')
    expect(wrapper.text()).toContain('upstreamBalance.wallet')
    await wrapper.get('button').trigger('click')
    expect(wrapper.emitted('refresh')).toHaveLength(1)
    await wrapper.setProps({ state: undefined, eligible: false })
    expect(wrapper.text()).toContain('upstreamBalance.unavailable')
    expect(wrapper.text()).not.toContain('0.00')
    expect(wrapper.find('button').exists()).toBe(false)
  })
})
