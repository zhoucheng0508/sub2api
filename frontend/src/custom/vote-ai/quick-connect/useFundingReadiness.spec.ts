import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, nextTick, reactive } from 'vue'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import type { PublicSettings } from '@/types'
import { useFundingReadiness } from './useFundingReadiness'

const mocks = vi.hoisted(() => ({
  getCurrentUser: vi.fn(), getActiveSubscriptions: vi.fn(), getCheckoutInfo: vi.fn(),
  fetchPublicSettings: vi.fn(), auth: null as any, app: null as any
}))
vi.mock('@/api/auth', () => ({ authAPI: { getCurrentUser: mocks.getCurrentUser } }))
vi.mock('@/api/subscriptions', () => ({ default: { getActiveSubscriptions: mocks.getActiveSubscriptions } }))
vi.mock('@/api/payment', () => ({ paymentAPI: { getCheckoutInfo: mocks.getCheckoutInfo } }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => mocks.auth }))
vi.mock('@/stores/app', () => ({ useAppStore: () => mocks.app }))

const settings = {
  payment_enabled: true, subscription_enabled: true, payment_balance_disabled: false
} as PublicSettings
let wrappers: VueWrapper[] = []

function mountReadiness() {
  let readiness!: ReturnType<typeof useFundingReadiness>
  const wrapper = mount(defineComponent({
    setup() { readiness = useFundingReadiness(); return () => null }
  }))
  wrappers.push(wrapper)
  return { readiness, wrapper }
}

function deferred<T>() {
  let resolve!: (result: T) => void
  const promise = new Promise<T>((done) => { resolve = done })
  return { resolve, promise }
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.auth = reactive({ user: { id: 1 }, token: 'current-token', runMode: 'standard' })
  mocks.app = reactive({
    publicSettingsLoaded: true,
    cachedPublicSettings: { ...settings },
    fetchPublicSettings: mocks.fetchPublicSettings
  })
  mocks.getCurrentUser.mockResolvedValue({ data: { id: 1, balance: 0, run_mode: 'standard' } })
  mocks.fetchPublicSettings.mockResolvedValue(settings)
  mocks.getActiveSubscriptions.mockResolvedValue([])
  mocks.getCheckoutInfo.mockResolvedValue({ data: {
    balance_disabled: false, plans: [], methods: { alipay: { available: true } }
  } })
})
afterEach(() => { wrappers.forEach((wrapper) => wrapper.unmount()); wrappers = [] })

describe('useFundingReadiness read-only lifecycle', () => {
  it('recognizes gifted available balance without subtracting frozen funds twice', async () => {
    mocks.getCurrentUser.mockResolvedValue({ data: {
      id: 1, balance: 2, frozen_balance: 8, run_mode: 'standard'
    } })
    const { readiness } = mountReadiness()
    await flushPromises()
    expect(readiness.balance.value).toBe(2)
    expect(readiness.phase.value).toBe('balance')
    expect(readiness.onlineAvailable.value).toBe(true)
    expect(readiness.canRedeem.value).toBe(true)
  })

  it('confirms empty funds only after an empty subscription result', async () => {
    const subscriptions = deferred<[]>()
    mocks.getActiveSubscriptions.mockReturnValue(subscriptions.promise)
    const { readiness } = mountReadiness()
    await flushPromises()
    expect(readiness.phase.value).toBe('unknown')
    subscriptions.resolve([])
    await flushPromises()
    expect(readiness.phase.value).toBe('empty')
    expect(readiness.loading.value).toBe(false)
  })

  it('recognizes active subscription entitlement with zero balance', async () => {
    mocks.getActiveSubscriptions.mockResolvedValue([{
      status: 'active', group_id: 10, starts_at: '2020-01-01T00:00:00Z',
      expires_at: '2100-01-01T00:00:00Z', group: { id: 10, status: 'active' }
    }])
    const { readiness } = mountReadiness()
    await flushPromises()
    expect(readiness.phase.value).toBe('subscription')
    expect(readiness.hasActiveSubscription.value).toBe(true)
  })

  it('leaves entitlement unknown after subscription failure instead of suggesting repeat payment', async () => {
    mocks.getActiveSubscriptions.mockRejectedValue(new Error('offline'))
    const { readiness } = mountReadiness()
    await flushPromises()
    expect(readiness.phase.value).toBe('unknown')
    expect(readiness.hasActiveSubscription.value).toBeNull()
    expect(readiness.loading.value).toBe(false)
  })

  it('keeps confirmed balance useful when subscription lookup fails', async () => {
    mocks.getCurrentUser.mockResolvedValue({ data: { id: 1, balance: 5, run_mode: 'standard' } })
    mocks.getActiveSubscriptions.mockRejectedValue(new Error('offline'))
    const { readiness } = mountReadiness()
    await flushPromises()
    expect(readiness.phase.value).toBe('balance')
  })

  it('does not mistake an unconfirmed public-settings fallback for closed payment', async () => {
    mocks.app.publicSettingsLoaded = false
    mocks.app.cachedPublicSettings = null
    mocks.fetchPublicSettings.mockResolvedValue(null)
    const { readiness } = mountReadiness()
    await flushPromises()
    expect(readiness.settingsReady.value).toBe(false)
    expect(readiness.onlineAvailable.value).toBeNull()
    expect(readiness.phase.value).toBe('unknown')
    expect(mocks.getActiveSubscriptions).not.toHaveBeenCalled()
    expect(mocks.getCheckoutInfo).not.toHaveBeenCalled()
  })

  it('skips explicitly closed payment and subscription APIs', async () => {
    mocks.app.cachedPublicSettings.payment_enabled = false
    mocks.app.cachedPublicSettings.subscription_enabled = false
    const { readiness } = mountReadiness()
    await flushPromises()
    expect(readiness.phase.value).toBe('empty')
    expect(readiness.onlineAvailable.value).toBe(false)
    expect(mocks.getActiveSubscriptions).not.toHaveBeenCalled()
    expect(mocks.getCheckoutInfo).not.toHaveBeenCalled()
  })

  it('does not advertise online payment when every checkout method is unavailable', async () => {
    mocks.getCheckoutInfo.mockResolvedValue({ data: {
      balance_disabled: true, plans: [{ for_sale: true }], methods: { alipay: { available: false } }
    } })
    const { readiness } = mountReadiness()
    await flushPromises()
    expect(readiness.onlineAvailable.value).toBe(false)
    expect(readiness.subscriptionOnly.value).toBe(false)
  })

  it.each([
    { subscriptionEnabled: false, plans: [{ for_sale: true }] },
    { subscriptionEnabled: true, plans: [] },
    { subscriptionEnabled: true, plans: [{ for_sale: false }] }
  ])('does not offer checkout when a method exists but no funding path is purchasable %#', async (facts) => {
    mocks.app.cachedPublicSettings.subscription_enabled = facts.subscriptionEnabled
    mocks.getCheckoutInfo.mockResolvedValue({ data: {
      balance_disabled: true, plans: facts.plans, methods: { alipay: { available: true } }
    } })
    const { readiness } = mountReadiness()
    await flushPromises()
    expect(readiness.onlineAvailable.value).toBe(false)
    expect(readiness.subscriptionOnly.value).toBe(false)
  })

  it('offers the subscription tab only with an enabled, purchasable subscription and payment method', async () => {
    mocks.getCheckoutInfo.mockResolvedValue({ data: {
      balance_disabled: true, plans: [{ for_sale: true }], methods: { alipay: { available: true } }
    } })
    const { readiness } = mountReadiness()
    await flushPromises()
    expect(readiness.onlineAvailable.value).toBe(true)
    expect(readiness.subscriptionOnly.value).toBe(true)
  })

  it('still offers balance recharge when subscriptions are closed or have no plans', async () => {
    mocks.app.cachedPublicSettings.subscription_enabled = false
    const { readiness } = mountReadiness()
    await flushPromises()
    expect(readiness.onlineAvailable.value).toBe(true)
    expect(readiness.subscriptionOnly.value).toBe(false)
  })

  it('keeps online availability unknown when checkout fails and retries successfully', async () => {
    mocks.getCheckoutInfo.mockRejectedValueOnce(new Error('offline'))
    const { readiness } = mountReadiness()
    await flushPromises()
    expect(readiness.onlineAvailable.value).toBeNull()
    await readiness.refresh()
    expect(readiness.onlineAvailable.value).toBe(true)
    expect(mocks.getCheckoutInfo).toHaveBeenCalledTimes(2)
  })

  it('does not use standard funds APIs or funding links in simple mode', async () => {
    mocks.getCurrentUser.mockResolvedValue({ data: { id: 1, balance: 0, run_mode: 'simple' } })
    const { readiness } = mountReadiness()
    await flushPromises()
    expect(readiness.phase.value).toBe('simple')
    expect(readiness.canRedeem.value).toBe(false)
    expect(readiness.onlineAvailable.value).toBe(false)
    expect(mocks.getCheckoutInfo).not.toHaveBeenCalled()
    expect(mocks.getActiveSubscriptions).not.toHaveBeenCalled()
  })

  it('ignores old profile results after an account switch', async () => {
    const oldProfile = deferred<any>()
    mocks.getCurrentUser.mockReturnValueOnce(oldProfile.promise)
      .mockResolvedValueOnce({ data: { id: 2, balance: 4, run_mode: 'standard' } })
    const { readiness } = mountReadiness()
    await flushPromises()
    mocks.auth.user = { id: 2 }
    await flushPromises()
    expect(readiness.balance.value).toBe(4)
    oldProfile.resolve({ data: { id: 1, balance: 999, run_mode: 'simple' } })
    await flushPromises()
    expect(readiness.balance.value).toBe(4)
    expect(readiness.phase.value).toBe('balance')
    expect(mocks.auth.user.id).toBe(2)
  })

  it('ignores old subscription results after an account switch', async () => {
    const oldSubscriptions = deferred<any[]>()
    mocks.getActiveSubscriptions.mockReturnValueOnce(oldSubscriptions.promise).mockResolvedValueOnce([])
    mocks.getCurrentUser.mockResolvedValueOnce({ data: { id: 1, balance: 0, run_mode: 'standard' } })
      .mockResolvedValueOnce({ data: { id: 2, balance: 0, run_mode: 'standard' } })
    const { readiness } = mountReadiness()
    await flushPromises()
    mocks.auth.user = { id: 2 }
    await flushPromises()
    expect(readiness.phase.value).toBe('empty')
    oldSubscriptions.resolve([{
      status: 'active', group_id: 10, starts_at: '2020-01-01T00:00:00Z',
      expires_at: '2100-01-01T00:00:00Z', group: { id: 10, status: 'active' }
    }])
    await flushPromises()
    expect(readiness.phase.value).toBe('empty')
    expect(readiness.hasActiveSubscription.value).toBe(false)
  })

  it('does not clear readiness or refetch for normal same-account token rotation', async () => {
    mocks.getCurrentUser.mockResolvedValue({ data: { id: 1, balance: 3, run_mode: 'standard' } })
    const { readiness } = mountReadiness()
    await flushPromises()
    mocks.auth.token = 'rotated-token'
    await nextTick()
    expect(readiness.phase.value).toBe('balance')
    expect(mocks.getCurrentUser).toHaveBeenCalledTimes(1)
  })

  it('invalidates funding facts on logout', async () => {
    mocks.getCurrentUser.mockResolvedValue({ data: { id: 1, balance: 3, run_mode: 'standard' } })
    const { readiness } = mountReadiness()
    await flushPromises()
    mocks.auth.token = null
    expect(readiness.phase.value).toBe('unknown')
    expect(readiness.balance.value).toBeNull()
    expect(readiness.canRedeem.value).toBe(false)
  })

  it('ignores profile completion after unmount', async () => {
    const profile = deferred<any>()
    mocks.getCurrentUser.mockReturnValue(profile.promise)
    const { readiness, wrapper } = mountReadiness()
    wrapper.unmount()
    profile.resolve({ data: { id: 1, balance: 9, run_mode: 'standard' } })
    await flushPromises()
    expect(readiness.balance.value).toBeNull()
  })

  it('retries profile failures and never mutates global auth user data', async () => {
    mocks.getCurrentUser.mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce({ data: { id: 1, balance: 7, run_mode: 'standard' } })
    const { readiness } = mountReadiness()
    await flushPromises()
    expect(readiness.phase.value).toBe('unknown')
    await readiness.refresh()
    expect(readiness.phase.value).toBe('balance')
    expect(mocks.auth.user).toEqual({ id: 1 })
  })
})
