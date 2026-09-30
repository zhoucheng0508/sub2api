<template>
  <section class="space-y-4 rounded-xl border border-primary-200 bg-white p-4 dark:border-primary-800 dark:bg-dark-800" data-testid="create-connect-key">
    <div>
      <p class="mt-1 text-sm leading-6 text-gray-600 dark:text-gray-300">{{ t('quickCreateKey.explanation') }}</p>
      <p class="mt-2 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('quickCreateKey.noPayment') }}</p>
    </div>

    <p v-if="loading" role="status" class="text-sm text-gray-500">{{ t('quickCreateKey.loading') }}</p>
    <div v-else-if="loadError" role="alert" class="space-y-3 text-sm text-red-700 dark:text-red-300">
      <p>{{ t('quickCreateKey.loadError') }}</p>
      <button type="button" class="btn btn-secondary btn-sm" data-testid="create-key-reload" @click="loadGroups">{{ t('quickCreateKey.retry') }}</button>
    </div>
    <div v-else-if="!availableGroups.length" class="space-y-2 text-sm" data-testid="create-key-no-groups" role="status">
      <p class="font-medium text-gray-900 dark:text-white">{{ t(scene === 'image' ? 'quickCreateKey.noImageGroups' : 'quickCreateKey.noTextGroups') }}</p>
      <p class="leading-6 text-gray-500 dark:text-gray-400">{{ t('quickCreateKey.noGroupsHelp') }}</p>
      <slot name="empty-actions">
        <RouterLink to="/dashboard" class="inline-flex text-primary-600 hover:underline dark:text-primary-400">{{ t('quickCreateKey.account') }}</RouterLink>
      </slot>
    </div>
    <form v-else class="space-y-4" data-testid="create-key-form" @submit.prevent="createKey">
      <fieldset :disabled="creating || completed" class="space-y-3">
        <legend class="text-sm font-medium text-gray-900 dark:text-white">{{ t('quickCreateKey.group') }}</legend>
        <p class="text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t(availableGroups.length === 1 ? 'quickCreateKey.onlyGroup' : 'quickCreateKey.groupHelp') }}</p>
        <label
          v-for="group in availableGroups"
          :key="group.id"
          class="flex cursor-pointer items-start gap-3 rounded-lg border p-3 transition-colors"
          :class="selectedGroupId === group.id ? 'border-primary-500 bg-primary-50/50 dark:bg-primary-900/10' : 'border-gray-200 dark:border-dark-600'"
          :data-testid="`create-key-group-${group.id}`"
        >
          <input v-model="selectedGroupId" type="radio" name="connect-key-group" :value="group.id" class="mt-1 shrink-0 accent-primary-600" />
          <span class="min-w-0 flex-1 space-y-2">
            <GroupOptionItem
              :name="group.name"
              :description="group.description"
              :platform="group.platform"
              :subscription-type="group.subscription_type"
              :rate-multiplier="scene === 'text' ? group.rate_multiplier : undefined"
              :user-rate-multiplier="scene === 'text' ? userGroupRates[group.id] : null"
              :peak-rate-enabled="scene === 'text' && group.peak_rate_enabled"
              :peak-start="group.peak_start"
              :peak-end="group.peak_end"
              :peak-rate-multiplier="group.peak_rate_multiplier"
              :show-checkmark="false"
            />
            <span class="block text-xs leading-5 text-gray-600 dark:text-gray-300">{{ t(group.subscription_type === 'subscription' ? 'quickCreateKey.subscription' : 'quickCreateKey.standard') }}</span>
            <span v-if="scene === 'text' && app" class="block text-xs leading-5 text-primary-700 dark:text-primary-300">{{ compatibilityLabel(group) }}</span>
            <template v-if="scene === 'image'">
              <span class="block text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('quickCreateKey.imagePricing') }}</span>
              <span v-for="price in imagePrices(group)" :key="price.size" class="block text-xs text-gray-500 dark:text-gray-400">{{ t('quickCreateKey.imagePrice', price) }}</span>
            </template>
          </span>
        </label>
        <label class="block text-sm text-gray-900 dark:text-white">
          <span>{{ t('quickCreateKey.name') }}</span>
          <input v-model="name" type="text" maxlength="100" autocomplete="off" :placeholder="defaultName" class="input mt-2 w-full" data-testid="create-key-name" />
          <span class="mt-1 block text-xs text-gray-500 dark:text-gray-400">{{ t('quickCreateKey.nameHelp') }}</span>
        </label>
      </fieldset>
      <div v-if="createError" role="alert" class="space-y-2 text-sm text-red-700 dark:text-red-300" data-testid="create-key-error">
        <p>{{ t(createError) }}</p>
        <button v-if="createError === 'quickCreateKey.groupDenied'" type="button" class="btn btn-secondary btn-sm" @click="loadGroups">{{ t('quickCreateKey.retry') }}</button>
        <button v-else type="button" class="btn btn-secondary btn-sm" data-testid="create-key-check-existing" @click="emit('refresh-keys')">{{ t('quickCreateKey.checkExisting') }}</button>
      </div>
      <p v-if="completed" role="status" class="text-sm text-green-700 dark:text-green-300">{{ t('quickCreateKey.created') }}</p>
      <button type="submit" class="btn btn-primary w-full sm:w-auto" :disabled="!selectedGroup || creating || completed" data-testid="create-key-submit">
        {{ t(creating ? 'quickCreateKey.creating' : 'quickCreateKey.create') }}
      </button>
    </form>
    <button v-if="showCancel" type="button" class="text-sm text-gray-500 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white" :disabled="creating" data-testid="create-key-cancel" @click="emit('cancel')">{{ t('quickCreateKey.cancel') }}</button>
  </section>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import { keysAPI, userGroupsAPI } from '@/api'
import GroupOptionItem from '@/components/common/GroupOptionItem.vue'
import { useAuthStore } from '@/stores/auth'
import type { ApiKey, Group } from '@/types'
import { CC_SWITCH_APP_CATALOG, type CcSwitchAppType } from '@/utils/ccswitchImport'
import { isCnOaiGroup } from '@/utils/cnOaiSetup'
import { extractApiErrorCode } from '@/utils/apiError'
import { supportsAutomaticConfig } from './auto-config'

const props = withDefaults(defineProps<{ scene: 'text' | 'image'; app?: CcSwitchAppType; showCancel?: boolean }>(), { showCancel: false })
const emit = defineEmits<{ created: [key: ApiKey]; cancel: []; 'refresh-keys': []; busy: [value: boolean] }>()
const { t } = useI18n()
const auth = useAuthStore()
const groups = ref<Group[]>([])
const userGroupRates = ref<Record<number, number>>({})
const selectedGroupId = ref<number | null>(null)
const name = ref('')
const loading = ref(true)
const loadError = ref(false)
const creating = ref(false)
watch(creating, value => emit('busy', value), { flush: 'sync' })
const completed = ref(false)
const createError = ref('')
let controller: AbortController | null = null
let readSequence = 0
let identitySequence = 0
let active = true

// Availability and subscription permission come from the same user API as KeysView.
// Capability flags, rather than group names, determine the creation scene.
const availableGroups = computed(() => groups.value.filter(group =>
  (!group.status || group.status === 'active') && (props.scene === 'image'
    ? group.platform === 'openai' && group.image_only === true && group.allow_image_generation === true
    : !group.image_only),
))
const selectedGroup = computed(() => availableGroups.value.find(group => group.id === selectedGroupId.value))
const appLabel = computed(() => CC_SWITCH_APP_CATALOG.find(app => app.id === props.app)?.label || t('quickCreateKey.defaultApp'))
const defaultName = computed(() => props.scene === 'image' ? t('quickCreateKey.defaultImageName') : t('quickCreateKey.defaultTextName', { app: appLabel.value }))

// Other groups stay selectable for their existing manual guides; never switch
// billing groups to match an app.
function compatibilityLabel(group: Group): string {
  if (props.app === 'codex' && isCnOaiGroup(group) && supportsAutomaticConfig(group, props.app)) return t('quickCreateKey.cnOai')
  const direct = props.app ? supportsAutomaticConfig(group, props.app) : false
  return t(direct ? 'quickCreateKey.direct' : 'quickCreateKey.manual', { app: appLabel.value })
}

function imagePrices(group: Group): Array<{ size: string; price: number }> {
  return [
    { size: '1K', price: group.image_price_1k },
    { size: '2K', price: group.image_price_2k },
    { size: '4K', price: group.image_price_4k },
  ].filter((entry): entry is { size: string; price: number } => typeof entry.price === 'number' && Number.isFinite(entry.price) && entry.price >= 0)
}

watch(availableGroups, available => {
  if (available.some(group => group.id === selectedGroupId.value)) return
  selectedGroupId.value = available.length === 1 ? available[0].id : null
})

async function loadGroups(): Promise<void> {
  controller?.abort()
  const request = ++readSequence
  const identity = identitySequence
  const userId = auth.user?.id
  const requestController = new AbortController()
  controller = requestController
  groups.value = []
  userGroupRates.value = {}
  selectedGroupId.value = null
  loadError.value = false
  createError.value = ''
  loading.value = true
  if (userId == null) {
    loading.value = false
    return
  }
  try {
    const [available, rates] = await Promise.all([
      userGroupsAPI.getAvailable({ signal: requestController.signal }),
      userGroupsAPI.getUserGroupRates({ signal: requestController.signal }),
    ])
    if (!active || requestController.signal.aborted || request !== readSequence || identity !== identitySequence || userId !== auth.user?.id) return
    groups.value = available
    userGroupRates.value = rates
  } catch {
    if (active && !requestController.signal.aborted && request === readSequence && identity === identitySequence) loadError.value = true
  } finally {
    if (active && request === readSequence && identity === identitySequence) {
      loading.value = false
      controller = null
    }
  }
}

async function createKey(): Promise<void> {
  const group = selectedGroup.value
  const userId = auth.user?.id
  if (!active || userId == null || !group || loading.value || loadError.value || creating.value || completed.value) return
  const identity = identitySequence
  creating.value = true
  createError.value = ''
  try {
    // This mutation only runs on an explicit submit; no automatic retries.
    const created = await keysAPI.create(name.value.trim() || defaultName.value, group.id)
    if (!active || identity !== identitySequence || userId !== auth.user?.id) return
    completed.value = true
    emit('created', { ...created, group: created.group ?? (created.group_id === group.id ? group : undefined) })
  } catch (error) {
    if (!active || identity !== identitySequence || userId !== auth.user?.id) return
    // Do not render raw backend errors, which can carry credentials or internals.
    createError.value = extractApiErrorCode(error) === 'GROUP_NOT_ALLOWED' ? 'quickCreateKey.groupDenied' : 'quickCreateKey.createError'
  } finally {
    if (active && identity === identitySequence && userId === auth.user?.id) creating.value = false
  }
}

watch(() => auth.user?.id, () => {
  identitySequence++
  name.value = ''
  creating.value = false
  completed.value = false
  void loadGroups()
}, { immediate: true, flush: 'sync' })

onUnmounted(() => {
  active = false
  identitySequence++
  controller?.abort()
  emit('busy', false)
})
</script>
