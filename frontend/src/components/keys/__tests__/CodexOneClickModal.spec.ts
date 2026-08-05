import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import CodexOneClickModal from '../CodexOneClickModal.vue'
import { buildCodexSetupScript } from '@/utils/codexOneClick'

const openSpy = vi.fn()
const clipboardCopySpy = vi.hoisted(() => vi.fn())
const mountedWrappers: Array<{ unmount: () => void }> = []

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string, params?: Record<string, string>) => `${key}${params?.name ? `:${params.name}` : ''}` })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: clipboardCopySpy })
}))

describe('CodexOneClickModal', () => {
  beforeEach(() => {
    openSpy.mockReset()
    clipboardCopySpy.mockReset()
    clipboardCopySpy.mockResolvedValue(true)
    vi.stubGlobal('open', openSpy)
  })

  afterEach(() => {
    mountedWrappers.splice(0).forEach((wrapper) => wrapper.unmount())
    vi.useRealTimers()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  const mountModal = (errorHandler?: (error: unknown) => void) => {
    const wrapper = mount(CodexOneClickModal, {
      attachTo: document.body,
      props: {
        show: true,
        apiKey: 'sk-complete-secret-value',
        keyName: 'Codex key',
        baseUrl: 'https://api.example.com',
        providerName: 'Example'
      },
      global: {
        ...(errorHandler ? { config: { errorHandler } } : {}),
        stubs: {
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: { template: '<span />' }
        }
      }
    })
    mountedWrappers.push(wrapper)
    return wrapper
  }

  it('renders only masked key material before an explicit action', async () => {
    const wrapper = mountModal()

    expect(wrapper.text()).not.toContain('sk-complete-secret-value')
    expect(wrapper.find('[data-testid="codex-method-guide"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="codex-method-ccswitch"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="codex-method-script"]').exists()).toBe(true)

    await wrapper.get('[data-testid="codex-method-script"]').trigger('click')
    expect(wrapper.text()).not.toContain('sk-complete-secret-value')
  })

  it('renders verified Codex and CC Switch downloads for each operating system', async () => {
    const wrapper = mountModal()
    const codexDownload = wrapper.get('[data-testid="download-codex-app"]')
    const ccSwitchDownload = wrapper.get('[data-testid="download-cc-switch"]')

    expect(codexDownload.attributes('href')).toBe(
      'https://get.microsoft.com/installer/download/9PLM9XGG6VKS?cid=website_cta_psi'
    )
    expect(codexDownload.text()).toContain('keys.oneClick.downloadCodexWindows')
    expect(ccSwitchDownload.attributes('href')).toBe(
      'https://github.com/farion1231/cc-switch/releases/latest'
    )
    expect(codexDownload.attributes('rel')).toBe('noopener noreferrer')

    await wrapper.get('[data-testid="guide-os-macos"]').trigger('click')
    expect(wrapper.get('[data-testid="download-codex-app"]').attributes('href')).toBe(
      'https://persistent.oaistatic.com/codex-app-prod/Codex.dmg'
    )
    expect(wrapper.get('[data-testid="download-codex-app"]').text()).toContain('keys.oneClick.downloadCodexMacos')

    await wrapper.get('[data-testid="guide-os-linux"]').trigger('click')
    expect(wrapper.get('[data-testid="download-codex-app"]').attributes('href')).toBe(
      'https://learn.chatgpt.com/docs/codex/cli'
    )
    expect(wrapper.get('[data-testid="download-codex-app"]').text()).toContain('keys.oneClick.openCodexLinuxGuide')
    expect(wrapper.text()).toContain('keys.oneClick.officialAddressHint')
  })

  it('keeps the guide key-free until step three starts the CC Switch import', async () => {
    const wrapper = mountModal()
    expect(wrapper.text()).not.toContain('sk-complete-secret-value')
    expect(openSpy).not.toHaveBeenCalled()

    await wrapper.get('[data-testid="guide-open-ccswitch"]').trigger('click')

    expect(openSpy).toHaveBeenCalledTimes(1)
    const deeplink = openSpy.mock.calls[0][0] as string
    const params = new URLSearchParams(deeplink.split('?')[1])
    expect(params.get('app')).toBe('codex')
    expect(params.get('endpoint')).toBe('https://api.example.com/v1')
    expect(params.get('apiKey')).toBe('sk-complete-secret-value')
    const usageScript = atob(params.get('usageScript') || '')
    expect(usageScript).toContain('https://api.example.com/v1/usage')
    expect(usageScript).not.toContain('/v1/v1/usage')
  })

  it('creates the CC Switch payload only after the user clicks import', async () => {
    const wrapper = mountModal()
    expect(openSpy).not.toHaveBeenCalled()

    await wrapper.get('[data-testid="codex-method-ccswitch"]').trigger('click')
    await wrapper.get('[data-testid="open-ccswitch"]').trigger('click')

    expect(openSpy).toHaveBeenCalledTimes(1)
    const deeplink = openSpy.mock.calls[0][0] as string
    const params = new URLSearchParams(deeplink.split('?')[1])
    expect(params.get('app')).toBe('codex')
    expect(params.get('endpoint')).toBe('https://api.example.com/v1')
    expect(params.get('apiKey')).toBe('sk-complete-secret-value')
  })

  it('reports a missing protocol handler only after the lifecycle timeout', async () => {
    vi.useFakeTimers()
    const wrapper = mountModal()

    await wrapper.get('[data-testid="codex-method-ccswitch"]').trigger('click')
    await wrapper.get('[data-testid="open-ccswitch"]').trigger('click')
    expect(wrapper.emitted('protocol-failed')).toBeUndefined()

    vi.advanceTimersByTime(1799)
    expect(wrapper.emitted('protocol-failed')).toBeUndefined()
    vi.advanceTimersByTime(1)
    expect(wrapper.emitted('protocol-failed')).toHaveLength(1)
    vi.advanceTimersByTime(1000)
    expect(wrapper.emitted('protocol-failed')).toHaveLength(1)
  })

  it('cancels protocol failure detection after a delayed window blur', async () => {
    vi.useFakeTimers()
    const wrapper = mountModal()

    await wrapper.get('[data-testid="guide-open-ccswitch"]').trigger('click')
    vi.advanceTimersByTime(1200)
    window.dispatchEvent(new Event('blur'))
    vi.advanceTimersByTime(2000)

    expect(wrapper.emitted('protocol-failed')).toBeUndefined()
  })

  it('cancels protocol failure detection when the document becomes hidden', async () => {
    vi.useFakeTimers()
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden')
    const wrapper = mountModal()

    await wrapper.get('[data-testid="guide-open-ccswitch"]').trigger('click')
    document.dispatchEvent(new Event('visibilitychange'))
    vi.advanceTimersByTime(2000)

    expect(wrapper.emitted('protocol-failed')).toBeUndefined()
  })

  it('cleans protocol listeners and stale timers when the modal closes and reopens', async () => {
    vi.useFakeTimers()
    const removeWindowListener = vi.spyOn(window, 'removeEventListener')
    const removeDocumentListener = vi.spyOn(document, 'removeEventListener')
    const wrapper = mountModal()

    await wrapper.get('[data-testid="guide-open-ccswitch"]').trigger('click')
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    vi.advanceTimersByTime(3000)

    expect(wrapper.emitted('protocol-failed')).toBeUndefined()
    expect(removeWindowListener).toHaveBeenCalledWith('blur', expect.any(Function))
    expect(removeDocumentListener).toHaveBeenCalledWith('visibilitychange', expect.any(Function))
  })

  it('cancels protocol failure detection when the modal unmounts', async () => {
    vi.useFakeTimers()
    const removeWindowListener = vi.spyOn(window, 'removeEventListener')
    const removeDocumentListener = vi.spyOn(document, 'removeEventListener')
    const wrapper = mountModal()

    await wrapper.get('[data-testid="guide-open-ccswitch"]').trigger('click')
    wrapper.unmount()
    mountedWrappers.splice(mountedWrappers.indexOf(wrapper), 1)
    vi.advanceTimersByTime(3000)

    expect(wrapper.emitted('protocol-failed')).toBeUndefined()
    expect(removeWindowListener).toHaveBeenCalledWith('blur', expect.any(Function))
    expect(removeDocumentListener).toHaveBeenCalledWith('visibilitychange', expect.any(Function))
  })

  it('connects tabs and panels and supports roving keyboard focus', async () => {
    const wrapper = mountModal()
    const guide = wrapper.get('[data-testid="codex-method-guide"]')

    expect(guide.attributes('id')).toBe('codex-method-tab-guide')
    expect(guide.attributes('aria-controls')).toBe('codex-method-panel-guide')
    expect(guide.attributes('aria-selected')).toBe('true')
    expect(guide.attributes('tabindex')).toBe('0')
    expect(wrapper.get('[role="tabpanel"]').attributes('aria-labelledby')).toBe('codex-method-tab-guide')

    await guide.trigger('keydown', { key: 'ArrowRight' })
    const ccSwitch = wrapper.get('[data-testid="codex-method-ccswitch"]')
    expect(ccSwitch.attributes('aria-selected')).toBe('true')
    expect(ccSwitch.attributes('tabindex')).toBe('0')
    expect(document.activeElement).toBe(ccSwitch.element)

    await ccSwitch.trigger('keydown', { key: 'End' })
    const script = wrapper.get('[data-testid="codex-method-script"]')
    expect(script.attributes('aria-selected')).toBe('true')
    expect(wrapper.get('[role="tabpanel"]').attributes('id')).toBe('codex-method-panel-script')

    await script.trigger('keydown', { key: 'Home' })
    expect(wrapper.get('[data-testid="codex-method-guide"]').attributes('aria-selected')).toBe('true')
  })

  it('supports roving keyboard focus for operating-system radios', async () => {
    const wrapper = mountModal()
    const windows = wrapper.get('[data-testid="guide-os-windows"]')

    expect(windows.attributes('aria-checked')).toBe('true')
    expect(windows.attributes('tabindex')).toBe('0')
    expect(wrapper.get('[data-testid="guide-os-macos"]').attributes('tabindex')).toBe('-1')

    await windows.trigger('keydown', { key: 'ArrowRight' })
    const linux = wrapper.get('[data-testid="guide-os-linux"]')
    expect(linux.attributes('aria-checked')).toBe('true')
    expect(linux.attributes('tabindex')).toBe('0')
    expect(document.activeElement).toBe(linux.element)

    await linux.trigger('keydown', { key: 'Home' })
    const macos = wrapper.get('[data-testid="guide-os-macos"]')
    expect(macos.attributes('aria-checked')).toBe('true')
    expect(document.activeElement).toBe(macos.element)
  })

  it('delays object URL cleanup until after the download click', async () => {
    vi.useFakeTimers()
    const createObjectURL = vi.fn(() => 'blob:codex-script')
    const revokeObjectURL = vi.fn()
    vi.stubGlobal('URL', { ...URL, createObjectURL, revokeObjectURL })
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    const wrapper = mountModal()

    await wrapper.get('[data-testid="codex-method-script"]').trigger('click')
    await wrapper.get('[data-testid="download-codex-script"]').trigger('click')

    expect(click).toHaveBeenCalledTimes(1)
    expect(createObjectURL).toHaveBeenCalledTimes(1)
    expect(revokeObjectURL).not.toHaveBeenCalled()
    vi.advanceTimersByTime(999)
    expect(revokeObjectURL).not.toHaveBeenCalled()
    vi.advanceTimersByTime(1)
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:codex-script')
  })

  it('still revokes the object URL when the download click throws', async () => {
    vi.useFakeTimers()
    const createObjectURL = vi.fn(() => 'blob:failed-codex-script')
    const revokeObjectURL = vi.fn()
    vi.stubGlobal('URL', { ...URL, createObjectURL, revokeObjectURL })
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {
      throw new Error('download blocked')
    })
    const errorHandler = vi.fn()
    const wrapper = mountModal(errorHandler)

    await wrapper.get('[data-testid="codex-method-script"]').trigger('click')
    await wrapper.get('[data-testid="download-codex-script"]').trigger('click')

    expect(errorHandler).toHaveBeenCalled()
    expect(errorHandler.mock.calls[0][0]).toEqual(expect.objectContaining({ message: 'download blocked' }))
    expect(revokeObjectURL).not.toHaveBeenCalled()
    vi.advanceTimersByTime(1000)
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:failed-codex-script')
  })

  it('shows a real script preview without exposing either generated payload', async () => {
    const wrapper = mountModal()
    const fullScript = buildCodexSetupScript('windows', 'https://api.example.com', 'sk-complete-secret-value')
    const payloads = [...fullScript.matchAll(/FromBase64String\('([^']+)'\)/g)].map((match) => match[1])

    await wrapper.get('[data-testid="codex-method-script"]').trigger('click')
    const preview = wrapper.get('[data-testid="codex-script-preview"]').text()

    expect(wrapper.get('[data-testid="codex-script-heading-row"]').classes()).toEqual(
      expect.arrayContaining(['flex-col', 'sm:flex-row'])
    )
    expect(wrapper.get('[data-testid="codex-script-toolbar"] [data-testid="copy-codex-script"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="codex-script-preview"]').classes()).toEqual(
      expect.arrayContaining(['max-h-32', 'whitespace-pre-wrap', 'break-all'])
    )
    expect(preview).toContain("$ErrorActionPreference = 'Stop'")
    expect(preview).toContain('<CONFIG_TOML_BASE64_REDACTED>')
    expect(preview).toContain('<AUTH_JSON_BASE64_REDACTED>')
    expect(preview).not.toContain('sk-complete-secret-value')
    expect(payloads).toHaveLength(2)
    payloads.forEach((payload) => expect(preview).not.toContain(payload))
  })

  it('generates and copies the complete runnable script only after the user clicks copy', async () => {
    const wrapper = mountModal()
    await wrapper.get('[data-testid="codex-method-script"]').trigger('click')

    expect(clipboardCopySpy).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="copy-codex-script"]').trigger('click')

    const expected = buildCodexSetupScript('windows', 'https://api.example.com', 'sk-complete-secret-value')
    expect(clipboardCopySpy).toHaveBeenCalledWith(expected, 'keys.oneClick.scriptCopied')
    expect(wrapper.get('[data-testid="copy-codex-script"]').text()).toContain('keys.oneClick.scriptCopied')
  })

  it('shows a visible fallback when copying fails', async () => {
    clipboardCopySpy.mockResolvedValue(false)
    const wrapper = mountModal()
    await wrapper.get('[data-testid="codex-method-script"]').trigger('click')
    await wrapper.get('[data-testid="copy-codex-script"]').trigger('click')

    expect(wrapper.get('[data-testid="copy-script-error"]').text()).toContain('keys.oneClick.scriptCopyFailed')
  })

  it('shows the OS-specific command for running the downloaded script', async () => {
    const wrapper = mountModal()
    await wrapper.get('[data-testid="codex-method-script"]').trigger('click')

    expect(wrapper.get('[data-testid="codex-script-run-command"]').text()).toBe(
      'powershell -ExecutionPolicy Bypass -File "$env:USERPROFILE\\Downloads\\sub2api-codex-windows.ps1"'
    )
    expect(wrapper.get('[data-testid="codex-script-run-command"]').classes()).toEqual(
      expect.arrayContaining(['whitespace-pre-wrap', 'break-all'])
    )
    expect(wrapper.get('[data-testid="codex-script-run-row"]').classes()).toEqual(
      expect.arrayContaining(['flex-col', 'sm:flex-row'])
    )

    await wrapper.get('[data-testid="codex-os-macos"]').trigger('click')
    expect(wrapper.get('[data-testid="codex-script-run-command"]').text()).toBe(
      'sh ~/Downloads/sub2api-codex-macos.sh'
    )

    await wrapper.get('[data-testid="codex-os-linux"]').trigger('click')
    expect(wrapper.get('[data-testid="codex-script-run-command"]').text()).toBe(
      'sh ~/Downloads/sub2api-codex-linux.sh'
    )
  })
})
