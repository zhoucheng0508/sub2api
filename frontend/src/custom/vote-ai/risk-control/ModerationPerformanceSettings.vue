<template>
  <fieldset class="border-t border-gray-100 pt-4 dark:border-dark-700">
    <legend class="px-1 text-sm font-semibold text-gray-900 dark:text-white">
      {{ t('admin.riskControl.aiPerformanceSettings') }}
    </legend>
    <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">
      {{ t('admin.riskControl.aiPerformanceSettingsHint') }}
    </p>
    <div class="mt-4 grid grid-cols-1 gap-4 md:grid-cols-3">
      <div>
        <label for="vote-ai-moderation-synchronous-budget" class="input-label">{{ t('admin.riskControl.aiSynchronousBudget') }}</label>
        <input
          id="vote-ai-moderation-synchronous-budget"
          :value="synchronousBudgetMs"
          type="number"
          min="500"
          max="5000"
          step="100"
          class="input"
          data-test="ai-synchronous-budget-ms"
          @input="updateSynchronousBudget"
        />
        <p class="mt-2 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.aiSynchronousBudgetHint') }}</p>
      </div>
      <div>
        <label for="vote-ai-moderation-fast-input" class="input-label">{{ t('admin.riskControl.aiFastInputChars') }}</label>
        <input
          id="vote-ai-moderation-fast-input"
          :value="fastInputChars"
          type="number"
          min="1"
          :max="maxInputChars"
          step="1000"
          class="input"
          data-test="ai-fast-input-chars"
          @input="updateFastInputChars"
        />
        <p class="mt-2 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.aiFastInputCharsHint') }}</p>
      </div>
      <div>
        <label for="vote-ai-moderation-fallback-input" class="input-label">{{ t('admin.riskControl.aiFallbackInputChars') }}</label>
        <input
          id="vote-ai-moderation-fallback-input"
          :value="fallbackInputChars"
          type="number"
          min="1"
          :max="fastInputChars"
          step="1000"
          class="input"
          data-test="ai-fallback-input-chars"
          @input="updateFallbackInputChars"
        />
        <p class="mt-2 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.aiFallbackInputCharsHint') }}</p>
      </div>
    </div>
  </fieldset>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

defineProps<{
  synchronousBudgetMs: number
  fastInputChars: number
  fallbackInputChars: number
  maxInputChars: number
}>()

const emit = defineEmits<{
  'update:synchronousBudgetMs': [value: number]
  'update:fastInputChars': [value: number]
  'update:fallbackInputChars': [value: number]
}>()

const { t } = useI18n()

function inputNumber(event: Event): number {
  return Number((event.target as HTMLInputElement).value)
}

function updateSynchronousBudget(event: Event) {
  emit('update:synchronousBudgetMs', inputNumber(event))
}

function updateFastInputChars(event: Event) {
  emit('update:fastInputChars', inputNumber(event))
}

function updateFallbackInputChars(event: Event) {
  emit('update:fallbackInputChars', inputNumber(event))
}
</script>
