<template>
  <AppLayout>
    <div class="mx-auto max-w-5xl space-y-6" data-testid="get-started-page">
      <header class="space-y-2">
        <h1 class="text-2xl font-semibold tracking-tight text-gray-900 dark:text-white">{{ t('quickConnect.startPage.heading') }}</h1>
        <p class="max-w-3xl text-sm leading-6 text-gray-600 dark:text-gray-300">{{ t(authStore.isSimpleMode ? 'firstUseJourney.simpleReadyBody' : 'quickConnect.startPage.description') }}</p>
      </header>
      <FirstUseJourney v-if="!authStore.isAdmin" :phase="fundingPhase" :balance="availableBalance" :loading="fundingLoading"
        :simple-mode="authStore.isSimpleMode"
        :online-available="onlineAvailable" :can-redeem="canRedeem" :subscription-only="subscriptionOnly"
        :key-ready="keyReady" :request-count="journeyRequestCount" :scene="scene" :busy="createPending" :contact-info="appStore.cachedPublicSettings?.contact_info"
        @refresh="refreshJourney" @continue="focusSetup" />

      <section id="connection-setup" class="space-y-3 scroll-mt-6" :aria-label="t('quickConnect.startPage.purpose')">
        <h2 class="text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('quickConnect.startPage.purpose') }}</h2>
        <div class="grid gap-3 sm:grid-cols-2">
          <button v-for="purpose in purposes" :key="purpose.id" type="button" :data-testid="`scene-${purpose.id}`"
            :disabled="createPending"
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
      <div v-if="createdKeyNotice" class="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-primary-200 bg-primary-50 p-4 text-sm text-primary-800 dark:border-primary-800 dark:bg-primary-950/30 dark:text-primary-200" role="status" data-testid="created-in-place">
        <p>{{ t(scene === 'image' ? 'quickConnect.startPage.newImageKeyReady' : 'quickConnect.startPage.newKeyReady') }}</p>
        <button type="button" class="btn btn-secondary btn-sm" @click="focusConnection">{{ t('quickConnect.startPage.continueSetup') }}</button>
      </div>

      <div v-if="loading" class="card flex items-center justify-center gap-3 p-10" role="status"><LoadingSpinner /><span class="text-sm text-gray-600 dark:text-gray-300">{{ t('quickConnect.startPage.loading') }}</span></div>
      <div v-else-if="loadError" class="card space-y-4 p-6" role="alert">
        <p class="text-sm text-red-600 dark:text-red-400">{{ t('quickConnect.startPage.loadError') }}</p>
        <button type="button" class="btn btn-primary" data-testid="retry-keys" @click="loadKeys">{{ t('quickConnect.startPage.retry') }}</button>
      </div>
      <section v-else-if="scene === 'code'" data-testid="developer-guide">
        <QuickConnectPanel :show="true" :api-key="initialKey?.key || ''" :key-name="initialKey?.name || ''" :base-url="baseUrl"
          :provider-name="appStore.siteName" :available-keys="keys" :initial-key-id="requestedKeyId" :platform="initialKey?.group?.platform" :creation-busy="createPending"
          @manage-keys="router.push('/keys')" @retry="loadKeys" @key-ready="textKeyReady = $event">
          <template #create-key="{ app }"><CreateConnectKey scene="text" :app="app" @created="handleCreatedKey" @refresh-keys="loadKeys" @busy="createPending = $event" /></template>
        </QuickConnectPanel>
      </section>

      <section v-else class="card space-y-6 p-4 sm:p-6" data-testid="image-guide">
        <header><h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('quickConnect.startPage.imageHeading') }}</h2><p class="mt-1 text-sm leading-6 text-gray-600 dark:text-gray-300">{{ t('quickConnect.startPage.imageIntro') }}</p></header>
        <ol class="space-y-6">
          <li class="flex gap-3"><span class="step-number">1</span><div class="min-w-0 flex-1 space-y-3">
            <h3 class="step-heading">{{ t('quickConnect.startPage.imageStep1') }}</h3>
            <p v-if="selectedImageKeyId !== null && !selectedImageKey" class="text-sm text-amber-700 dark:text-amber-300" role="status" data-testid="image-requested-unavailable">{{ t('quickConnect.startPage.imageRequestedUnavailable') }}</p>
            <template v-if="imageKeys.length && !createImageKeyOpen">
              <label class="block text-sm text-gray-700 dark:text-gray-200">{{ t('quickConnect.startPage.imageKeyLabel') }}
                <select v-model.number="selectedImageKeyId" class="input mt-2 w-full" data-testid="image-key-select"><option v-if="!selectedImageKey" :value="selectedImageKeyId" disabled>{{ t('quickConnect.startPage.chooseImageReplacement') }}</option><option v-for="key in imageKeys" :key="key.id" :value="key.id">{{ key.name }} · {{ key.group?.name }} · {{ key.key.slice(-4) }}</option></select>
              </label>
              <p class="text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('quickConnect.startPage.imageKeyHelp') }}</p>
              <button type="button" class="btn btn-secondary" data-testid="create-image-key" @click="createImageKeyOpen = true">{{ t('quickConnect.startPage.createAnother') }}</button>
            </template>
            <div v-else class="space-y-3" data-testid="inline-image-create">
              <p class="text-sm leading-6 text-gray-600 dark:text-gray-300">{{ t('quickConnect.startPage.keyMeaning') }}</p>
              <CreateConnectKey scene="image" @created="handleCreatedKey" @refresh-keys="loadKeys" @busy="createPending = $event" />
              <button v-if="imageKeys.length" type="button" :disabled="createPending" class="text-sm font-medium text-primary-600 hover:underline disabled:opacity-50 dark:text-primary-400" @click="createImageKeyOpen = false">{{ t('quickConnect.startPage.useExisting') }}</button>
            </div>
            <p class="text-sm leading-6 text-gray-600 dark:text-gray-300">{{ t('quickConnect.startPage.imageGroupHint') }}</p>
          </div></li>
          <li class="flex gap-3"><span class="step-number">2</span><div class="min-w-0 flex-1 space-y-3"><h3 id="image-connect-title" class="step-heading">{{ t('quickConnect.startPage.imageStep2') }}</h3><p class="text-sm leading-6 text-gray-600 dark:text-gray-300">{{ t('quickConnect.startPage.imageStep2Body') }}</p>
            <p v-if="selectedImageKey" class="break-words rounded-lg bg-primary-50 p-3 text-sm text-primary-800 dark:bg-primary-950/30 dark:text-primary-200">{{ t('quickConnect.startPage.selectedImageKey', { name: selectedImageKey.name, group: selectedImageKey.group?.name, suffix: selectedImageKey.key.slice(-4) }) }}</p>
            <RouterLink v-if="imageWorkbench && selectedImageKey && !createImageKeyOpen" :to="`/custom/${encodeURIComponent(imageWorkbench.id)}`" class="btn btn-primary" data-testid="enter-workbench"><Icon name="arrowRight" size="sm" />{{ t('quickConnect.startPage.enterWorkbench') }}</RouterLink>
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
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import QuickConnectPanel from '@/components/keys/QuickConnectPanel.vue'
import CreateConnectKey from '@/custom/vote-ai/quick-connect/CreateConnectKey.vue'
import FirstUseJourney from '@/custom/vote-ai/quick-connect/FirstUseJourney.vue'
import { useFundingReadiness } from '@/custom/vote-ai/quick-connect/useFundingReadiness'
import { usageAPI } from '@/api/usage'
import { keysAPI } from '@/api/keys'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { supportsCanvasKind, unavailableReason } from '@/custom/vote-ai/canvas-key-bridge'
import { findImageWorkbench, parseConnectKeyId, readAllConnectKeys } from '@/custom/vote-ai/quick-connect/page-helpers'
import type { ApiKey } from '@/types'

const { t } = useI18n()
const route = useRoute(), router = useRouter()
const appStore = useAppStore(), authStore = useAuthStore()
const { phase: fundingPhase, balance: availableBalance, loading: fundingLoading, onlineAvailable, canRedeem, subscriptionOnly, refresh: refreshFunding } = useFundingReadiness()
const journeyRequestCount = ref<number | null>(null)
let journeyRequestVersion = 0
let disposed = false
const purposes = [{ id: 'code', icon: 'terminal' }, { id: 'image', icon: 'sparkles' }] as const
const scene = ref<'code' | 'image'>(route.query.scene === 'image' ? 'image' : 'code')
const keys = ref<ApiKey[]>([]), loading = ref(true), loadError = ref(false)
const selectedImageKeyId = ref<number | null>(null)
const justCreatedKeyId = ref<number | null>(null)
const createImageKeyOpen = ref(false)
const createPending = ref(false)
const createdKeyNotice = ref(false)
const textKeyReady = ref(false)
const requestedKeyId = computed(() => justCreatedKeyId.value ?? parseConnectKeyId(route.query.key))
const initialKey = computed(() => keys.value.find(key => key.id === requestedKeyId.value))
const missingRequestedKey = computed(() => !loading.value && !loadError.value && requestedKeyId.value !== null && !initialKey.value)
const baseUrl = computed(() => appStore.cachedPublicSettings?.api_base_url || window.location.origin)
const imageKeys = computed(() => keys.value.filter(key => key.key?.trim() && supportsCanvasKind(key, 'image') && !unavailableReason(key)))
const selectedImageKey = computed(() => imageKeys.value.find(key => key.id === selectedImageKeyId.value))
const keyReady = computed(() => !loading.value && !loadError.value && (scene.value === 'image' ? !!selectedImageKey.value && !createImageKeyOpen.value : textKeyReady.value))
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
  if (createPending.value) return
  createdKeyNotice.value = false
  scene.value = next
  void router.replace({ query: { ...route.query, scene: next } })
}
async function focusConnection() {
  await nextTick()
  document.getElementById(scene.value === 'image' ? 'image-connect-title' : 'quick-connect-install-title')?.scrollIntoView?.({ behavior: 'auto', block: 'start' })
}
async function focusSetup() {
  if (keyReady.value) return focusConnection()
  await nextTick()
  document.getElementById('connection-setup')?.scrollIntoView?.({ behavior: 'auto', block: 'start' })
}
async function loadJourneyUsage() {
  const version = ++journeyRequestVersion
  const userId = authStore.user?.id
  journeyRequestCount.value = null
  if (!authStore.token || !userId || authStore.isAdmin) return
  try {
    const stats = await usageAPI.getDashboardStats()
    if (!disposed && version === journeyRequestVersion && authStore.user?.id === userId && authStore.token) journeyRequestCount.value = stats.total_requests
  } catch { /* Unread usage is unknown; clicking an import button is never treated as completion. */ }
}
function refreshJourney() { void refreshFunding(); void loadJourneyUsage() }
async function handleCreatedKey(key: ApiKey) {
  // The creation component returns a user-owned key only after an explicit submit.
  // Keep the key in memory; routes carry IDs only.
  keys.value = [key, ...keys.value.filter(item => item.id !== key.id)]
  justCreatedKeyId.value = key.id
  selectedImageKeyId.value = key.id
  createImageKeyOpen.value = false
  createdKeyNotice.value = true
  await router.replace({ query: { ...route.query, key: String(key.id), scene: scene.value } })
  void focusConnection()
}
watch(() => route.query.key, value => {
  if (parseConnectKeyId(value) !== justCreatedKeyId.value) justCreatedKeyId.value = null
})
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
watch([() => authStore.user?.id, () => Boolean(authStore.token)], () => {
  controller?.abort(); ++requestVersion; keys.value = []
  justCreatedKeyId.value = null; selectedImageKeyId.value = null; createdKeyNotice.value = false; createImageKeyOpen.value = false; createPending.value = false
  journeyRequestCount.value = null; ++journeyRequestVersion
  if (authStore.token) { void loadKeys(); void loadJourneyUsage() }
})
onMounted(() => { void appStore.fetchPublicSettings(); void loadKeys(); void loadJourneyUsage() })
onBeforeUnmount(() => { disposed = true; ++journeyRequestVersion; ++requestVersion; controller?.abort(); keys.value = [] })
</script>

<style scoped>
.step-number { @apply flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-primary-50 text-sm font-medium text-primary-700 dark:bg-primary-950/40 dark:text-primary-300; }
.step-heading { @apply pt-0.5 text-sm font-semibold text-gray-900 dark:text-white; }
</style>
