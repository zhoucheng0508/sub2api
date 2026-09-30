import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import CodexOneClickModal from '../CodexOneClickModal.vue'
import QuickConnectPanel from '../QuickConnectPanel.vue'

vi.mock('vue-i18n', async (importOriginal) => ({ ...(await importOriginal<typeof import('vue-i18n')>()), useI18n: () => ({ t: (key: string) => key }) }))

describe('CodexOneClickModal compatibility wrapper', () => {
  it('passes connection props to the shared panel and forwards existing events', () => {
    const props = {
      show: true, apiKey: 'sk-private-value', keyName: 'Key', baseUrl: 'https://api.example.com', providerName: 'Example',
      platform: 'openai' as const, initialMethod: 'ccswitch' as const, initialKeyId: 42, availableKeys: [], loading: true
    }
    const wrapper = mount(CodexOneClickModal, {
      props,
      global: { stubs: { BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' }, QuickConnectPanel: true } }
    })
    const panel = wrapper.findComponent(QuickConnectPanel)
    expect(panel.props()).toMatchObject(props)
    panel.vm.$emit('manage-keys')
    panel.vm.$emit('protocol-failed')
    panel.vm.$emit('retry')
    expect(wrapper.emitted('manage-keys')).toHaveLength(1)
    expect(wrapper.emitted('protocol-failed')).toHaveLength(1)
    expect(wrapper.emitted('retry')).toHaveLength(1)
    wrapper.unmount()
  })

  it('preserves the close event', async () => {
    const wrapper = mount(CodexOneClickModal, {
      props: { show: true, apiKey: '', keyName: '', baseUrl: '', providerName: '' },
      global: { stubs: { BaseDialog: { template: '<div><slot name="footer" /></div>' }, QuickConnectPanel: true } }
    })
    await wrapper.get('button').trigger('click')
    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()
  })
})
