<template>
  <div class="space-y-3">
    <div>
      <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.auditProvider') }}</p>
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.auditProviderHint') }}</p>
    </div>
    <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
      <button
        v-for="option in options"
        :key="option.value"
        type="button"
        class="flex min-h-24 items-start gap-3 rounded-lg border p-4 text-left transition-colors"
        :class="modelValue === option.value
          ? 'border-primary-400 bg-primary-50 text-primary-900 shadow-sm dark:border-primary-500 dark:bg-primary-900/20 dark:text-primary-100'
          : 'border-gray-200 bg-white text-gray-900 hover:border-gray-300 hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-800 dark:text-white dark:hover:border-dark-600 dark:hover:bg-dark-700'"
        @click="$emit('update:modelValue', option.value)"
      >
        <span
          class="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg"
          :class="modelValue === option.value
            ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/40 dark:text-primary-300'
            : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-300'"
        >
          <Icon :name="option.icon" size="sm" />
        </span>
        <span class="min-w-0">
          <span class="block text-sm font-semibold">{{ option.label }}</span>
          <span class="mt-1 block text-xs leading-5 text-gray-500 dark:text-gray-400">{{ option.description }}</span>
        </span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { ContentModerationAuditProvider } from '@/api/admin/riskControl'

defineProps<{ modelValue: ContentModerationAuditProvider }>()
defineEmits<{ 'update:modelValue': [value: ContentModerationAuditProvider] }>()

const { t } = useI18n()

const options = computed(() => [
  {
    value: 'openai_moderations' as const,
    icon: 'shield' as const,
    label: t('admin.riskControl.providerOpenAI'),
    description: t('admin.riskControl.providerOpenAIDesc'),
  },
  {
    value: 'ai_chat' as const,
    icon: 'sparkles' as const,
    label: t('admin.riskControl.providerAIChat'),
    description: t('admin.riskControl.providerAIChatDesc'),
  },
])
</script>
