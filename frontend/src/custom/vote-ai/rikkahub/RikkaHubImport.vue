<template>
  <section class="space-y-4" data-testid="rikkahub-import">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <h4 class="font-semibold text-gray-900 dark:text-white">RikkaHub · Android</h4>
      <a class="text-sm text-primary-600 underline dark:text-primary-400" href="https://github.com/rikkahub/rikkahub/releases/tag/2.5.5" target="_blank" rel="noopener noreferrer">{{ t('rikkaHub.downloadApp') }}</a>
    </div>
    <p class="text-sm text-gray-600 dark:text-gray-300">{{ t('rikkaHub.intro') }}</p>
    <p class="break-all rounded-lg bg-gray-50 p-3 font-mono text-xs dark:bg-dark-800" data-testid="rikka-endpoint">{{ endpoint }}</p>
    <p v-if="loading" role="status" class="text-sm text-gray-500">{{ t('rikkaHub.loading') }}</p>
    <div v-else-if="loadError" role="alert" class="space-y-2 text-sm text-red-600 dark:text-red-400">
      <p>{{ t('rikkaHub.loadError') }}</p>
      <button type="button" class="btn btn-secondary" @click="loadModels">{{ t('rikkaHub.retry') }}</button>
    </div>
    <p v-else-if="!models.length" class="text-sm text-gray-500">{{ t('rikkaHub.empty') }}</p>
    <template v-else>
      <label class="block text-sm font-medium text-gray-700 dark:text-gray-200">
        {{ t('rikkaHub.model') }}
        <select v-model="selectedId" class="input mt-2 w-full" data-testid="rikka-model">
          <option v-for="model in models" :key="model.id" :value="model.id">{{ model.id }}</option>
        </select>
      </label>
      <p class="text-xs text-gray-500 dark:text-gray-400">{{ selectedModel?.vision ? t('rikkaHub.vision') : t('rikkaHub.textOnly') }}<span v-if="selectedModel?.effort"> · {{ t('rikkaHub.effort', { value: selectedModel.effort }) }}</span></p>
      <p class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800 dark:border-amber-800 dark:bg-amber-950/30 dark:text-amber-200">{{ t('rikkaHub.private') }}</p>
      <button v-if="!qrUrl" type="button" class="btn btn-primary" :disabled="generating" data-testid="rikka-generate" @click="generateQr">{{ generating ? t('rikkaHub.generating') : t('rikkaHub.generate') }}</button>
      <p v-if="qrError" role="alert" class="text-sm text-red-600">{{ t('rikkaHub.qrError') }}</p>
      <div v-if="qrUrl" class="space-y-3">
        <img :src="qrUrl" :alt="t('rikkaHub.qrAlt')" class="mx-auto h-auto w-full max-w-sm rounded-lg bg-white" data-testid="rikka-qr" />
        <div class="flex flex-wrap justify-center gap-2">
          <a :href="qrUrl" download="rikkahub-personal-responses.png" class="btn btn-primary">{{ t('rikkaHub.save') }}</a>
          <button type="button" class="btn btn-secondary" @click="clearQr">{{ t('rikkaHub.hide') }}</button>
        </div>
      </div>
      <ol class="list-inside list-decimal space-y-2 text-sm text-gray-600 dark:text-gray-300">
        <li>{{ t('rikkaHub.step1') }}</li>
        <li>{{ t('rikkaHub.step2') }}</li>
        <li>{{ t('rikkaHub.step3') }}</li>
      </ol>
      <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('rikkaHub.snapshot') }}</p>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import QRCode from 'qrcode'
import { fetchCodexModelsManifest } from '@/api/codex'
import { buildRikkaPayload, normalizeRikkaBaseUrl, parseRikkaModels, type RikkaModel } from './config'

const props = defineProps<{ apiKey: string; baseUrl: string }>()
const { t } = useI18n()

const models = ref<RikkaModel[]>([])
const selectedId = ref('')
const selectedModel = computed(() => models.value.find(model => model.id === selectedId.value))
const loading = ref(false)
const loadError = ref(false)
const generating = ref(false)
const qrError = ref(false)
const qrUrl = ref('')
let controller: AbortController | undefined
let loadVersion = 0
let qrVersion = 0
const endpoint = computed(() => {
  try { return `${normalizeRikkaBaseUrl(props.baseUrl)}/responses` } catch { return '' }
})

function clearQr() {
  qrVersion++
  qrUrl.value = ''
  qrError.value = false
  generating.value = false
}

async function loadModels() {
  controller?.abort()
  controller = new AbortController()
  const signal = controller.signal
  const version = ++loadVersion
  const timeout = setTimeout(() => controller?.signal === signal && controller.abort(), 15000)
  clearQr()
  models.value = []
  selectedId.value = ''
  loading.value = true
  loadError.value = false
  try {
    if (!props.apiKey) throw new Error('Missing key')
    const result = await fetchCodexModelsManifest(normalizeRikkaBaseUrl(props.baseUrl), props.apiKey, signal)
    if (version !== loadVersion) return
    models.value = parseRikkaModels(result.content)
    selectedId.value = models.value.find(model => model.id === 'gpt-6-astra')?.id || models.value[0]?.id || ''
  } catch {
    if (version === loadVersion) loadError.value = true
  } finally {
    clearTimeout(timeout)
    if (version === loadVersion) loading.value = false
  }
}

async function generateQr() {
  if (!selectedModel.value || loading.value) return
  clearQr()
  const version = qrVersion
  generating.value = true
  try {
    const payload = buildRikkaPayload(normalizeRikkaBaseUrl(props.baseUrl), props.apiKey, selectedModel.value)
    const url = await QRCode.toDataURL(payload, { errorCorrectionLevel: 'L', margin: 4, width: 768 })
    if (version === qrVersion) qrUrl.value = url
  } catch {
    if (version === qrVersion) qrError.value = true
  } finally {
    if (version === qrVersion) generating.value = false
  }
}

watch(() => [props.apiKey, props.baseUrl], loadModels, { immediate: true, flush: 'sync' })
watch(selectedId, clearQr, { flush: 'sync' })
onBeforeUnmount(() => { ++loadVersion; controller?.abort(); clearQr() })
</script>
