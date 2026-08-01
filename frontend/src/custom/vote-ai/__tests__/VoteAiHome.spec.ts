import { mount, RouterLinkStub } from '@vue/test-utils'
import { ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import VoteAiHome from '../views/VoteAiHome.vue'

const push = vi.fn()

vi.mock('vue-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-router')>()
  return { ...actual, useRouter: () => ({ push }) }
})

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key, locale: ref('zh-CN') })
  }
})

vi.mock('../components/InteractiveGlobe.vue', () => ({
  default: { template: '<div data-testid="interactive-globe" />' }
}))

function mountHome(siteLogo = '/configured-logo.png') {
  return mount(VoteAiHome, {
    props: {
      siteLogo,
      siteSubtitle: 'Test subtitle',
      isDark: false,
      isAuthenticated: false,
      dashboardPath: '/dashboard',
      userInitial: '',
      currentYear: 2026
    },
    global: {
      stubs: {
        RouterLink: RouterLinkStub,
        LocaleSwitcher: { template: '<div data-testid="locale-switcher" />' },
        Icon: { template: '<span data-testid="icon" />' }
      }
    }
  })
}

describe('VoteAiHome', () => {
  beforeEach(() => push.mockReset())

  it('renders the isolated brand homepage and primary public routes', () => {
    const wrapper = mountHome()
    const destinations = wrapper.findAllComponents(RouterLinkStub).map((link) => link.props('to'))

    expect(wrapper.get('.brand-name').text()).toBe('Vote AI')
    expect(wrapper.get('[data-testid="interactive-globe"]').exists()).toBe(true)
    expect(destinations).toContain('/pricing')
    expect(destinations).toContain('/docs')
  })

  it('prefers the configured site logo and falls back to the Vote AI default', () => {
    expect(mountHome().get('.brand-logo img').attributes('src')).toBe('/configured-logo.png')
    expect(mountHome('').get('.brand-logo img').attributes('src')).toBe('/vote-ai-logo.png')
  })

  it('delegates theme changes to the official HomeView shell', async () => {
    const wrapper = mountHome()
    await wrapper.get('.icon-button').trigger('click')
    expect(wrapper.emitted('toggleTheme')).toHaveLength(1)
  })
})
