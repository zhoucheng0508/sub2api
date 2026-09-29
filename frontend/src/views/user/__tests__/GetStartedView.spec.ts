import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, flushPromises, type VueWrapper } from '@vue/test-utils'
import type { ApiKey } from '@/types'

const { list, settings, route, replace, auth, app } = vi.hoisted(() => ({
  list: vi.fn(), settings: vi.fn(), replace: vi.fn(), route: { query: {} as Record<string, string> },
  auth: { token: 'test-session', user: { id: 1 } },
  app: { siteName: 'Vote', cachedPublicSettings: { api_base_url: 'https://api.example', custom_menu_items: [] }, fetchPublicSettings: vi.fn() },
}))
vi.mock('@/api/keys', () => ({ keysAPI: { list } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => app }))
vi.mock('@/stores/auth', async () => {
  const { reactive } = await import('vue')
  const state = reactive(auth)
  return { useAuthStore: () => state }
})
vi.mock('vue-router', () => ({ useRoute: () => route, useRouter: () => ({ replace, push: vi.fn() }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<div><slot /></div>' } }))
vi.mock('@/components/keys/QuickConnectPanel.vue', () => ({ default: { name: 'QuickConnectPanel', props: ['availableKeys', 'initialKeyId', 'apiKey'], template: '<div data-testid="panel"><slot v-if="!availableKeys.length" name="create-key" app="codex" /></div>' } }))
vi.mock('@/custom/vote-ai/quick-connect/CreateConnectKey.vue', () => ({ default: { name: 'CreateConnectKey', props: ['scene', 'app'], emits: ['created', 'busy'], template: '<div data-testid="create-connect-key" />' } }))
import GetStartedView from '../GetStartedView.vue'
import { useAuthStore } from '@/stores/auth'

const wrappers: VueWrapper[] = []
const fixture = (id: number, imageOnly = false) => ({ id, user_id: 1, name: `Example ${id}`, key: `private-key-${id}-7890`, status: 'active', group_id: id, quota: 0, quota_used: 0,
  group: { id, name: `Group ${id}`, platform: 'openai', status: 'active', allow_image_generation: imageOnly, image_only: imageOnly } } as ApiKey)
function render() {
  const wrapper = mount(GetStartedView, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, Icon: true, LoadingSpinner: true, RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' } } } })
  wrappers.push(wrapper)
  return wrapper
}
beforeEach(() => { vi.clearAllMocks(); route.query = {}; auth.token = 'test-session'; settings.mockResolvedValue(null); app.fetchPublicSettings.mockResolvedValue(null); list.mockResolvedValue({ items: [fixture(1)], pages: 1 }) })
afterEach(() => { wrappers.splice(0).forEach(w => w.unmount()) })

describe('A guided get started page', () => {
  it('loads all pages, preserving a key chosen from another list page', async () => {
    route.query = { key: '2' }
    list.mockResolvedValueOnce({ items: [fixture(1)], pages: 2 }).mockResolvedValueOnce({ items: [fixture(2)], pages: 2 })
    const wrapper = render(); await flushPromises()
    const panel = wrapper.findComponent({ name: 'QuickConnectPanel' })
    expect(panel.props('availableKeys')).toHaveLength(2)
    expect(panel.props('initialKeyId')).toBe(2)
    expect(list).toHaveBeenNthCalledWith(2, 2, 100, undefined, { signal: expect.any(AbortSignal) })
    expect(wrapper.html()).not.toContain('private-key')
  })
  it('lets a user without keys create one without leaving get started', async () => {
    list.mockResolvedValue({ items: [], pages: 0 })
    const wrapper = render(); await flushPromises()
    expect(wrapper.find('[data-testid="panel"]').exists()).toBe(true)
    const create = wrapper.findComponent({ name: 'CreateConnectKey' })
    expect(create.props()).toMatchObject({ scene: 'text', app: 'codex' })
    create.vm.$emit('created', fixture(5))
    await flushPromises()
    const panel = wrapper.findComponent({ name: 'QuickConnectPanel' })
    expect(panel.props('initialKeyId')).toBe(5)
    expect(panel.props('availableKeys')).toHaveLength(1)
    expect(wrapper.find('[data-testid="created-in-place"]').exists()).toBe(true)
    expect(replace).toHaveBeenLastCalledWith({ query: { key: '5', scene: 'code' } })
    expect(JSON.stringify(replace.mock.calls)).not.toContain('private-key')
    expect(wrapper.html()).not.toContain('private-key')
  })
  it('shows retry rather than using a partial list', async () => {
    list.mockResolvedValueOnce({ items: [fixture(1)], pages: 2 }).mockRejectedValueOnce(new Error('offline'))
    const wrapper = render(); await flushPromises()
    expect(wrapper.find('[data-testid="retry-keys"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="panel"]').exists()).toBe(false)
    list.mockResolvedValue({ items: [fixture(2)], pages: 1 })
    await wrapper.get('[data-testid="retry-keys"]').trigger('click'); await flushPromises()
    expect(wrapper.find('[data-testid="panel"]').exists()).toBe(true)
  })
  it('shows only usable image keys without exposing their secrets', async () => {
    route.query = { scene: 'image', key: '2' }
    list.mockResolvedValue({ items: [fixture(1), fixture(2, true), { ...fixture(3, true), status: 'inactive' }], pages: 1 })
    const wrapper = render(); await flushPromises()
    expect(wrapper.findAll('option')).toHaveLength(1)
    expect(wrapper.get('select').element.value).toBe('2')
    expect(wrapper.html()).not.toContain('private-key')
    expect(wrapper.text()).not.toContain('RikkaHub')
    expect(wrapper.find('[data-testid="enter-workbench"]').exists()).toBe(false)
  })
  it('does not leave a late request mounted after navigation away', async () => {
    let resolve!: (value: unknown) => void
    list.mockImplementation(() => new Promise(r => { resolve = r }))
    const wrapper = render(); await flushPromises()
    const signal = list.mock.calls[0][3].signal
    wrapper.unmount()
    expect(signal.aborted).toBe(true)
    resolve({ items: [fixture(1)], pages: 1 }); await flushPromises()
  })
  it('requires an explicit replacement for an unavailable requested image key', async () => {
    route.query = { scene: 'image', key: '3' }
    list.mockResolvedValue({ items: [fixture(2, true), { ...fixture(3, true), status: 'inactive' }], pages: 1 })
    const wrapper = render(); await flushPromises()
    expect(wrapper.find('[data-testid="image-requested-unavailable"]').exists()).toBe(true)
    expect(wrapper.get('select').element.value).toBe('3')
    expect(wrapper.find('[data-testid="enter-workbench"]').exists()).toBe(false)
    await wrapper.get('select').setValue('2')
    expect(wrapper.find('[data-testid="image-requested-unavailable"]').exists()).toBe(false)
  })
  it('creates and selects an image key in the same flow', async () => {
    route.query = { scene: 'image' }
    list.mockResolvedValue({ items: [], pages: 0 })
    const wrapper = render(); await flushPromises()
    const create = wrapper.findComponent({ name: 'CreateConnectKey' })
    expect(create.props('scene')).toBe('image')
    create.vm.$emit('created', fixture(8, true)); await flushPromises()
    expect(wrapper.get('select').element.value).toBe('8')
    expect(wrapper.find('[data-testid="inline-image-create"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="created-in-place"]').exists()).toBe(true)
    expect(wrapper.html()).not.toContain('private-key')
    expect(replace).toHaveBeenLastCalledWith({ query: { scene: 'image', key: '8' } })
  })
  it('keeps a pending creator mounted across token refresh and prevents changing scenes', async () => {
    list.mockResolvedValue({ items: [], pages: 0 })
    const wrapper = render(); await flushPromises()
    const create = wrapper.findComponent({ name: 'CreateConnectKey' })
    create.vm.$emit('busy', true); await flushPromises()
    expect(wrapper.get('[data-testid="scene-image"]').attributes('disabled')).toBeDefined()
    useAuthStore().token = 'refreshed-session'
    await flushPromises()
    expect(list).toHaveBeenCalledTimes(1)
    expect(wrapper.findComponent({ name: 'CreateConnectKey' }).vm).toBe(create.vm)
    create.vm.$emit('busy', false); await flushPromises()
    expect(wrapper.get('[data-testid="scene-image"]').attributes('disabled')).toBeUndefined()
  })
})
