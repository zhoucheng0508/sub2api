import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import DashboardView from '../DashboardView.vue'
import { useAuthStore } from '@/stores/auth'

const { getStats, refreshUser, auth } = vi.hoisted(() => ({
  getStats: vi.fn(), refreshUser: vi.fn(),
  auth: { user: { id: 1, balance: 0 }, token: 'test-session', isAdmin: false, isSimpleMode: false },
}))
vi.mock('@/stores/auth', async () => {
  const { reactive } = await import('vue')
  const state = reactive({ ...auth, refreshUser })
  return { useAuthStore: () => state }
})
vi.mock('@/api/usage', () => ({ usageAPI: {
  getDashboardStats: getStats,
  getDashboardTrend: async () => ({ trend: [] }),
  getDashboardModels: async () => ({ models: [] }),
  getByDateRange: async () => ({ items: [] }),
} }))
vi.mock('@/api/user', () => ({ getMyPlatformQuotas: async () => ({ platform_quotas: [] }) }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ cachedPublicSettings: {} }) }))
vi.mock('@/custom/vote-ai/quick-connect/useFundingReadiness', async () => {
  const { ref } = await import('vue')
  return { useFundingReadiness: () => ({ phase: ref('balance'), balance: ref(10), loading: ref(false), onlineAvailable: ref(true), canRedeem: ref(true), subscriptionOnly: ref(false), refresh: vi.fn() }) }
})
vi.mock('vue-i18n', () => ({
  createI18n: () => ({ global: { t: (key: string) => key } }),
  useI18n: () => ({ t: (key: string) => key }),
}))

const wrappers: VueWrapper[] = []
function render() {
  const wrapper = mount(DashboardView, { global: { stubs: {
    AppLayout: { template: '<div><slot /></div>' }, LoadingSpinner: true,
    UserDashboardStats: true, UserDashboardCharts: true, UserDashboardRecentUsage: true, UserDashboardQuickActions: true,
    RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' },
  } } })
  wrappers.push(wrapper)
  return wrapper
}
beforeEach(() => {
  vi.clearAllMocks()
  auth.isAdmin = false
  const store = useAuthStore()
  store.user = { id: 1, balance: 0 } as typeof store.user
  store.token = 'test-session'
  store.isAdmin = false
  refreshUser.mockResolvedValue({})
  getStats.mockResolvedValue({ total_api_keys: 0, active_api_keys: 0, total_requests: 0 })
})
afterEach(() => {
  wrappers.splice(0).forEach(wrapper => wrapper.unmount())
  vi.restoreAllMocks()
})

describe('dashboard newcomer guidance', () => {
  it('offers guided setup only after the server confirms an account has no keys', async () => {
    let resolveStats!: (stats: { total_api_keys: number; active_api_keys: number; total_requests: number }) => void
    getStats.mockImplementationOnce(() => new Promise(resolve => { resolveStats = resolve }))
    const wrapper = render()
    await flushPromises()
    expect(wrapper.find('[data-testid="newcomer-guide"]').exists()).toBe(false)

    resolveStats({ total_api_keys: 0, active_api_keys: 0, total_requests: 0 })
    await flushPromises()
    expect(wrapper.get('[data-testid="newcomer-guide"] a').attributes('href')).toBe('/get-started')
    expect(wrapper.findComponent({ name: 'UserDashboardStats' }).exists()).toBe(true)
  })

  it('keeps an existing account dashboard unchanged', async () => {
    getStats.mockResolvedValue({ total_api_keys: 1, active_api_keys: 1, total_requests: 5 })
    const wrapper = render()
    await flushPromises()
    expect(wrapper.find('[data-testid="newcomer-guide"]').exists()).toBe(false)
    expect(wrapper.findComponent({ name: 'UserDashboardStats' }).exists()).toBe(true)
  })

  it('does not infer a new account when loading statistics fails', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    getStats.mockRejectedValue(new Error('offline'))
    const wrapper = render()
    await flushPromises()
    expect(wrapper.find('[data-testid="newcomer-guide"]').exists()).toBe(false)
  })
  it('keeps guidance after creating a key until the account has request history', async () => {
    getStats.mockResolvedValue({ total_api_keys: 1, active_api_keys: 1, total_requests: 0 })
    const wrapper = render()
    await flushPromises()
    expect(wrapper.find('[data-testid="newcomer-guide"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="journey-step-key"]').text()).toContain('firstUseJourney.done')
    expect(wrapper.get('[data-testid="journey-step-use"]').attributes('aria-current')).toBe('step')
  })
  it('does not classify an incomplete statistics response as a new account', async () => {
    getStats.mockResolvedValue({ total_api_keys: 0 })
    const wrapper = render()
    await flushPromises()
    expect(wrapper.find('[data-testid="newcomer-guide"]').exists()).toBe(false)
  })

  it('does not offer user onboarding to an administrator', async () => {
    useAuthStore().isAdmin = true
    const wrapper = render()
    await flushPromises()
    expect(wrapper.find('[data-testid="newcomer-guide"]').exists()).toBe(false)
  })

  it('hides stale guidance if refreshing statistics fails', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const wrapper = render()
    await flushPromises()
    expect(wrapper.find('[data-testid="newcomer-guide"]').exists()).toBe(true)
    getStats.mockRejectedValue(new Error('offline'))
    wrapper.findComponent({ name: 'UserDashboardCharts' }).vm.$emit('refresh')
    await flushPromises()
    expect(wrapper.find('[data-testid="newcomer-guide"]').exists()).toBe(false)
  })
  it('discards an earlier account statistics response after changing accounts', async () => {
    let finishOld!: (value: unknown) => void
    getStats.mockImplementationOnce(() => new Promise(resolve => { finishOld = resolve }))
    const wrapper = render(); await flushPromises()
    getStats.mockResolvedValue({ total_api_keys: 0, active_api_keys: 0, total_requests: 0 })
    const store = useAuthStore()
    store.user = { id: 2, balance: 0 } as typeof store.user
    await flushPromises()
    expect(wrapper.find('[data-testid="newcomer-guide"]').exists()).toBe(true)
    finishOld({ total_api_keys: 1, active_api_keys: 1, total_requests: 25 }); await flushPromises()
    expect(wrapper.find('[data-testid="newcomer-guide"]').exists()).toBe(true)
  })
  it('does not restart setup after a same-user profile or token refresh', async () => {
    const wrapper = render(); await flushPromises()
    const store = useAuthStore()
    store.user = { id: 1, balance: 15 } as typeof store.user
    store.token = 'rotated-session'
    await flushPromises()
    expect(getStats).toHaveBeenCalledTimes(1)
    expect(wrapper.find('[data-testid="newcomer-guide"]').exists()).toBe(true)
  })
})
