import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import FirstUseJourney from './FirstUseJourney.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const render = (props = {}) => mount(FirstUseJourney, { props: { phase: 'empty', balance: 0, onlineAvailable: true, canRedeem: true, ...props }, global: { stubs: { Icon: true, RouterLink: { props: ['to'], template: '<a><slot /></a>' } } } })
describe('first-use journey', () => {
  it('offers payment and redemption as alternatives before key setup', () => {
    const view = render()
    expect(view.get('[data-testid="journey-step-funding"]').attributes('aria-current')).toBe('step')
    expect(view.find('[data-testid="journey-purchase"]').exists()).toBe(true)
    expect(view.find('[data-testid="journey-redeem"]').exists()).toBe(true)
    view.unmount()
  })
  it('does not advertise an unavailable online payment method', () => {
    const view = render({ onlineAvailable: false })
    expect(view.find('[data-testid="journey-purchase"]').exists()).toBe(false)
    expect(view.find('[data-testid="journey-redeem"]').exists()).toBe(true)
    expect(view.text()).toContain('firstUseJourney.offlineHint')
    view.unmount()
  })
  it.each(['balance', 'subscription'])('does not require additional funding for %s', (phase) => {
    const view = render({ phase, balance: phase === 'balance' ? 5 : 0 })
    expect(view.find('[data-testid="journey-purchase"]').exists()).toBe(false)
    expect(view.get('[data-testid="journey-step-key"]').attributes('aria-current')).toBe('step')
    expect(view.find('[data-testid="journey-continue"]').exists()).toBe(true)
    view.unmount()
  })
  it('never declares usage complete just because a key exists', async () => {
    const view = render({ phase: 'balance', balance: 5, keyReady: true })
    expect(view.get('[data-testid="journey-step-use"]').attributes('aria-current')).toBe('step')
    expect(view.find('[data-testid="journey-verify-usage"]').exists()).toBe(true)
    await view.get('[data-testid="journey-verify-usage"] button').trigger('click')
    expect(view.emitted('refresh')).toHaveLength(1)
    expect(view.get('[data-testid="journey-step-use"]').attributes('aria-current')).toBe('step')
    await view.setProps({ requestCount: 1 })
    expect(view.get('[data-testid="journey-step-use"]').text()).toContain('firstUseJourney.done')
    view.unmount()
  })
  it('uses a retry state when funds have not been verified', () => {
    const view = render({ phase: 'unknown', balance: null })
    expect(view.find('[data-testid="journey-purchase"]').exists()).toBe(false)
    expect(view.text()).toContain('firstUseJourney.unknownBody')
    expect(view.find('[data-testid="journey-refresh"]').exists()).toBe(true)
    view.unmount()
  })
  it('omits financial steps and links in simple mode', () => {
    const view = render({ phase: 'simple', canRedeem: false, onlineAvailable: false })
    expect(view.find('[data-testid="journey-step-funding"]').exists()).toBe(false)
    expect(view.find('[data-testid="journey-redeem"]').exists()).toBe(false)
    expect(view.text()).not.toContain('firstUseJourney.fundingNotUse')
    view.unmount()
  })
  it('keeps known simple-mode finance hidden while profile verification is unavailable', () => {
    const view = render({ phase: 'unknown', simpleMode: true, loading: true, canRedeem: true })
    expect(view.find('[data-testid="journey-step-funding"]').exists()).toBe(false)
    expect(view.find('[data-testid="journey-redeem"]').exists()).toBe(false)
    expect(view.text()).not.toContain('firstUseJourney.verifying')
    view.unmount()
  })
})
