import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import CodexOneClickModal from '../CodexOneClickModal.vue'
import type { ApiKey, GroupPlatform } from '@/types'
import type { CcSwitchAppType } from '@/utils/ccswitchImport'

const openSpy = vi.fn()
const resolveDownloadSpy = vi.hoisted(() => vi.fn())
const startDownloadSpy = vi.hoisted(() => vi.fn())
const listVersionsSpy = vi.hoisted(() => vi.fn())
const directUrlSpy = vi.hoisted(() => vi.fn())
const mounted: Array<{ unmount: () => void }> = []

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: vi.fn().mockResolvedValue(true) })
}))
vi.mock('@/api/downloads', () => ({
  resolveCCSwitchDownload: resolveDownloadSpy,
  startCCSwitchDownload: startDownloadSpy,
  listCCSwitchVersions: listVersionsSpy,
  buildCCSwitchDirectDownloadURL: directUrlSpy
}))

describe('CodexOneClickModal', () => {
  beforeEach(() => {
    openSpy.mockReset()
    resolveDownloadSpy.mockReset().mockResolvedValue({
      download_url: 'https://github.com/farion1231/cc-switch/releases/download/v3.19.1/app.msi',
      file_name: 'app.msi',
      release_url: 'https://github.com/farion1231/cc-switch/releases/tag/v3.19.1'
    })
    startDownloadSpy.mockReset()
    listVersionsSpy.mockReset().mockResolvedValue({ versions: [] })
    directUrlSpy.mockReset().mockReturnValue('/api/v1/downloads/cc-switch/file')
    vi.stubGlobal('open', openSpy)
  })

  afterEach(() => {
    mounted.splice(0).forEach(wrapper => wrapper.unmount())
    vi.useRealTimers()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  function mountModal(overrides: Partial<{
    platform: GroupPlatform | null
    defaultApp: CcSwitchAppType
    initialMethod: 'guide' | 'ccswitch' | 'script'
    availableKeys: ApiKey[]
    initialKeyId: number | null
  }> = {}) {
    const wrapper = mount(CodexOneClickModal, {
      attachTo: document.body,
      props: {
        show: true,
        apiKey: 'sk-complete-secret-value',
        keyName: 'Codex key',
        baseUrl: 'https://api.example.com',
        providerName: 'Example',
        ...overrides
      },
      global: { stubs: {
        BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
        Icon: { template: '<span />' }
      } }
    })
    mounted.push(wrapper)
    return wrapper
  }

  const cnKey = {
    id: 91, name: 'CN', key: 'sk-cn', status: 'active',
    group: { id: 9, name: '国模 OAI', platform: 'openai' }
  } as ApiKey
  const normalKey = {
    ...cnKey, id: 92, name: 'Normal', key: 'sk-normal',
    group: { id: 10, name: 'Normal', platform: 'openai' }
  } as ApiKey

  it('shows only the Codex setup guide for a normal group', () => {
    const wrapper = mountModal({ defaultApp: 'gemini', initialMethod: 'ccswitch' })
    expect(wrapper.find('[data-testid="ccswitch-app-selector"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="codex-method-guide"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="codex-method-cn-oai"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="codex-method-ccswitch"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="codex-method-script"]').exists()).toBe(false)
    expect(wrapper.get('[role="tabpanel"]').attributes('id')).toBe('codex-method-panel-guide')
    expect(wrapper.text()).not.toContain('sk-complete-secret-value')
  })

  it('shows only setup guide and CN OAI setup for the dedicated group', async () => {
    const wrapper = mountModal({ availableKeys: [cnKey], initialKeyId: 91 })
    expect(wrapper.findAll('[role="tab"]').map(tab => tab.attributes('data-testid'))).toEqual([
      'codex-method-guide', 'codex-method-cn-oai'
    ])
    await wrapper.get('[data-testid="codex-method-cn-oai"]').trigger('click')
    expect(wrapper.find('[data-testid="cn-oai-setup"]').exists()).toBe(true)
  })

  it('labels both flows as three explicit steps', async () => {
    const wrapper = mountModal({ availableKeys: [cnKey], initialKeyId: 91 })
    for (const key of ['stepOne', 'stepTwo', 'stepThree']) expect(wrapper.text()).toContain(`keys.oneClick.${key}`)
    await wrapper.get('[data-testid="codex-method-cn-oai"]').trigger('click')
    const steps = wrapper.get('[data-testid="cn-oai-setup-steps"]')
    expect(steps.findAll('li')).toHaveLength(3)
    for (const key of ['stepOne', 'stepTwo', 'stepThree']) expect(steps.text()).toContain(`keys.oneClick.${key}`)
  })

  it('returns to guide when changing from CN OAI to a normal key', async () => {
    const wrapper = mountModal({ availableKeys: [cnKey, normalKey], initialKeyId: 91 })
    await wrapper.get('[data-testid="codex-method-cn-oai"]').trigger('click')
    await wrapper.get('[data-testid="ccswitch-key-select"]').setValue(92)
    expect(wrapper.find('[data-testid="codex-method-cn-oai"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="codex-method-guide"]').attributes('aria-selected')).toBe('true')
    expect(wrapper.text()).not.toContain('sk-normal')
  })

  it('downloads the dedicated CN OAI CMD only after an explicit click', async () => {
    const createObjectURL = vi.fn(() => 'blob:cn-oai')
    vi.stubGlobal('URL', class extends URL {
      static createObjectURL = createObjectURL
      static revokeObjectURL = vi.fn()
    })
    const filenames: string[] = []
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) { filenames.push(this.download) })
    const wrapper = mountModal({ availableKeys: [cnKey], initialKeyId: 91 })
    await wrapper.get('[data-testid="codex-method-cn-oai"]').trigger('click')
    await wrapper.get('[data-testid="cn-oai-model"]').setValue('glm-5.3')
    await wrapper.get('[data-testid="download-cn-oai-script"]').trigger('click')
    expect(filenames).toEqual(['sub2api-cn-oai-setup.cmd'])
    expect(createObjectURL).toHaveBeenCalledTimes(1)
    expect(openSpy).not.toHaveBeenCalled()
  })

  it('keeps verified Codex downloads and OS keyboard navigation', async () => {
    const wrapper = mountModal()
    expect(wrapper.get('[data-testid="download-codex-app"]').attributes('href')).toContain('microsoft.com')
    await wrapper.get('[data-testid="guide-os-windows"]').trigger('keydown', { key: 'ArrowRight' })
    expect(wrapper.get('[data-testid="guide-os-linux"]').attributes('aria-checked')).toBe('true')
    await wrapper.get('[data-testid="guide-os-macos"]').trigger('click')
    expect(wrapper.get('[data-testid="download-codex-app"]').attributes('href')).toContain('Codex.dmg')
  })

  it('downloads the selected CC Switch build from the guide', async () => {
    const wrapper = mountModal()
    await wrapper.get('[data-testid="cc-switch-arch-arm64"]').trigger('click')
    await wrapper.get('[data-testid="download-cc-switch"]').trigger('click')
    expect(resolveDownloadSpy).toHaveBeenCalledWith('windows', 'arm64', expect.any(AbortSignal))
    expect(startDownloadSpy).toHaveBeenCalledWith('/api/v1/downloads/cc-switch/file')
  })

  it('imports the selected key into Codex from step three', async () => {
    const wrapper = mountModal({ availableKeys: [normalKey], initialKeyId: 92 })
    await wrapper.get('[data-testid="guide-open-ccswitch"]').trigger('click')
    const params = new URLSearchParams((openSpy.mock.calls[0][0] as string).split('?')[1])
    expect(params.get('app')).toBe('codex')
    expect(params.get('apiKey')).toBe('sk-normal')
    expect(params.get('endpoint')).toBe('https://api.example.com/v1')
  })

  it('cancels a pending installer request when closed', async () => {
    resolveDownloadSpy.mockReturnValueOnce(new Promise(() => {}))
    const wrapper = mountModal()
    await wrapper.get('[data-testid="download-cc-switch"]').trigger('click')
    const signal = resolveDownloadSpy.mock.calls[0].at(-1) as AbortSignal
    await wrapper.setProps({ show: false })
    expect(signal.aborted).toBe(true)
  })
})
