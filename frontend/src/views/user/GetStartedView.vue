<template>
  <AppLayout>
    <div class="mx-auto max-w-5xl space-y-6" data-testid="get-started-page">
      <header class="space-y-2">
        <h1 class="text-2xl font-semibold tracking-tight text-gray-900 dark:text-white">{{ t('quickConnect.startPage.heading') }}</h1>
        <p class="max-w-3xl text-sm leading-6 text-gray-600 dark:text-gray-300">{{ t('quickConnect.startPage.description') }}</p>
      </header>

      <section class="space-y-3" :aria-label="t('quickConnect.startPage.purpose')">
        <h2 class="text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('quickConnect.startPage.purpose') }}</h2>
        <div class="grid gap-3 sm:grid-cols-2">
          <button v-for="purpose in purposes" :key="purpose.id" type="button" :data-testid="`scene-${purpose.id}`"
            :aria-pressed="scene === purpose.id" class="flex items-start gap-4 rounded-xl border p-5 text-left transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary-500"
            :class="scene === purpose.id ? 'border-primary-500 bg-primary-50 dark:border-primary-500 dark:bg-primary-950/30' : 'border-gray-200 bg-white hover:border-primary-300 dark:border-dark-700 dark:bg-dark-800 dark:hover:border-primary-600'"
            @click="selectScene(purpose.id)">
            <Icon :name="purpose.icon" size="lg" class="mt-0.5 shrink-0 text-primary-600 dark:text-primary-400" />
            <span class="min-w-0 space-y-1"><span class="block font-semibold text-gray-900 dark:text-white">{{ t(`quickConnect.startPage.${purpose.id === 'code' ? 'develop' : 'image'}`) }}</span>
              <span class="block text-sm leading-6 text-gray-600 dark:text-gray-300">{{ t(`quickConnect.startPage.${purpose.id === 'code' ? 'developDescription' : 'imageDescription'}`) }}</span></span>
            <Icon v-if="scene === purpose.id" name="checkCircle" size="sm" class="ml-auto shrink-0 text-primary-600 dark:text-primary-400" />
          </button>
        </div>
      </section>

      <p v-if="missingRequestedKey" class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800 dark:border-amber-800 dark:bg-amber-950/30 dark:text-amber-200" role="status">{{ t('quickConnect.startPage.keyMissing') }}</p>

      <div v-if="loading" class="card flex items-center justify-center gap-3 p-10" role="status"><LoadingSpinner /><span class="text-sm text-gray-600 dark:text-gray-300">{{ t('quickConnect.startPage.loading') }}</span></div>
      <div v-else-if="loadError" class="card space-y-4 p-6" role="alert">
        <p class="text-sm text-red-600 dark:text-red-400">{{ t('quickConnect.startPage.loadError') }}</p>
        <button type="button" class="btn btn-primary" data-testid="retry-keys" @click="loadKeys">{{ t('quickConnect.startPage.retry') }}</button>
      </div>
      <section v-else-if="!keys.length" class="card space-y-3 p-6" data-testid="no-keys">
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('quickConnect.startPage.noKeys') }}</h2>
        <p class="max-w-2xl text-sm leading-6 text-gray-600 dark:text-gray-300">{{ t('quickConnect.startPage.noKeysDescription') }}</p>
        <RouterLink to="/keys" class="btn btn-primary">{{ t('quickConnect.startPage.createKey') }}</RouterLink>
      </section>

      <section v-else-if="scene === 'code'" data-testid="developer-guide">
        <QuickConnectPanel :show="true" :api-key="initialKey?.key || ''" :key-name="initialKey?.name || ''" :base-url="baseUrl"
          :provider-name="appStore.siteName" :available-keys="keys" :initial-key-id="requestedKeyId" :platform="initialKey?.group?.platform"
          @manage-keys="router.push('/keys')" @retry="loadKeys" />
      </section>

      <section v-else class="card space-y-6 p-4 sm:p-6" data-testid="image-guide">
        <header><h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('quickConnect.startPage.imageHeading') }}</h2><p class="mt-1 text-sm leading-6 text-gray-600 dark:text-gray-300">{{ t('quickConnect.startPage.imageIntro') }}</p></header>
        <ol class="space-y-6">
          <li class="flex gap-3"><span class="step-number">1</span><div class="min-w-0 flex-1 space-y-3">
            <h3 class="step-heading">{{ t('quickConnect.startPage.imageStep1') }}</h3>
            <p v-if="selectedImageKeyId !== null && !selectedImageKey" class="text-sm text-amber-700 dark:text-amber-300" role="status" data-testid="image-requested-unavailable">{{ t('quickConnect.startPage.imageRequestedUnavailable') }}</p>
            <template v-if="imageKeys.length">
              <label class="block text-sm text-gray-700 dark:text-gray-200">{{ t('quickConnect.startPage.imageKeyLabel') }}
                <select v-model.number="selectedImageKeyId" class="input mt-2 w-full" data-testid="image-key-select"><option v-if="!selectedImageKey" :value="selectedImageKeyId" disabled>{{ t('quickConnect.startPage.chooseImageReplacement') }}</option><option v-for="key in imageKeys" :key="key.id" :value="key.id">{{ key.name }} · {{ key.group?.name }} · {{ key.key.slice(-4) }}</option></select>
              </label>
              <p class="text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('quickConnect.startPage.imageKeyHelp') }}</p>
            </template>
            <div v-else class="space-y-2" data-testid="no-image-keys"><p class="font-medium text-gray-800 dark:text-gray-200">{{ t('quickConnect.startPage.imageUnavailable') }}</p><p class="text-sm leading-6 text-gray-600 dark:text-gray-300">{{ t('quickConnect.startPage.imageUnavailableHelp') }}</p><RouterLink to="/keys" class="btn btn-secondary">{{ t('quickConnect.startPage.manageKeys') }}</RouterLink></div>
            <p class="text-sm leading-6 text-gray-600 dark:text-gray-300">{{ t('quickConnect.startPage.imageGroupHint') }}</p>
          </div></li>
          <li class="flex gap-3"><span class="step-number">2</span><div class="min-w-0 flex-1 space-y-3"><h3 class="step-heading">{{ t('quickConnect.startPage.imageStep2') }}</h3><p class="text-sm leading-6 text-gray-600 dark:text-gray-300">{{ t('quickConnect.startPage.imageStep2Body') }}</p>
            <p v-if="selectedImageKey" class="break-words rounded-lg bg-primary-50 p-3 text-sm text-primary-800 dark:bg-primary-950/30 dark:text-primary-200">{{ t('quickConnect.startPage.selectedImageKey', { name: selectedImageKey.name, group: selectedImageKey.group?.name, suffix: selectedImageKey.key.slice(-4) }) }}</p>
            <RouterLink v-if="imageWorkbench && selectedImageKey" :to="`/custom/${encodeURIComponent(imageWorkbench.id)}`" class="btn btn-primary" data-testid="enter-workbench"><Icon name="arrowRight" size="sm" />{{ t('quickConnect.startPage.enterWorkbench') }}</RouterLink>
            <p v-else-if="!imageWorkbench" class="text-sm text-gray-600 dark:text-gray-300" role="status">{{ t('quickConnect.startPage.workbenchUnavailable') }}</p>
          </div></li>
          <li class="flex gap-3"><span class="step-number">3</span><div class="min-w-0 flex-1 space-y-2"><h3 class="step-heading">{{ t('quickConnect.startPage.imageStep3') }}</h3><p class="text-sm leading-6 text-gray-600 dark:text-gray-300">{{ t('quickConnect.startPage.imageStep3Body') }}</p></div></li>
        </ol>
      </section>

      <details class="text-sm text-gray-600 dark:text-gray-300"><summary class="cursor-pointer font-medium">{{ t('quickConnect.startPage.helpTitle') }}</summary><p class="mt-2 max-w-3xl leading-6">{{ t('quickConnect.startPage.helpBody') }}</p></details>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import QuickConnectPanel from '@/components/keys/QuickConnectPanel.vue'
import { keysAPI } from '@/api/keys'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { supportsCanvasKind, unavailableReason } from '@/custom/vote-ai/canvas-key-bridge'
import { findImageWorkbench, parseConnectKeyId, readAllConnectKeys } from '@/custom/vote-ai/quick-connect/page-helpers'
import type { ApiKey } from '@/types'

const { t } = useI18n()
const route = useRoute(), router = useRouter()
const appStore = useAppStore(), authStore = useAuthStore()
const purposes = [{ id: 'code', icon: 'terminal' }, { id: 'image', icon: 'sparkles' }] as const
const scene = ref<'code' | 'image'>(route.query.scene === 'image' ? 'image' : 'code')
const keys = ref<ApiKey[]>([]), loading = ref(true), loadError = ref(false)
const selectedImageKeyId = ref<number | null>(null)
const requestedKeyId = computed(() => parseConnectKeyId(route.query.key))
const initialKey = computed(() => keys.value.find(key => key.id === requestedKeyId.value))
const missingRequestedKey = computed(() => !loading.value && !loadError.value && requestedKeyId.value !== null && !initialKey.value)
const baseUrl = computed(() => appStore.cachedPublicSettings?.api_base_url || window.location.origin)
const imageKeys = computed(() => keys.value.filter(key => key.key?.trim() && supportsCanvasKind(key, 'image') && !unavailableReason(key)))
const selectedImageKey = computed(() => imageKeys.value.find(key => key.id === selectedImageKeyId.value))
const imageWorkbench = computed(() => findImageWorkbench(appStore.cachedPublicSettings?.custom_menu_items || [], window.location.origin))
let controller: AbortController | undefined
let requestVersion = 0

async function loadKeys() {
  controller?.abort()
  const current = new AbortController()
  controller = current
  const version = ++requestVersion
  keys.value = []
  loading.value = true
  loadError.value = false
  try {
    const result = await readAllConnectKeys((page, signal) => keysAPI.list(page, 100, undefined, { signal }), current.signal)
    if (version === requestVersion) keys.value = result
  } catch {
    if (version === requestVersion && !current.signal.aborted) loadError.value = true
  } finally {
    if (version === requestVersion) loading.value = false
  }
}
function selectScene(next: 'code' | 'image') {
  scene.value = next
  void router.replace({ query: { ...route.query, scene: next } })
}
watch(() => route.query.scene, value => { scene.value = value === 'image' ? 'image' : 'code' })
watch([imageKeys, requestedKeyId], () => {
  // Preserve explicit intent even if the key becomes unavailable. A different
  // image group may have different output behavior and pricing.
  if (requestedKeyId.value !== null) {
    selectedImageKeyId.value = requestedKeyId.value
    return
  }
  if (selectedImageKeyId.value === null) selectedImageKeyId.value = imageKeys.value[0]?.id ?? null
}, { immediate: true })
watch(() => authStore.token, () => {
  controller?.abort(); ++requestVersion; keys.value = []
  if (authStore.token) void loadKeys()
})
onMounted(() => { void appStore.fetchPublicSettings(); void loadKeys() })
onBeforeUnmount(() => { ++requestVersion; controller?.abort(); keys.value = [] })
</script>

<style scoped>
.step-number { @apply flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-primary-50 text-sm font-medium text-primary-700 dark:bg-primary-950/40 dark:text-primary-300; }
.step-heading { @apply pt-0.5 text-sm font-semibold text-gray-900 dark:text-white; }
</style>
