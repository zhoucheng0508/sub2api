<template>
  <div>
    <div class="mb-3 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
      <div class="min-w-0">
        <label for="vote-ai-moderation-system-prompt" class="input-label mb-0">{{ t('admin.riskControl.aiSystemPrompt') }}</label>
        <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">
          {{ t('admin.riskControl.aiSystemPromptHint') }}
        </p>
        <div class="mt-2 flex flex-wrap gap-2 text-xs">
          <span class="inline-flex rounded-md bg-gray-100 px-2 py-1 text-gray-700 dark:bg-dark-700 dark:text-gray-200" data-test="active-prompt-version">
            {{ t('admin.riskControl.aiPromptCurrentVersion', { version: currentVersionLabel }) }}
          </span>
          <span class="inline-flex rounded-md bg-emerald-50 px-2 py-1 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300" data-test="recommended-prompt-version">
            {{ t('admin.riskControl.aiPromptRecommendedVersion', { version: recommendedVersionLabel }) }}
          </span>
        </div>
      </div>
      <button
        type="button"
        class="btn btn-secondary shrink-0"
        :disabled="!canApplyRecommended"
        data-test="apply-recommended-prompt"
        @click="applyRecommended"
      >
        {{ promptIsRecommended ? t('admin.riskControl.aiPromptRecommendedActive') : t('admin.riskControl.aiApplyRecommendedPrompt') }}
      </button>
    </div>
    <textarea
      id="vote-ai-moderation-system-prompt"
      :value="modelValue"
      class="input min-h-48 resize-y font-mono text-xs leading-5"
      data-test="ai-system-prompt"
      @input="updatePrompt"
    ></textarea>
    <p v-if="!promptIsRecommended" class="mt-2 text-xs leading-5 text-amber-600 dark:text-amber-300" data-test="custom-prompt-notice">
      {{ t('admin.riskControl.aiPromptCustomNotice') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

const props = defineProps<{
  modelValue: string
  recommendedSystemPrompt: string
  recommendedPromptVersion: string
  systemPromptVersion: string
  usesRecommendedSystemPrompt: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
  'apply-recommended': []
}>()

const { t } = useI18n()

const promptIsRecommended = computed(() => {
  const recommended = props.recommendedSystemPrompt.trim()
  return recommended !== '' && props.modelValue.trim() === recommended
})

const currentVersionLabel = computed(() => {
  if (promptIsRecommended.value) {
    return props.recommendedPromptVersion || props.systemPromptVersion || t('admin.riskControl.aiPromptVersionUnknown')
  }
  if (!props.usesRecommendedSystemPrompt && props.systemPromptVersion) return props.systemPromptVersion
  return t('admin.riskControl.aiPromptCustomVersion')
})

const recommendedVersionLabel = computed(() => (
  props.recommendedPromptVersion || t('admin.riskControl.aiPromptVersionUnknown')
))

const canApplyRecommended = computed(() => (
  props.recommendedSystemPrompt.trim() !== '' && !promptIsRecommended.value
))

function updatePrompt(event: Event) {
  emit('update:modelValue', (event.target as HTMLTextAreaElement).value)
}

function applyRecommended() {
  if (!canApplyRecommended.value) return
  emit('apply-recommended')
}
</script>
