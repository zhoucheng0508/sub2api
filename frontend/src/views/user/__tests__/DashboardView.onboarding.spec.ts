import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import DashboardView from '../DashboardView.vue'

const { getStats, refreshUser, auth } = vi.hoisted(() => ({
  getStats: vi.fn(), refreshUser: vi.fn(),
  auth: { user: { balance: 0 }, isAdmin: false, isSimpleMode: false },
}))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ ...auth, refreshUser }) }))
vi.mock('@/api/usage', () => ({ usageAPI: {
  getDashboardStats: getStats,
  getDashboardTrend: async () => ({ trend: [] }),
  getDashboardModels: async () => ({ models: [] }),
  getByDateRange: async () => ({ items: [] }),
} }))
vi.mock('@/api/user', () => ({ getMyPlatformQuotas: async () => ({ platform_quotas: [] }) }))
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
  refreshUser.mockResolvedValue({})
  getStats.mockResolvedValue({ total_api_keys: 0 })
})
afterEach(() => {
  wrappers.splice(0).forEach(wrapper => wrapper.unmount())
  vi.restoreAllMocks()
})

describe('dashboard newcomer guidance', () => {
  it('offers guided setup only after the server confirms an account has no keys', async () => {
    let resolveStats!: (stats: { total_api_keys: number }) => void
    getStats.mockImplementationOnce(() => new Promise(resolve => { resolveStats = resolve }))
    const wrapper = render()
    await flushPromises()
    expect(wrapper.find('[data-testid="newcomer-guide"]').exists()).toBe(false)

    resolveStats({ total_api_keys: 0 })
    await flushPromises()
    expect(wrapper.get('[data-testid="newcomer-guide"] a').attributes('href')).toBe('/get-started')
    expect(wrapper.findComponent({ name: 'UserDashboardStats' }).exists()).toBe(true)
  })

  it('keeps an existing account dashboard unchanged', async () => {
    getStats.mockResolvedValue({ total_api_keys: 1 })
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

  it('does not offer user onboarding to an administrator', async () => {
    auth.isAdmin = true
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
})
