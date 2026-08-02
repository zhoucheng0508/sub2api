import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import AuditProviderSelector from '../AuditProviderSelector.vue'

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

function mountSelector(provider: 'openai_moderations' | 'ai_chat' = 'openai_moderations') {
  return mount(AuditProviderSelector, {
    props: { modelValue: provider },
    global: {
      stubs: {
        Icon: { template: '<span data-testid="icon" />' },
      },
    },
  })
}

describe('AuditProviderSelector', () => {
  it('renders both supported audit providers', () => {
    const wrapper = mountSelector()
    expect(wrapper.text()).toContain('admin.riskControl.providerOpenAI')
    expect(wrapper.text()).toContain('admin.riskControl.providerAIChat')
    expect(wrapper.findAll('button')).toHaveLength(2)
  })

  it('emits the AI chat provider when its card is selected', async () => {
    const wrapper = mountSelector()
    await wrapper.findAll('button')[1].trigger('click')
    expect(wrapper.emitted('update:modelValue')).toEqual([['ai_chat']])
  })

  it('applies selected styling from the model value', () => {
    const wrapper = mountSelector('ai_chat')
    expect(wrapper.findAll('button')[1].classes()).toContain('border-primary-400')
    expect(wrapper.findAll('button')[0].classes()).toContain('border-gray-200')
  })
})
