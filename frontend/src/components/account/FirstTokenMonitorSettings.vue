<template>
  <div class="space-y-4 border-t border-gray-200 pt-4 dark:border-dark-600">
    <div class="flex items-center justify-between gap-4">
      <div>
        <label class="input-label mb-0" :id="labelId">{{ t('admin.accounts.firstTokenMonitor.title') }}</label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.firstTokenMonitor.hint') }}</p>
      </div>
      <button type="button" role="switch" :aria-checked="modelValue.enabled" :aria-labelledby="labelId"
        class="relative inline-flex h-6 w-11 shrink-0 rounded-full border-2 border-transparent transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500"
        :class="modelValue.enabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'"
        @click="update('enabled', !modelValue.enabled)">
        <span class="inline-block h-5 w-5 rounded-full bg-white shadow transition-transform" :class="modelValue.enabled ? 'translate-x-5' : 'translate-x-0'" />
      </button>
    </div>
    <div v-if="modelValue.enabled" class="grid grid-cols-1 gap-3 sm:grid-cols-3">
      <label class="input-label">{{ t('admin.accounts.firstTokenMonitor.threshold') }}
        <input class="input mt-1" type="number" min="1" max="3600" step="1" :value="modelValue.thresholdSeconds" @input="update('thresholdSeconds', Number(($event.target as HTMLInputElement).value))" />
      </label>
      <label class="input-label">{{ t('admin.accounts.firstTokenMonitor.anomalyCount') }}
        <input class="input mt-1" type="number" min="1" max="100" step="1" :value="modelValue.anomalyCount" @input="update('anomalyCount', Number(($event.target as HTMLInputElement).value))" />
      </label>
      <label class="input-label">{{ t('admin.accounts.firstTokenMonitor.cooldown') }}
        <input class="input mt-1" type="number" min="1" max="1440" step="1" :value="modelValue.cooldownMinutes" @input="update('cooldownMinutes', Number(($event.target as HTMLInputElement).value))" />
      </label>
    </div>
    <p v-if="modelValue.enabled" class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.firstTokenMonitor.notice') }}</p>
  </div>
</template>

<script setup lang="ts">
import { getCurrentInstance } from 'vue'
import { useI18n } from 'vue-i18n'
type Settings = { enabled: boolean; thresholdSeconds: number; anomalyCount: number; cooldownMinutes: number }
const props = defineProps<{ modelValue: Settings }>()
const emit = defineEmits<{ 'update:modelValue': [value: Settings] }>()
const { t } = useI18n()
const labelId = `first-token-monitor-${getCurrentInstance()?.uid}`
function update<K extends keyof Settings>(key: K, value: Settings[K]) {
  emit('update:modelValue', { ...props.modelValue, [key]: value })
}
</script>
