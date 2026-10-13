import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import FirstTokenMonitorSettings from '../FirstTokenMonitorSettings.vue'

describe('first token monitoring account controls', () => {
  const settings = { enabled: false, thresholdSeconds: 30, anomalyCount: 3, cooldownMinutes: 10 }
  const plugin = createI18n({ legacy: false, locale: 'en', missingWarn: false, fallbackWarn: false })
  it('exposes an accessible opt-in switch and shows settings only after enabling', async () => {
    const wrapper = mount(FirstTokenMonitorSettings, { props: { modelValue: settings }, global: { plugins: [plugin] } })
    expect(wrapper.find('[role="switch"]').attributes('aria-checked')).toBe('false')
    expect(wrapper.findAll('input')).toHaveLength(0)
    await wrapper.find('[role="switch"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([{ ...settings, enabled: true }])
    await wrapper.setProps({ modelValue: { ...settings, enabled: true } })
    expect(wrapper.findAll('input').map(input => (input.element as HTMLInputElement).value)).toEqual(['30', '3', '10'])
    await wrapper.findAll('input')[0].setValue('45')
    expect(wrapper.emitted('update:modelValue')?.[1]).toEqual([{ ...settings, enabled: true, thresholdSeconds: 45 }])
  })
})
