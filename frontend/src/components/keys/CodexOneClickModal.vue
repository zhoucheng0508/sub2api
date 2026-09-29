<template>
  <BaseDialog :show="show" :title="t('quickConnect.title')" width="extra-wide" @close="emit('close')">
    <QuickConnectPanel v-bind="props" @manage-keys="emit('manage-keys')" @protocol-failed="emit('protocol-failed')" @retry="emit('retry')" />
    <template #footer>
      <button type="button" class="btn btn-secondary" @click="emit('close')">{{ t('keys.oneClick.later') }}</button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import QuickConnectPanel from './QuickConnectPanel.vue'
import type { ApiKey, GroupPlatform } from '@/types'
import type { CcSwitchAppType } from '@/utils/ccswitchImport'

const props = defineProps<{
  show: boolean
  apiKey: string
  keyName: string
  baseUrl: string
  providerName: string
  platform?: GroupPlatform | null
  defaultApp?: CcSwitchAppType
  initialMethod?: 'guide' | 'ccswitch' | 'script' | 'cn-oai'
  availableKeys?: ApiKey[]
  initialKeyId?: number | null
  loading?: boolean
  loadError?: boolean
}>()
const emit = defineEmits<{
  (event: 'close'): void
  (event: 'protocol-failed'): void
  (event: 'manage-keys'): void
  (event: 'retry'): void
}>()
const { t } = useI18n()
</script>
