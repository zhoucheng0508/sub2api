import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import ScopeEntitySelector from '../ScopeEntitySelector.vue'

const { listUsers, getUserById, listAccounts, getAccountById } = vi.hoisted(() => ({
  listUsers: vi.fn(),
  getUserById: vi.fn(),
  listAccounts: vi.fn(),
  getAccountById: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: { list: listUsers, getById: getUserById },
    accounts: { list: listAccounts, getById: getAccountById },
  },
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) => {
      if (key === 'admin.riskControl.accountFilterShadowLabel') {
        return `${params?.name} (shadow -> parent #${params?.id})`
      }
      return key.replace(/\{(\w+)\}/g, (_, token) => String(params?.[token] ?? `{${token}}`))
    },
  }),
}))

describe('ScopeEntitySelector', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    listUsers.mockReset()
    getUserById.mockReset()
    listAccounts.mockReset()
    getAccountById.mockReset()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('searches users and emits a deduplicated selected user ID', async () => {
    listUsers.mockResolvedValue({
      items: [{ id: 9, email: 'user@example.com', username: 'user', role: 'user' }],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })
    const wrapper = mount(ScopeEntitySelector, {
      props: { entity: 'user', modelValue: [] },
      global: { stubs: { Icon: true } },
    })

    await wrapper.get('[data-test="user-scope-search"]').setValue('user')
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()
    await wrapper.get('[data-test="user-scope-option-9"]').trigger('click')

    expect(listUsers).toHaveBeenCalledWith(1, 20, { search: 'user' })
    expect(wrapper.emitted('update:modelValue')).toEqual([[[9]]])
  })

  it('searches accounts and exposes platform/type context before selection', async () => {
    listAccounts.mockResolvedValue({
      items: [{ id: 12, name: 'OpenAI Pro', platform: 'openai', type: 'oauth' }],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })
    const wrapper = mount(ScopeEntitySelector, {
      props: { entity: 'account', modelValue: [] },
      global: { stubs: { Icon: true } },
    })

    await wrapper.get('[data-test="account-scope-search"]').setValue('pro')
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()

    const option = wrapper.get('[data-test="account-scope-option-12"]')
    expect(option.text()).toContain('OpenAI Pro')
    expect(option.text()).toContain('openai / oauth')
    await option.trigger('click')

    expect(listAccounts).toHaveBeenCalledWith(1, 20, { search: 'pro' })
    expect(wrapper.emitted('update:modelValue')).toEqual([[[12]]])
  })

  it('maps a Spark shadow search result to its parent account ID before selection', async () => {
    listAccounts.mockResolvedValue({
      items: [{
        id: 302,
        name: 'OpenAI Pro Spark',
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
    const wrapper = mount(ScopeEntitySelector, {
      props: { entity: 'account', modelValue: [] },
      global: { stubs: { Icon: true } },
    })

    await wrapper.get('[data-test="account-scope-search"]').setValue('spark')
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()

    expect(wrapper.find('[data-test="account-scope-option-302"]').exists()).toBe(false)
    const option = wrapper.get('[data-test="account-scope-option-202"]')
    expect(option.text()).toContain('OpenAI Pro Spark (shadow -> parent #202)')
    await option.trigger('click')

    expect(wrapper.emitted('update:modelValue')).toEqual([[[202]]])
  })

  it('deduplicates parent and Spark shadow search results by parent account ID', async () => {
    listAccounts.mockResolvedValue({
      items: [
        { id: 302, name: 'OpenAI Pro Spark', platform: 'openai', type: 'oauth', parent_account_id: 202 },
        { id: 202, name: 'OpenAI Pro', platform: 'openai', type: 'oauth', parent_account_id: null },
      ],
      total: 2,
      page: 1,
      page_size: 20,
      pages: 1,
    })
    const wrapper = mount(ScopeEntitySelector, {
      props: { entity: 'account', modelValue: [] },
      global: { stubs: { Icon: true } },
    })

    await wrapper.get('[data-test="account-scope-search"]').setValue('openai')
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()

    const options = wrapper.findAll('[data-test="account-scope-option-202"]')
    expect(options).toHaveLength(1)
    expect(options[0].text()).toContain('OpenAI Pro')
    expect(options[0].text()).not.toContain('shadow ->')
  })

  it('migrates a previously saved Spark shadow ID to its parent account ID', async () => {
    getAccountById.mockResolvedValue({
      id: 302,
      name: 'OpenAI Pro Spark',
      platform: 'openai',
      type: 'oauth',
      parent_account_id: 202,
    })
    const wrapper = mount(ScopeEntitySelector, {
      props: { entity: 'account', modelValue: [302] },
      global: { stubs: { Icon: true } },
    })
    await flushPromises()

    expect(getAccountById).toHaveBeenCalledWith(302)
    expect(wrapper.emitted('update:modelValue')).toEqual([[[202]]])
  })

  it('hydrates saved IDs and keeps unresolved IDs removable', async () => {
    getUserById.mockResolvedValueOnce({ id: 7, email: 'saved@example.com', username: '', role: 'user' })
    getUserById.mockRejectedValueOnce(new Error('not found'))
    const wrapper = mount(ScopeEntitySelector, {
      props: { entity: 'user', modelValue: [7, 42] },
      global: { stubs: { Icon: true } },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('saved@example.com')
    expect(wrapper.text()).toContain('admin.riskControl.userFilterIdFallback')
    const removeButtons = wrapper.findAll('button[aria-label="admin.riskControl.userFilterRemove"]')
    await removeButtons[1].trigger('click')
    expect(wrapper.emitted('update:modelValue')).toEqual([[[7]]])
  })
})
