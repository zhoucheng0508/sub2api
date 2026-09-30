import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
const { fetchModels, qr } = vi.hoisted(() => ({ fetchModels: vi.fn(), qr: vi.fn() }))
vi.mock('@/api/codex', async (importOriginal) => ({
  ...await importOriginal<typeof import('@/api/codex')>(),
  fetchCodexModelsManifest: fetchModels
}))
vi.mock('qrcode', () => ({ default: { toDataURL: qr } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard: vi.fn() }) }))
import RikkaHubImport from './RikkaHubImport.vue'
import UseKeyModal from '@/components/keys/UseKeyModal.vue'

const catalog = { content: JSON.stringify({ models: [{ slug: 'gpt-6-astra', input_modalities: ['text', 'image'], supported_reasoning_levels: [{ effort: 'low' }] }, { slug: 'plain' }] }), modelCount: 2 }
const wrappers: ReturnType<typeof mount>[] = []
function render() {
  fetchModels.mockResolvedValue(catalog)
  qr.mockResolvedValue('data:image/png;base64,test')
  const wrapper = mount(RikkaHubImport, { props: { apiKey: 'test-key-a', baseUrl: 'https://example.com' } })
  wrappers.push(wrapper)
  return wrapper
}
afterEach(() => { wrappers.forEach(w => w.unmount()); wrappers.length = 0; vi.clearAllMocks() })

describe('RikkaHub import privacy and integration', () => {
  it('waits for explicit display and clears QR when model changes', async () => {
    const wrapper = render()
    await flushPromises()
    expect(qr).not.toHaveBeenCalled()
    expect(wrapper.html()).not.toContain('test-key-a')
    await wrapper.get('[data-testid="rikka-generate"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="rikka-qr"]').exists()).toBe(true)
    await wrapper.get('select').setValue('plain')
    expect(wrapper.find('[data-testid="rikka-qr"]').exists()).toBe(false)
  })

  it('discards a pending QR result when the key changes', async () => {
    const wrapper = render()
    await flushPromises()
    let finish!: (url: string) => void
    qr.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    await wrapper.get('[data-testid="rikka-generate"]').trigger('click')
    await wrapper.setProps({ apiKey: 'test-key-b' })
    finish('data:image/png;base64,old-key')
    await flushPromises()
    expect(wrapper.find('[data-testid="rikka-qr"]').exists()).toBe(false)
    expect(fetchModels).toHaveBeenLastCalledWith('https://example.com/v1', 'test-key-b', expect.any(AbortSignal))
  })

  it('shows a generic failure without exposing upstream error content', async () => {
    fetchModels.mockRejectedValueOnce(new Error('secret upstream response'))
    const wrapper = render()
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    expect(wrapper.html()).not.toContain('secret upstream response')
    expect(wrapper.find('[data-testid="rikka-generate"]').exists()).toBe(false)
  })

  it('keeps the historical module disconnected from manual configuration', async () => {
    fetchModels.mockResolvedValue(catalog)
    const wrapper = mount(UseKeyModal, {
      props: { show: true, platform: 'openai', apiKey: 'test-key-a', baseUrl: 'https://example.com' },
      global: { stubs: { BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' }, Icon: true } }
    })
    wrappers.push(wrapper)
    await flushPromises()
    expect(fetchModels).not.toHaveBeenCalled()
    expect(qr).not.toHaveBeenCalled()
    expect(wrapper.text()).not.toContain('RikkaHub')
    expect(wrapper.find('[data-testid="rikkahub-import"]').exists()).toBe(false)
    expect(wrapper.findAll('pre').length).toBeGreaterThan(0)
  })
})
