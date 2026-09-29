import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import QuickConnectPanel from '../QuickConnectPanel.vue'
import type { ApiKey } from '@/types'
import { buildCnOaiSetupScript } from '@/utils/cnOaiSetup'
import { buildCodexSetupScript } from '@/utils/codexOneClick'

const resolveDownloadSpy = vi.hoisted(() => vi.fn())
const startDownloadSpy = vi.hoisted(() => vi.fn())
const listVersionsSpy = vi.hoisted(() => vi.fn())
const directUrlSpy = vi.hoisted(() => vi.fn())
const openSpy = vi.fn()
const mounted: Array<{ unmount: () => void }> = []
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard: vi.fn().mockResolvedValue(true) }) }))
vi.mock('@/api/downloads', () => ({ resolveCCSwitchDownload: resolveDownloadSpy, startCCSwitchDownload: startDownloadSpy, listCCSwitchVersions: listVersionsSpy, buildCCSwitchDirectDownloadURL: directUrlSpy }))

const normalKey = {
  id: 92, name: 'Normal key', key: 'sk-normal-private-key-value', status: 'active', group_id: 10,
  quota: 0, quota_used: 0, expires_at: null,
  group: { id: 10, name: 'Normal group', platform: 'openai', status: 'active' }
} as ApiKey
const cnKey = { ...normalKey, id: 91, name: 'CN key', key: 'sk-cn-private-value', group_id: 9, group: { ...normalKey.group, id: 9, name: '国模 OAI' } } as ApiKey
const otherKey = { ...normalKey, id: 93, name: 'Other key', key: 'sk-other-private-value' } as ApiKey

function mountPanel(overrides: Record<string, unknown> = {}, slots: Record<string, string> = {}) {
  const wrapper = mount(QuickConnectPanel, {
    attachTo: document.body,
    props: { show: true, apiKey: normalKey.key, keyName: normalKey.name, baseUrl: 'https://api.example.com', providerName: 'Example', availableKeys: [normalKey], initialKeyId: normalKey.id, ...overrides },
    slots,
    global: { stubs: { Icon: { template: '<span />' }, CcSwitchAppIcon: { template: '<span />' }, UseKeyModal: true } }
  })
  mounted.push(wrapper)
  return wrapper
}

beforeEach(() => {
  openSpy.mockReset()
  resolveDownloadSpy.mockReset().mockResolvedValue({ file_name: 'app.msi' })
  startDownloadSpy.mockReset()
  listVersionsSpy.mockReset().mockResolvedValue({ versions: [] })
  directUrlSpy.mockReset().mockReturnValue('/custom-api/downloads/cc-switch/file')
  vi.stubGlobal('open', openSpy)
})
afterEach(() => {
  mounted.splice(0).forEach(wrapper => wrapper.unmount())
  vi.useRealTimers()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('QuickConnectPanel guided connection', () => {
  it('shows inline creation for no usable keys and keeps import disabled', async () => {
    const wrapper = mountPanel({ availableKeys: [], apiKey: '', initialKeyId: null }, { 'create-key': '<div data-testid="inline-form">Create here</div>' })
    await flushPromises()
    expect(wrapper.find('[data-testid="inline-form"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="guide-open-ccswitch"]').attributes('disabled')).toBeDefined()
    expect(wrapper.find('[data-testid="connect-manage-keys"]').exists()).toBe(false)
  })
  it('keeps the chosen app after in-place creation even for a manual-setup group', async () => {
    const wrapper = mountPanel({ availableKeys: [], apiKey: '', initialKeyId: null }, { 'create-key': '<div>Create here</div>' })
    const created = { ...normalKey, id: 94, group: { ...normalKey.group!, platform: 'anthropic' } } as ApiKey
    await wrapper.setProps({ availableKeys: [created], initialKeyId: created.id, apiKey: created.key })
    expect(wrapper.get('[data-testid="connect-app-codex"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.find('[data-testid="connect-incompatible-key"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="guide-open-ccswitch"]').attributes('disabled')).toBeDefined()
  })
  it('switches from creation to the created key without importing an old key', async () => {
    const wrapper = mountPanel({}, { 'create-key': '<div data-testid="inline-form">Create here</div>' })
    await wrapper.get('[data-testid="connect-create-key"]').trigger('click')
    expect(wrapper.find('[data-testid="inline-form"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="guide-open-ccswitch"]').attributes('disabled')).toBeDefined()
    await wrapper.setProps({ availableKeys: [normalKey, otherKey], initialKeyId: otherKey.id, apiKey: otherKey.key })
    expect(wrapper.find('[data-testid="inline-form"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="ccswitch-key-select"]').element.value).toBe(String(otherKey.id))
    expect(wrapper.get('[data-testid="guide-open-ccswitch"]').attributes('disabled')).toBeUndefined()
    expect(openSpy).not.toHaveBeenCalled()
  })
  it('does not let local controls unmount a pending create request', async () => {
    const wrapper = mountPanel({}, { 'create-key': '<div data-testid="pending-create">Create</div>' })
    await wrapper.get('[data-testid="connect-create-key"]').trigger('click')
    await wrapper.setProps({ creationBusy: true })
    expect(wrapper.get('[data-testid="connect-use-existing"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="connect-app-claude"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="connect-use-existing"]').trigger('click')
    expect(wrapper.find('[data-testid="pending-create"]').exists()).toBe(true)
  })
  it('starts with installation guidance, masks keys, and keeps advanced settings closed', async () => {
    const wrapper = mountPanel()
    expect(wrapper.find('[data-testid="connect-install-help"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="connect-advanced"]').attributes('open')).toBeUndefined()
    expect(wrapper.get('[data-testid="connect-download-advanced"]').attributes('open')).toBeUndefined()
    expect(wrapper.get('[data-testid="ccswitch-app-selector"]').findAll('button')).toHaveLength(3)
    expect(wrapper.get('[data-testid="connect-more-apps"]').attributes('open')).toBeUndefined()
    expect(wrapper.text()).not.toContain(normalKey.key)
    expect(wrapper.get('[data-testid="ccswitch-key-select"]').text()).toContain('Normal group')
    expect(openSpy).not.toHaveBeenCalled()
    expect(startDownloadSpy).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="connect-toggle-install"]').trigger('click')
    expect(wrapper.find('[data-testid="connect-install-help"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="guide-open-ccswitch"]').exists()).toBe(true)
  })

  it('exposes the supported app catalog and prevents unsupported provider imports', async () => {
    const geminiKey = { ...normalKey, group: { ...normalKey.group, platform: 'gemini' } } as ApiKey
    const wrapper = mountPanel({ availableKeys: [geminiKey] })
    for (const app of ['claude', 'codex', 'gemini', 'grokbuild', 'opencode', 'openclaw', 'hermes']) {
      expect(wrapper.get(`[data-testid="connect-app-${app}"]`).attributes('disabled')).toBeUndefined()
    }
    for (const app of ['claude-desktop', 'pi']) expect(wrapper.get(`[data-testid="connect-app-${app}"]`).attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="connect-app-gemini"]').trigger('click')
    await wrapper.get('[data-testid="guide-open-ccswitch"]').trigger('click')
    const params = new URLSearchParams(openSpy.mock.calls[0][0].split('?')[1])
    expect(params.get('app')).toBe('gemini')
    expect(params.get('apiKey')).toBe(normalKey.key)
  })

  it.each([
    ['anthropic', 'codex'], ['gemini', 'codex'], ['grok', 'codex'], ['deepseek', 'codex'],
    ['openai', 'claude'], ['openai', 'gemini'], ['gemini', 'opencode']
  ])('keeps broader %s / %s combinations available through manual setup only', async (platform, app) => {
    const key = { ...normalKey, group: { ...normalKey.group, platform } } as ApiKey
    const wrapper = mountPanel({ availableKeys: [key] })
    await wrapper.get(`[data-testid="connect-app-${app}"]`).trigger('click')
    expect(wrapper.get(`[data-testid="connect-app-${app}"]`).attributes('aria-pressed')).toBe('true')
    expect(wrapper.get('[data-testid="guide-open-ccswitch"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="connect-manual-config"]').attributes('disabled')).toBeUndefined()
    expect(wrapper.find('[data-testid="connect-incompatible-key"]').exists()).toBe(true)
    await wrapper.get('[data-testid="connect-manual-config"]').trigger('click')
    expect(wrapper.find('use-key-modal-stub').exists()).toBe(true)
  })

  it('allows OpenAI Claude Code import only when Messages dispatch is enabled', async () => {
    const key = { ...normalKey, group: { ...normalKey.group, allow_messages_dispatch: true } } as ApiKey
    const wrapper = mountPanel({ availableKeys: [key] })
    await wrapper.get('[data-testid="connect-app-claude"]').trigger('click')
    expect(wrapper.get('[data-testid="guide-open-ccswitch"]').attributes('disabled')).toBeUndefined()
    await wrapper.get('[data-testid="guide-open-ccswitch"]').trigger('click')
    expect(new URLSearchParams(openSpy.mock.calls[0][0].split('?')[1]).get('app')).toBe('claude')
  })

  it('honors a supported default client and resolves its existing model and endpoint', async () => {
    const wrapper = mountPanel({ defaultApp: 'hermes', baseUrl: 'https://api.example.com/v1/' })
    await wrapper.get('[data-testid="guide-open-ccswitch"]').trigger('click')
    const params = new URLSearchParams(openSpy.mock.calls[0][0].split('?')[1])
    expect(params.get('app')).toBe('hermes')
    expect(params.get('endpoint')).toBe('https://api.example.com/v1')
    expect(params.get('model')).toBe('gpt-5.6')
  })

  it('shows pending confirmation rather than success, including after browser focus changes', async () => {
    vi.useFakeTimers()
    const wrapper = mountPanel()
    await wrapper.get('[data-testid="guide-open-ccswitch"]').trigger('click')
    expect(wrapper.get('[data-testid="connect-import-status"]').text()).toContain('quickConnect.importPending')
    window.dispatchEvent(new Event('blur'))
    await vi.advanceTimersByTimeAsync(2000)
    expect(wrapper.get('[data-testid="connect-import-status"]').text()).toContain('quickConnect.importPending')
    expect(wrapper.emitted('protocol-failed')).toBeUndefined()
    expect(wrapper.get('[data-testid="connect-verify"]').text()).toContain('quickConnect.verifyNote')
  })

  it('offers uncertainty help after the existing timeout without declaring not installed', async () => {
    vi.useFakeTimers()
    const wrapper = mountPanel()
    await wrapper.get('[data-testid="guide-open-ccswitch"]').trigger('click')
    await vi.advanceTimersByTimeAsync(1800)
    expect(wrapper.get('[data-testid="connect-import-status"]').text()).toContain('quickConnect.importUncertain')
    expect(wrapper.emitted('protocol-failed')).toHaveLength(1)
    expect(wrapper.text()).not.toContain('notInstalled')
    await wrapper.get('[data-testid="guide-open-ccswitch"]').trigger('click')
    expect(openSpy).toHaveBeenCalledTimes(2)
    expect(wrapper.get('[data-testid="connect-import-status"]').text()).toContain('quickConnect.importPending')
  })

  it('keeps import exceptions inline and allows retry', async () => {
    openSpy.mockImplementationOnce(() => { throw new Error('protocol rejected') })
    const wrapper = mountPanel()
    await wrapper.get('[data-testid="guide-open-ccswitch"]').trigger('click')
    expect(wrapper.get('[data-testid="connect-import-status"]').text()).toContain('quickConnect.importError')
    await wrapper.get('[data-testid="guide-open-ccswitch"]').trigger('click')
    expect(wrapper.get('[data-testid="connect-import-status"]').text()).toContain('quickConnect.importPending')
  })

  it('does not use a stale prop key after the selected key is removed', async () => {
    const wrapper = mountPanel()
    await wrapper.get('[data-testid="guide-open-ccswitch"]').trigger('click')
    await wrapper.setProps({ availableKeys: [otherKey] })
    expect(wrapper.get('[data-testid="guide-open-ccswitch"]').attributes('disabled')).toBeDefined()
    expect(wrapper.find('[data-testid="connect-import-status"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('quickConnect.selectedUnavailable')
    await wrapper.get('[data-testid="ccswitch-key-select"]').setValue(otherKey.id)
    await wrapper.get('[data-testid="guide-open-ccswitch"]').trigger('click')
    const params = new URLSearchParams(openSpy.mock.calls[1][0].split('?')[1])
    expect(params.get('apiKey')).toBe(otherKey.key)
  })

  it('excludes inactive, expired, exhausted, unassigned and image-only keys', () => {
    const invalid = [
      { ...normalKey, id: 1, status: 'inactive' },
      { ...normalKey, id: 2, expires_at: '2000-01-01T00:00:00Z' },
      { ...normalKey, id: 3, quota: 1, quota_used: 1 },
      { ...normalKey, id: 4, group_id: null, group: undefined },
      { ...normalKey, id: 5, group: { ...normalKey.group, image_only: true } }
    ] as ApiKey[]
    const wrapper = mountPanel({ availableKeys: invalid })
    expect(wrapper.find('[data-testid="connect-no-keys"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="guide-open-ccswitch"]').attributes('disabled')).toBeDefined()
    expect(wrapper.find('[data-testid="connect-codex-script"]').exists()).toBe(false)
  })

  it('does not infer an OpenAI platform for a standalone key without a group', () => {
    const wrapper = mountPanel({ availableKeys: undefined, platform: null })
    expect(wrapper.get('[data-testid="guide-open-ccswitch"]').attributes('disabled')).toBeDefined()
  })

  it('loads a requested key when it arrives without substituting another key', async () => {
    const wrapper = mountPanel({ availableKeys: [], initialKeyId: cnKey.id, loading: true })
    expect(wrapper.get('[data-testid="guide-open-ccswitch"]').attributes('disabled')).toBeDefined()
    await wrapper.setProps({ availableKeys: [normalKey, cnKey], loading: false })
    expect((wrapper.get('[data-testid="ccswitch-key-select"]').element as HTMLSelectElement).value).toBe(String(cnKey.id))
    expect(wrapper.find('[data-testid="cn-oai-setup"]').exists()).toBe(true)
  })

  it('resets model overrides and import status when switching keys', async () => {
    const wrapper = mountPanel({ availableKeys: [normalKey, otherKey] })
    await wrapper.get('[data-testid="connect-model"]').setValue('custom-model')
    await wrapper.get('[data-testid="guide-open-ccswitch"]').trigger('click')
    await wrapper.get('[data-testid="ccswitch-key-select"]').setValue(otherKey.id)
    expect((wrapper.get('[data-testid="connect-model"]').element as HTMLInputElement).value).toBe('')
    expect(wrapper.find('[data-testid="connect-import-status"]').exists()).toBe(false)
    await wrapper.get('[data-testid="guide-open-ccswitch"]').trigger('click')
    const params = new URLSearchParams(openSpy.mock.calls[1][0].split('?')[1])
    expect(params.get('apiKey')).toBe(otherKey.key)
    expect(params.get('model')).toBe('gpt-5.6')
  })

  it('keeps verified Codex downloads and keyboard navigation for operating systems', async () => {
    const wrapper = mountPanel()
    expect(wrapper.get('[data-testid="download-codex-app"]').attributes('href')).toContain('microsoft.com')
    await wrapper.get('[data-testid="guide-os-windows"]').trigger('keydown', { key: 'ArrowRight' })
    expect(wrapper.get('[data-testid="guide-os-linux"]').attributes('aria-checked')).toBe('true')
    await wrapper.get('[data-testid="guide-os-macos"]').trigger('click')
    expect(wrapper.get('[data-testid="download-codex-app"]').attributes('href')).toContain('Codex.dmg')
  })

  it('downloads the selected CC Switch build through the current API prefix', async () => {
    const wrapper = mountPanel()
    await wrapper.get('[data-testid="cc-switch-arch-arm64"]').trigger('click')
    await wrapper.get('[data-testid="ccswitch-version-input"]').setValue('v3.19.1')
    await wrapper.get('[data-testid="download-cc-switch"]').trigger('click')
    expect(resolveDownloadSpy).toHaveBeenCalledWith('windows', 'arm64', 'v3.19.1', expect.any(AbortSignal))
    expect(startDownloadSpy).toHaveBeenCalledWith('/custom-api/downloads/cc-switch/file')
  })

  it('cancels installer requests on OS changes and ignores late responses', async () => {
    let resolve!: (value: unknown) => void
    resolveDownloadSpy.mockReturnValueOnce(new Promise(done => { resolve = done }))
    const wrapper = mountPanel()
    await wrapper.get('[data-testid="download-cc-switch"]').trigger('click')
    const signal = resolveDownloadSpy.mock.calls[0].at(-1) as AbortSignal
    await wrapper.get('[data-testid="guide-os-macos"]').trigger('click')
    expect(signal.aborted).toBe(true)
    resolve({ file_name: 'old.msi' })
    await flushPromises()
    expect(startDownloadSpy).not.toHaveBeenCalled()
  })

  it('cancels both pending request types when hidden', async () => {
    listVersionsSpy.mockReturnValueOnce(new Promise(() => {}))
    resolveDownloadSpy.mockReturnValueOnce(new Promise(() => {}))
    const wrapper = mountPanel()
    await wrapper.get('[data-testid="download-cc-switch"]').trigger('click')
    const downloadSignal = resolveDownloadSpy.mock.calls[0].at(-1) as AbortSignal
    const versionSignal = listVersionsSpy.mock.calls[0].at(-1) as AbortSignal
    await wrapper.setProps({ show: false })
    expect(downloadSignal.aborted).toBe(true)
    expect(versionSignal.aborted).toBe(true)
  })

  it('emits retry and key management actions from empty and error states', async () => {
    const wrapper = mountPanel({ availableKeys: [], loadError: true })
    await wrapper.get('[data-testid="connect-keys-error"] button').trigger('click')
    await wrapper.get('[data-testid="connect-manage-keys"]').trigger('click')
    expect(wrapper.emitted('retry')).toHaveLength(1)
    expect(wrapper.emitted('manage-keys')).toHaveLength(1)
  })

  it('downloads the unchanged dedicated CN OAI script only on explicit action and on Windows', async () => {
    const blobs: Blob[] = []
    const createObjectURL = vi.fn((blob: Blob) => { blobs.push(blob); return 'blob:cn-oai' })
    vi.stubGlobal('URL', class extends URL { static createObjectURL = createObjectURL; static revokeObjectURL = vi.fn() })
    const filenames: string[] = []
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) { filenames.push(this.download) })
    const wrapper = mountPanel({ availableKeys: [cnKey], initialKeyId: cnKey.id })
    expect(createObjectURL).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="connect-advanced"]').attributes('open')).toBeUndefined()
    expect(wrapper.get('[data-testid="download-cn-oai-script"]').element.closest('details')).toBeNull()
    expect(wrapper.find('[data-testid="guide-open-ccswitch"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="connect-model"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="cn-oai-run-steps"]').text()).toContain('quickConnect.cnOaiBackup')
    expect(wrapper.get('[data-testid="cn-oai-run-steps"]').text()).toContain('quickConnect.cnOaiRestart')
    await wrapper.get('[data-testid="cn-oai-model"]').setValue('glm-5.3')
    await wrapper.get('[data-testid="download-cn-oai-script"]').trigger('click')
    expect(filenames).toEqual(['sub2api-cn-oai-setup.cmd'])
    const content = await new Promise<string>((resolve) => { const reader = new FileReader(); reader.onload = () => resolve(String(reader.result)); reader.readAsText(blobs[0]) })
    expect(content).toBe(buildCnOaiSetupScript({ baseUrl: 'https://api.example.com', apiKey: cnKey.key, providerName: 'Example', groupId: 9, model: 'glm-5.3' }))
    await wrapper.get('[data-testid="guide-os-macos"]').trigger('click')
    expect(wrapper.find('[data-testid="download-cn-oai-script"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="guide-open-ccswitch"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="download-codex-script"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="cn-oai-windows-only"]').exists()).toBe(true)
    expect(openSpy).not.toHaveBeenCalled()
  })

  it('promotes dedicated setup immediately when a CN OAI key is selected', async () => {
    const wrapper = mountPanel({ availableKeys: [normalKey, cnKey] })
    await wrapper.get('[data-testid="guide-open-ccswitch"]').trigger('click')
    await wrapper.get('[data-testid="ccswitch-key-select"]').setValue(cnKey.id)
    expect(wrapper.get('[data-testid="download-cn-oai-script"]').element.closest('details')).toBeNull()
    expect(wrapper.find('[data-testid="guide-open-ccswitch"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="connect-import-status"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="connect-verify"]').text()).toContain('quickConnect.cnOaiVerify')
    await wrapper.get('[data-testid="connect-toggle-install"]').trigger('click')
    expect(wrapper.find('[data-testid="download-cn-oai-script"]').exists()).toBe(true)
  })

  it('preserves generated Codex script content', async () => {
    const blobs: Blob[] = []
    vi.stubGlobal('URL', class extends URL { static createObjectURL = (blob: Blob) => { blobs.push(blob); return 'blob:codex' }; static revokeObjectURL = vi.fn() })
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    const wrapper = mountPanel()
    await wrapper.get('[data-testid="download-codex-script"]').trigger('click')
    const content = await new Promise<string>((resolve) => { const reader = new FileReader(); reader.onload = () => resolve(String(reader.result)); reader.readAsText(blobs[0]) })
    expect(content).toBe(buildCodexSetupScript('windows', 'https://api.example.com', normalKey.key))
  })
})
