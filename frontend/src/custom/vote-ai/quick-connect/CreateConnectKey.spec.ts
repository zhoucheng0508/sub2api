import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick, reactive } from 'vue'
import type { ApiKey, Group } from '@/types'
import CreateConnectKey from './CreateConnectKey.vue'
import { en, zh } from './create-key-messages'

const { getAvailable, getUserGroupRates, create } = vi.hoisted(() => ({
  getAvailable: vi.fn(), getUserGroupRates: vi.fn(), create: vi.fn(),
}))
const auth = reactive<{ user: { id: number } | null }>({ user: { id: 1 } })
const wrappers: Array<{ unmount: () => void }> = []

vi.mock('@/api', () => ({ keysAPI: { create }, userGroupsAPI: { getAvailable, getUserGroupRates } }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('vue-i18n', async importOriginal => ({ ...await importOriginal<typeof import('vue-i18n')>(), useI18n: () => ({
  t: (key: string, params: Record<string, unknown> = {}) => {
    const template = en[key.replace('quickCreateKey.', '') as keyof typeof en] ?? key
    return template.replace(/\{(\w+)\}/g, (_, name: string) => String(params[name] ?? ''))
  },
}) }))

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: unknown) => void
  const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail })
  return { promise, resolve, reject }
}

function group(overrides: Partial<Group> = {}): Group {
  return {
    id: 10, name: 'Available group', description: 'Model access from this group', platform: 'openai',
    status: 'active', subscription_type: 'standard', rate_multiplier: 1.25, image_only: false,
    allow_image_generation: false, ...overrides,
  } as Group
}

function key(overrides: Partial<ApiKey> = {}): ApiKey {
  return { id: 50, name: 'Created key', group_id: 10, key: 'sk-do-not-render-this-secret', status: 'active', ...overrides } as ApiKey
}

function mountCreator(props: { scene?: 'text' | 'image'; app?: 'codex' | 'claude' | 'gemini'; showCancel?: boolean } = {}) {
  const wrapper = mount(CreateConnectKey, {
    props: { scene: 'text', app: 'codex', ...props },
    global: { stubs: {
      RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' },
      GroupOptionItem: {
        props: ['name', 'description', 'rateMultiplier', 'userRateMultiplier', 'subscriptionType'],
        template: '<span><strong>{{ name }}</strong><span>{{ description }}</span><span v-if="rateMultiplier !== undefined" data-testid="group-rate">{{ userRateMultiplier ?? rateMultiplier }}×</span></span>',
      },
    } },
  })
  wrappers.push(wrapper)
  return wrapper
}

beforeEach(() => {
  auth.user = { id: 1 }
  getAvailable.mockReset().mockResolvedValue([group()])
  getUserGroupRates.mockReset().mockResolvedValue({ 10: 0.8 })
  create.mockReset().mockResolvedValue(key())
})

afterEach(() => {
  wrappers.splice(0).forEach(wrapper => wrapper.unmount())
  vi.restoreAllMocks()
})

describe('CreateConnectKey', () => {
  it('shows an empty state without creating keys or suggesting an unapproved purchase', async () => {
    getAvailable.mockResolvedValue([])
    const wrapper = mountCreator()
    await flushPromises()
    expect(wrapper.get('[data-testid="create-key-no-groups"]').text()).toContain(en.noTextGroups)
    expect(wrapper.get('a').attributes('href')).toBe('/dashboard')
    expect(wrapper.find('form').exists()).toBe(false)
    expect(wrapper.find('[data-testid="create-key-cancel"]').exists()).toBe(false)
    expect(create).not.toHaveBeenCalled()
  })

  it('does not read protected groups without an account', async () => {
    auth.user = null
    const wrapper = mountCreator()
    await flushPromises()
    expect(getAvailable).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="create-key-no-groups"]').exists()).toBe(true)
    expect(create).not.toHaveBeenCalled()
  })

  it('uses capability flags and active status to separate text and image groups', async () => {
    getAvailable.mockResolvedValue([
      group({ id: 1, name: 'Text despite image name' }),
      group({ id: 2, name: 'Text-looking image group', image_only: true, allow_image_generation: true }),
      group({ id: 3, name: 'Both uses', allow_image_generation: true }),
      group({ id: 4, status: 'inactive', allow_image_generation: true }),
      group({ id: 5, platform: 'anthropic', image_only: true, allow_image_generation: true }),
    ])
    const wrapper = mountCreator()
    await flushPromises()
    expect(wrapper.find('[data-testid="create-key-group-1"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="create-key-group-2"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="create-key-group-3"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="create-key-group-4"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="create-key-submit"]').attributes('disabled')).toBeDefined()
    await wrapper.setProps({ scene: 'image' })
    expect(wrapper.find('[data-testid="create-key-group-1"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="create-key-group-2"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="create-key-group-3"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="create-key-group-5"]').exists()).toBe(false)
    expect(create).not.toHaveBeenCalled()
  })

  it('labels automatic, manual and CN OAI setup without choosing among billing groups', async () => {
    getAvailable.mockResolvedValue([
      group(), group({ id: 11, platform: 'anthropic', name: 'Claude group' }),
      group({ id: 12, name: '国模 OAI' }),
    ])
    const wrapper = mountCreator()
    await flushPromises()
    expect(wrapper.get('[data-testid="create-key-group-10"]').text()).toContain('Automatic configuration for Codex')
    expect(wrapper.get('[data-testid="create-key-group-11"]').text()).toContain('manual setup guide for Codex')
    expect(wrapper.get('[data-testid="create-key-group-12"]').text()).toContain(en.cnOai)
    expect(wrapper.findAll('input[type="radio"]').every(input => !(input.element as HTMLInputElement).checked)).toBe(true)
    await wrapper.get('[data-testid="create-key-group-11"] input').setValue(true)
    await wrapper.setProps({ app: 'claude' })
    expect((wrapper.get('[data-testid="create-key-group-11"] input').element as HTMLInputElement).checked).toBe(true)
    expect(wrapper.get('[data-testid="create-key-group-11"]').text()).toContain('Automatic configuration for Claude Code')
    expect(create).not.toHaveBeenCalled()
  })

  it('creates only once on explicit submission, preserves the group and never renders the secret', async () => {
    const response = deferred<ApiKey>()
    create.mockReturnValue(response.promise)
    const wrapper = mountCreator()
    await flushPromises()
    expect((wrapper.get('input[type="radio"]').element as HTMLInputElement).checked).toBe(true)
    expect(wrapper.get('[data-testid="group-rate"]').text()).toBe('0.8×')
    expect(wrapper.text()).toContain(en.noPayment)
    expect(create).not.toHaveBeenCalled()
    await wrapper.get('form').trigger('submit')
    await wrapper.get('form').trigger('submit')
    expect(create).toHaveBeenCalledTimes(1)
    expect(create).toHaveBeenCalledWith('Codex connection', 10)
    expect(wrapper.get('[data-testid="create-key-submit"]').attributes('disabled')).toBeDefined()
    response.resolve(key())
    await flushPromises()
    expect(wrapper.emitted('created')).toEqual([[{ ...key(), group: group() }]])
    expect(wrapper.html()).not.toContain(key().key)
    await wrapper.get('form').trigger('submit')
    expect(create).toHaveBeenCalledTimes(1)
  })

  it('uses an optional trimmed name and the active subscription returned by the availability API', async () => {
    getAvailable.mockResolvedValue([group({ subscription_type: 'subscription' })])
    const wrapper = mountCreator()
    await flushPromises()
    expect(wrapper.text()).toContain(en.subscription)
    await wrapper.get('[data-testid="create-key-name"]').setValue('  My work laptop  ')
    await wrapper.get('form').trigger('submit')
    expect(create).toHaveBeenCalledTimes(1)
    expect(create).toHaveBeenCalledWith('My work laptop', 10)
  })

  it('shows only configured image prices, without inventing a rate when an image price is absent', async () => {
    getAvailable.mockResolvedValue([group({ image_only: true, allow_image_generation: true, image_price_1k: 0.12, image_price_2k: null })])
    const wrapper = mountCreator({ scene: 'image' })
    await flushPromises()
    expect(wrapper.text()).toContain('1K image reference price: $0.12/image')
    expect(wrapper.text()).not.toContain('2K image reference')
    expect(wrapper.find('[data-testid="group-rate"]').exists()).toBe(false)
    await wrapper.get('form').trigger('submit')
    expect(create).toHaveBeenCalledTimes(1)
    expect(create).toHaveBeenCalledWith('Image creation', 10)
  })

  it.each(['groups', 'rates'])('keeps an incomplete %s read from enabling creation and supports an explicit read retry', async (failed) => {
    if (failed === 'groups') getAvailable.mockRejectedValueOnce(new Error('private request details'))
    else getUserGroupRates.mockRejectedValueOnce(new Error('private request details'))
    const wrapper = mountCreator()
    await flushPromises()
    expect(wrapper.find('form').exists()).toBe(false)
    expect(wrapper.text()).toContain(en.loadError)
    expect(wrapper.text()).not.toContain('private request details')
    await wrapper.get('[data-testid="create-key-reload"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('form').exists()).toBe(true)
    expect(create).not.toHaveBeenCalled()
  })

  it('keeps create errors local, hides raw details and does not automatically retry', async () => {
    create.mockRejectedValue({ status: 500, message: 'secret sk-private-value internal trace' })
    const wrapper = mountCreator()
    await flushPromises()
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.get('[data-testid="create-key-error"]').text()).toContain(en.createError)
    await wrapper.get('[data-testid="create-key-check-existing"]').trigger('click')
    expect(wrapper.emitted('refresh-keys')).toHaveLength(1)
    expect(wrapper.html()).not.toContain('sk-private-value')
    expect(wrapper.emitted('created')).toBeUndefined()
    expect(create).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[data-testid="create-key-submit"]').attributes('disabled')).toBeUndefined()
  })

  it('explains changed group permission and offers a read retry', async () => {
    create.mockRejectedValue({ reason: 'GROUP_NOT_ALLOWED' })
    const wrapper = mountCreator()
    await flushPromises()
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.get('[data-testid="create-key-error"]').text()).toContain(en.groupDenied)
    expect(wrapper.get('[data-testid="create-key-error"] button').text()).toBe(en.retry)
    expect(create).toHaveBeenCalledTimes(1)
  })

  it('aborts old group reads on account change and ignores their late result', async () => {
    const oldGroups = deferred<Group[]>()
    getAvailable.mockReturnValueOnce(oldGroups.promise).mockResolvedValueOnce([group({ id: 22, name: 'New account group' })])
    const wrapper = mountCreator()
    const firstSignal = getAvailable.mock.calls[0][0].signal as AbortSignal
    expect(getUserGroupRates.mock.calls[0][0].signal).toBe(firstSignal)
    auth.user = { id: 2 }
    await flushPromises()
    expect(firstSignal.aborted).toBe(true)
    oldGroups.resolve([group({ id: 99, name: 'Old account group' })])
    await flushPromises()
    expect(wrapper.text()).toContain('New account group')
    expect(wrapper.text()).not.toContain('Old account group')
  })

  it('does not emit a late create result for another account or reset its in-flight state', async () => {
    const oldCreate = deferred<ApiKey>()
    const newCreate = deferred<ApiKey>()
    create.mockReturnValueOnce(oldCreate.promise).mockReturnValueOnce(newCreate.promise)
    const wrapper = mountCreator()
    await flushPromises()
    await wrapper.get('form').trigger('submit')
    auth.user = { id: 2 }
    await flushPromises()
    await wrapper.get('form').trigger('submit')
    oldCreate.resolve(key())
    await flushPromises()
    expect(wrapper.emitted('created')).toBeUndefined()
    expect(wrapper.get('[data-testid="create-key-submit"]').attributes('disabled')).toBeDefined()
    newCreate.resolve(key({ id: 55 }))
    await flushPromises()
    expect(wrapper.emitted('created')).toHaveLength(1)
    expect((wrapper.emitted('created')![0][0] as ApiKey).id).toBe(55)
  })

  it('clears a previous name on account change even when the same user id signs back in', async () => {
    const pending = deferred<ApiKey>()
    create.mockReturnValueOnce(pending.promise)
    const wrapper = mountCreator()
    await flushPromises()
    await wrapper.get('[data-testid="create-key-name"]').setValue('Old account device')
    await wrapper.get('form').trigger('submit')
    auth.user = null
    auth.user = { id: 1 }
    await flushPromises()
    pending.resolve(key())
    await flushPromises()
    expect(wrapper.emitted('created')).toBeUndefined()
    expect((wrapper.get('[data-testid="create-key-name"]').element as HTMLInputElement).value).toBe('')
  })

  it('aborts reads on unmount and ignores a late create response', async () => {
    const loading = deferred<Group[]>()
    getAvailable.mockReturnValueOnce(loading.promise)
    const loadingWrapper = mountCreator()
    const signal = getAvailable.mock.calls[0][0].signal as AbortSignal
    loadingWrapper.unmount()
    expect(signal.aborted).toBe(true)
    loading.resolve([group()])
    await flushPromises()

    const pending = deferred<ApiKey>()
    create.mockReturnValueOnce(pending.promise)
    const wrapper = mountCreator()
    await flushPromises()
    await wrapper.get('form').trigger('submit')
    wrapper.unmount()
    pending.resolve(key())
    await flushPromises()
    expect(wrapper.emitted('created')).toBeUndefined()
  })

  it('supports an optional cancel action and keeps both locales complete', async () => {
    const wrapper = mountCreator({ showCancel: true })
    await nextTick()
    await wrapper.get('[data-testid="create-key-cancel"]').trigger('click')
    expect(wrapper.emitted('cancel')).toHaveLength(1)
    expect(Object.keys(en).sort()).toEqual(Object.keys(zh).sort())
  })
})
