import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import RedeemView from '../RedeemView.vue'

enableAutoUnmount(afterEach)

const { redeem, getHistory, refreshUser, fetchActiveSubscriptions, showError, showWarning, showSuccess } = vi.hoisted(() => ({
  redeem: vi.fn(),
  getHistory: vi.fn(),
  refreshUser: vi.fn(),
  fetchActiveSubscriptions: vi.fn(),
  showError: vi.fn(),
  showWarning: vi.fn(),
  showSuccess: vi.fn(),
}))

const routeState = vi.hoisted(() => ({ query: {} as Record<string, unknown> }))
const routerPush = vi.hoisted(() => vi.fn())
const authState = vi.hoisted(() => ({
  isAuthenticated: true,
  token: 'session-a' as string | null,
  user: { id: 9, balance: 10, concurrency: 2 } as { id: number; balance: number; concurrency: number } | null,
  setState: (_updates: { token?: string | null; isAuthenticated?: boolean; user?: { id: number; balance: number; concurrency: number } | null }) => {},
}))
vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return { ...actual, useRoute: () => routeState, useRouter: () => ({ push: routerPush }) }
})

vi.mock('@/api', () => ({
  redeemAPI: { redeem, getHistory },
  authAPI: { getPublicSettings: vi.fn().mockResolvedValue({}) },
}))
vi.mock('@/stores/auth', async () => {
  const { reactive } = await import('vue')
  const state = reactive(authState)
  authState.setState = (updates) => { Object.assign(state, updates) }
  return { useAuthStore: () => ({
    refreshUser,
    get isAuthenticated() { return state.isAuthenticated },
    get token() { return state.token },
    get user() { return state.user },
  }) }
})
vi.mock('@/stores/subscriptions', () => ({
  useSubscriptionStore: () => ({ fetchActiveSubscriptions }),
}))
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showWarning, showSuccess }),
}))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

async function submitCode() {
  const wrapper = mount(RedeemView, {
    global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, Icon: true } },
  })
  await flushPromises()
  await wrapper.get('input#code').setValue(' REDEEM-CODE ')
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  return wrapper
}

const usableSubscription = () => ({
  id: 71, user_id: 9, group_id: 3, status: 'active',
  starts_at: '2026-01-01T00:00:00Z', expires_at: '2099-01-01T00:00:00Z',
  group: { id: 3, status: 'active' },
})

describe('RedeemView refresh after redemption', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    routeState.query = {}
    authState.setState({ isAuthenticated: true, token: 'session-a', user: { id: 9, balance: 10, concurrency: 2 } })
    redeem.mockResolvedValue({ type: 'balance', value: 20, message: 'Code applied' })
    getHistory.mockResolvedValue({ items: [], total: 0 })
    refreshUser.mockResolvedValue({ id: 9, balance: 30, concurrency: 2 })
    fetchActiveSubscriptions.mockResolvedValue([usableSubscription()])
    vi.spyOn(console, 'error').mockImplementation(() => {})
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it.each(['balance', 'concurrency', 'subscription'])(
    'keeps a successful %s redemption when profile refresh fails', async (type) => {
      redeem.mockResolvedValue({ type, value: 20, message: 'Code applied' })
      refreshUser.mockRejectedValue({ status: 503, message: 'Service unavailable' })
      getHistory.mockResolvedValueOnce({ items: [], total: 0 }).mockResolvedValueOnce({ total: 1, items: [{
        id: 1, code: 'REDEEM-CODE', type, value: 20, used_at: '2026-03-08T00:00:00Z',
      }] })

      const wrapper = await submitCode()

      expect(redeem).toHaveBeenCalledWith('REDEEM-CODE')
      expect(showError).not.toHaveBeenCalled()
      expect(showWarning).toHaveBeenCalledWith('redeem.userRefreshFailed')
      expect(showSuccess).toHaveBeenCalledWith('redeem.codeRedeemSuccess')
      expect(wrapper.text()).toContain('Code applied')
      expect(wrapper.text()).not.toContain('redeem.failedToRedeem')
      expect((wrapper.get('input#code').element as HTMLInputElement).value).toBe('')
      expect((wrapper.get('input#code').element as HTMLInputElement).disabled).toBe(false)
      expect(getHistory).toHaveBeenCalledTimes(2)
      expect(wrapper.text()).toContain('REDEEM-C...')
      if (type === 'subscription') {
        expect(fetchActiveSubscriptions).toHaveBeenCalledWith(true)
      } else {
        expect(fetchActiveSubscriptions).not.toHaveBeenCalled()
      }
      wrapper.unmount()
    }
  )

  it('pages on the server, changes size, and resets page and total after redeeming', async () => {
    getHistory.mockResolvedValue({ items: [], total: 101 })
    const wrapper = mount(RedeemView, {
      global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, Icon: true } },
    })
    await flushPromises()
    const button = (text: string) => wrapper.findAll('button').find(b => b.text() === text)!
    expect(getHistory).toHaveBeenLastCalledWith(1, 20)
    expect(button('pagination.previous').attributes('disabled')).toBeDefined()
    await button('pagination.next').trigger('click')
    await flushPromises()
    expect(getHistory).toHaveBeenLastCalledWith(2, 20)
    await button('pagination.previous').trigger('click')
    await flushPromises()
    expect(getHistory).toHaveBeenLastCalledWith(1, 20)
    expect(wrapper.findAll('select option').map(o => o.text())).toEqual(['20', '50', '100'])
    await wrapper.get('select').setValue('50')
    await flushPromises()
    expect(getHistory).toHaveBeenLastCalledWith(1, 50)
    await wrapper.get('select').setValue('100')
    await flushPromises()
    await button('pagination.next').trigger('click')
    await flushPromises()
    expect(getHistory).toHaveBeenLastCalledWith(2, 100)
    expect(button('pagination.next').attributes('disabled')).toBeDefined()
    getHistory.mockResolvedValue({ items: [], total: 102 })
    await wrapper.get('input#code').setValue('NEW-CODE')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(getHistory).toHaveBeenLastCalledWith(1, 100)
    expect(wrapper.text()).toContain('102')
    expect(button('pagination.previous').attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })

  it('restores the loaded size and keeps rows and navigation usable after a size request fails', async () => {
    const item = { id: 1, code: 'OLD-ROWS', type: 'balance', value: 20, used_at: '2026-03-08T00:00:00Z' }
    getHistory.mockResolvedValue({ items: [item], total: 61 })
    const wrapper = mount(RedeemView, {
      global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, Icon: true } },
    })
    await flushPromises()
    const button = (text: string) => wrapper.findAll('button').find(b => b.text() === text)!
    await button('pagination.next').trigger('click')
    await flushPromises()
    let rejectRequest!: (error: Error) => void
    getHistory.mockImplementationOnce(() => new Promise((_resolve, reject) => { rejectRequest = reject }))
    await wrapper.get('select').setValue('50')
    expect(getHistory).toHaveBeenLastCalledWith(1, 50)
    expect(button('pagination.next').attributes('disabled')).toBeDefined()
    rejectRequest(new Error('Network error'))
    await flushPromises()
    expect(wrapper.get('select').element.value).toBe('20')
    expect(wrapper.text()).toContain('OLD-ROWS')
    expect(wrapper.text()).toContain('61')
    expect(button('pagination.previous').attributes('disabled')).toBeUndefined()
    expect(button('pagination.next').attributes('disabled')).toBeUndefined()
    expect(showError).toHaveBeenCalledWith('redeem.historyLoadFailed')
    await button('pagination.next').trigger('click')
    await flushPromises()
    expect(getHistory).toHaveBeenLastCalledWith(3, 20)
    await wrapper.get('select').setValue('50')
    await flushPromises()
    expect(getHistory).toHaveBeenLastCalledWith(1, 50)
    expect(wrapper.get('select').element.value).toBe('50')
    expect(button('pagination.previous').attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })

  it.each(['success', 'failure'])('ignores a stale history %s after a newer size request succeeds', async (outcome) => {
    let resolveOld!: (value: unknown) => void
    let rejectOld!: (error: Error) => void
    getHistory.mockImplementationOnce(() => new Promise((resolve, reject) => {
      resolveOld = resolve
      rejectOld = reject
    }))
    const wrapper = mount(RedeemView, {
      global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, Icon: true } },
    })
    getHistory.mockResolvedValue({ items: [{
      id: 2, code: 'NEW-ROWS', type: 'balance', value: 30, used_at: '2026-03-08T00:00:00Z',
    }], total: 61 })
    // Force overlapping requests to exercise responses arriving out of order.
    await wrapper.get('select').setValue('50')
    await flushPromises()
    const button = (text: string) => wrapper.findAll('button').find(b => b.text() === text)!
    await button('pagination.next').trigger('click')
    await flushPromises()
    expect(getHistory).toHaveBeenLastCalledWith(2, 50)
    if (outcome === 'success') {
      resolveOld({ items: [], total: 0 })
    } else {
      rejectOld(new Error('Stale network error'))
    }
    await flushPromises()
    expect(wrapper.get('select').element.value).toBe('50')
    expect(wrapper.text()).toContain('NEW-ROWS')
    expect(wrapper.text()).toContain('61')
    expect(button('pagination.previous').attributes('disabled')).toBeUndefined()
    expect(button('pagination.next').attributes('disabled')).toBeDefined()
    expect(wrapper.get('select').attributes('disabled')).toBeUndefined()
    expect(showError).not.toHaveBeenCalled()
    await button('pagination.previous').trigger('click')
    await flushPromises()
    expect(getHistory).toHaveBeenLastCalledWith(1, 50)
    wrapper.unmount()
  })

  it('disables both paging buttons for empty history', async () => {
    const wrapper = await submitCode()
    for (const button of wrapper.findAll('button').filter(b => b.text().startsWith('pagination.'))) {
      expect(button.attributes('disabled')).toBeDefined()
    }
    wrapper.unmount()
  })

  it('finishes normally without a warning when profile refresh succeeds', async () => {
    const wrapper = await submitCode()

    expect(refreshUser).toHaveBeenCalledOnce()
    expect(showWarning).not.toHaveBeenCalled()
    expect(showError).not.toHaveBeenCalled()
    expect(showSuccess).toHaveBeenCalledWith('redeem.codeRedeemSuccess')
    expect((wrapper.get('input#code').element as HTMLInputElement).value).toBe('')
    wrapper.unmount()
  })

  it('preserves the existing subscription refresh warning after successful redemption', async () => {
    redeem.mockResolvedValue({ type: 'subscription', value: 20, message: 'Code applied' })
    fetchActiveSubscriptions.mockRejectedValue(new Error('Network Error'))
    const wrapper = await submitCode()

    expect(showWarning).toHaveBeenCalledWith('redeem.subscriptionRefreshFailed')
    expect(showError).not.toHaveBeenCalled()
    expect(showSuccess).toHaveBeenCalledWith('redeem.codeRedeemSuccess')
    expect(getHistory).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('keeps the code and reports failure when the redemption request itself fails', async () => {
    redeem.mockRejectedValue({ response: { data: { detail: 'Invalid code' } } })
    const wrapper = await submitCode()

    expect(showError).toHaveBeenCalledWith('redeem.redeemFailed')
    expect(wrapper.text()).toContain('Invalid code')
    expect(wrapper.text()).not.toContain('Code applied')
    expect((wrapper.get('input#code').element as HTMLInputElement).value).toBe(' REDEEM-CODE ')
    expect(refreshUser).not.toHaveBeenCalled()
    expect(fetchActiveSubscriptions).not.toHaveBeenCalled()
    expect(getHistory).toHaveBeenCalledOnce()
    expect(showSuccess).not.toHaveBeenCalled()
    expect(showWarning).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it.each(['balance', 'subscription', 'concurrency'])(
    'offers a truthful next step after %s redemption without starting another transaction', async (type) => {
      routeState.query = { scene: 'image', token: 'do-not-copy', api_key: 'do-not-copy' }
      redeem.mockResolvedValue({ type, value: 20, message: 'Code applied' })
      const wrapper = await submitCode()
      const next = wrapper.get('[data-test="redemption-next-step"]')
      const expectedKey = type === 'balance' ? 'balanceCreditedBody' : type === 'subscription' ? 'subscriptionCreditedBody' : 'concurrencyNextBody'
      expect(next.text()).toContain(`firstUseJourney.${expectedKey}`)
      if (type === 'concurrency') {
        expect(next.text()).not.toContain('firstUseJourney.balanceCreditedBody')
        expect(next.text()).not.toContain('firstUseJourney.subscriptionCreditedBody')
      }
      await next.get('button').trigger('click')
      expect(routerPush).toHaveBeenCalledWith({ path: '/get-started', query: { scene: 'image' } })
      expect(redeem).toHaveBeenCalledOnce()
      expect(redeem).toHaveBeenCalledWith('REDEEM-CODE')
      wrapper.unmount()
    },
  )

  it('retains success but does not imply a verified balance when profile refresh fails', async () => {
    refreshUser.mockRejectedValueOnce(new Error('offline'))
    const wrapper = await submitCode()
    expect(wrapper.text()).toContain('redeem.redeemSuccess')
    expect(wrapper.text()).toContain('firstUseJourney.accountRefreshPendingBody')
    expect(wrapper.text()).not.toContain('firstUseJourney.balanceCreditedBody')
    expect(wrapper.find('[data-test="redemption-next-step"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('does not offer a funding completion for an unknown redemption type', async () => {
    redeem.mockResolvedValue({ type: 'unknown', value: 20, message: 'Code applied' })
    const wrapper = await submitCode()
    expect(wrapper.find('[data-test="redemption-next-step"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('can return to the guide before redeeming, preserving only a known scene', async () => {
    routeState.query = { scene: 'code', token: 'do-not-copy' }
    const wrapper = mount(RedeemView, {
      global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, Icon: true } },
    })
    await flushPromises()
    expect(wrapper.get('[data-test="redeem-guidance"]').text()).toContain('firstUseJourney.redeemIntroBody')
    await wrapper.get('[data-test="redeem-guidance"] button').trigger('click')
    expect(routerPush).toHaveBeenCalledWith({ path: '/get-started', query: { scene: 'code' } })
    expect(redeem).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it.each(['empty', 'expired', 'future', 'other-user'])(
    'preserves redemption success but treats %s subscriptions as unverified', async (scenario) => {
      redeem.mockResolvedValue({ type: 'subscription', value: 20, message: 'Code applied' })
      const subscription = usableSubscription()
      if (scenario === 'expired') subscription.expires_at = '2000-01-01T00:00:00Z'
      if (scenario === 'future') subscription.starts_at = '2098-01-01T00:00:00Z'
      if (scenario === 'other-user') subscription.user_id = 8
      fetchActiveSubscriptions.mockResolvedValueOnce(scenario === 'empty' ? [] : [subscription])
      const wrapper = await submitCode()
      expect(wrapper.text()).toContain('redeem.redeemSuccess')
      expect(wrapper.text()).toContain('firstUseJourney.accountRefreshPendingBody')
      expect(wrapper.text()).not.toContain('firstUseJourney.subscriptionCreditedBody')
      expect(showSuccess).toHaveBeenCalledWith('redeem.codeRedeemSuccess')
      wrapper.unmount()
    },
  )

  it.each([
    ['switch-account', 'resolve'], ['logout', 'resolve'], ['unmount', 'resolve'],
    ['switch-account', 'reject'], ['logout', 'reject'], ['unmount', 'reject'],
  ])(
    'ignores a late redemption %s / %s without reading or notifying another account', async (scenario, outcome) => {
      let resolveRedemption!: (result: { type: string; value: number; message: string }) => void
      let rejectRedemption!: (error: Error) => void
      redeem.mockImplementationOnce(() => new Promise((resolve, reject) => { resolveRedemption = resolve; rejectRedemption = reject }))
      const wrapper = await submitCode()
      expect(redeem).toHaveBeenCalledOnce()
      if (scenario === 'switch-account') {
        authState.setState({ user: { id: 8, balance: 0, concurrency: 1 }, token: 'session-b' })
      } else if (scenario === 'logout') {
        authState.setState({ user: null, token: null, isAuthenticated: false })
      } else {
        wrapper.unmount()
      }
      if (outcome === 'resolve') resolveRedemption({ type: 'balance', value: 20, message: 'Old account credited' })
      else rejectRedemption(new Error('Old account request failed'))
      await flushPromises()
      expect(refreshUser).not.toHaveBeenCalled()
      expect(fetchActiveSubscriptions).not.toHaveBeenCalled()
      expect(getHistory).toHaveBeenCalledOnce()
      expect(showSuccess).not.toHaveBeenCalled()
      expect(showWarning).not.toHaveBeenCalled()
      expect(showError).not.toHaveBeenCalled()
      if (scenario !== 'unmount') {
        expect(wrapper.text()).not.toContain('Old account credited')
        expect(wrapper.find('[data-test="redemption-next-step"]').exists()).toBe(false)
        wrapper.unmount()
      }
    },
  )

  it('discards profile completion from an older session even when the account signs back in', async () => {
    let resolveProfile!: (profile: { id: number; balance: number }) => void
    refreshUser.mockImplementationOnce(() => new Promise((resolve) => { resolveProfile = resolve }))
    const wrapper = await submitCode()
    expect(wrapper.text()).toContain('Code applied')
    authState.setState({ user: { id: 8, balance: 0, concurrency: 1 }, token: 'session-b' })
    await flushPromises()
    expect(wrapper.text()).not.toContain('Code applied')
    authState.setState({ user: { id: 9, balance: 0, concurrency: 1 }, token: 'new-session-a' })
    resolveProfile({ id: 9, balance: 30 })
    await flushPromises()
    expect(wrapper.find('[data-test="redemption-next-step"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('Code applied')
    expect(getHistory).toHaveBeenCalledOnce()
    expect(showSuccess).not.toHaveBeenCalled()
    expect(showWarning).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('discards a late subscription response after logging out instead of notifying success', async () => {
    redeem.mockResolvedValue({ type: 'subscription', value: 20, message: 'Code applied' })
    let resolveSubscriptions!: (subscriptions: ReturnType<typeof usableSubscription>[]) => void
    fetchActiveSubscriptions.mockImplementationOnce(() => new Promise((resolve) => { resolveSubscriptions = resolve }))
    const wrapper = await submitCode()
    expect(fetchActiveSubscriptions).toHaveBeenCalledOnce()
    authState.setState({ user: null, token: null, isAuthenticated: false })
    resolveSubscriptions([usableSubscription()])
    await flushPromises()
    expect(wrapper.text()).not.toContain('Code applied')
    expect(wrapper.find('[data-test="redemption-next-step"]').exists()).toBe(false)
    expect(getHistory).toHaveBeenCalledOnce()
    expect(showSuccess).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('removes completed redemption and history from the page when another account signs in', async () => {
    getHistory.mockResolvedValue({ total: 1, items: [{
      id: 1, code: 'REDEEM-CODE', type: 'balance', value: 20, used_at: '2026-03-08T00:00:00Z',
    }] })
    const wrapper = await submitCode()
    expect(wrapper.text()).toContain('Code applied')
    expect(wrapper.text()).toContain('REDEEM-C...')
    authState.setState({ user: { id: 8, balance: 0, concurrency: 1 }, token: 'session-b' })
    await flushPromises()
    expect(wrapper.text()).not.toContain('Code applied')
    expect(wrapper.text()).not.toContain('REDEEM-C...')
    expect(wrapper.find('[data-test="redemption-next-step"]').exists()).toBe(false)
    expect(redeem).toHaveBeenCalledOnce()
    wrapper.unmount()
  })

  it('does not discard a successful redemption when the same account rotates its token', async () => {
    let resolveProfile!: (profile: { id: number; balance: number }) => void
    refreshUser.mockImplementationOnce(() => new Promise((resolve) => { resolveProfile = resolve }))
    const wrapper = await submitCode()
    authState.setState({ token: 'rotated-session-a' })
    resolveProfile({ id: 9, balance: 30 })
    await flushPromises()
    expect(wrapper.text()).toContain('firstUseJourney.balanceCreditedBody')
    expect(showSuccess).toHaveBeenCalledWith('redeem.codeRedeemSuccess')
    expect(getHistory).toHaveBeenCalledTimes(2)
    expect(redeem).toHaveBeenCalledOnce()
    wrapper.unmount()
  })

  it('discards history rows from an old account when the history request returns after switching users', async () => {
    let resolveHistory!: (result: { total: number; items: Record<string, unknown>[] }) => void
    getHistory.mockImplementationOnce(() => new Promise((resolve) => { resolveHistory = resolve }))
    const wrapper = mount(RedeemView, {
      global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, Icon: true } },
    })
    await flushPromises()
    authState.setState({ user: { id: 8, balance: 0, concurrency: 1 }, token: 'session-b' })
    resolveHistory({ total: 1, items: [{
      id: 1, code: 'OLD-USER-CODE', type: 'balance', value: 20, used_at: '2026-03-08T00:00:00Z',
    }] })
    await flushPromises()
    expect(wrapper.text()).not.toContain('OLD-USER')
    expect(showError).not.toHaveBeenCalled()
    expect(getHistory).toHaveBeenCalledOnce()
    expect(redeem).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
