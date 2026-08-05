<template>
  <BaseDialog
    :show="show"
    :title="t('keys.oneClick.title')"
    width="extra-wide"
    @close="emit('close')"
  >
    <div class="space-y-3">
      <div class="flex items-center justify-between gap-3 rounded-lg border border-primary-200 bg-primary-50 px-3 py-2 dark:border-primary-800 dark:bg-primary-950/30">
        <div class="min-w-0">
          <p class="truncate text-sm font-medium text-gray-900 dark:text-white">
            {{ t('keys.oneClick.currentKey', { name: keyName }) }}
          </p>
          <p class="text-xs text-gray-600 dark:text-gray-300">
            {{ t('keys.oneClick.securityHint') }}
          </p>
        </div>
        <code class="shrink-0 rounded-md bg-white px-2 py-1 text-xs text-primary-700 shadow-sm dark:bg-dark-800 dark:text-primary-300">
          {{ maskedKey }}
        </code>
      </div>

      <div
        class="grid grid-cols-3 gap-1 rounded-lg bg-gray-100 p-1 dark:bg-dark-800"
        role="tablist"
        :aria-label="t('keys.oneClick.title')"
        aria-orientation="horizontal"
      >
        <button
          v-for="method in methods"
          :key="method.id"
          :id="`codex-method-tab-${method.id}`"
          type="button"
          role="tab"
          :aria-selected="activeMethod === method.id"
          :aria-controls="`codex-method-panel-${method.id}`"
          :tabindex="activeMethod === method.id ? 0 : -1"
          :data-testid="`codex-method-${method.id}`"
          :class="[
            'min-h-9 rounded-md px-2 py-1.5 text-sm font-medium transition-colors',
            activeMethod === method.id
              ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white'
              : 'text-gray-500 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white'
          ]"
          @click="activeMethod = method.id"
          @keydown="handleMethodKeydown($event, method.id)"
        >
          {{ method.label }}
        </button>
      </div>

      <section
        :id="`codex-method-panel-${activeMethod}`"
        class="rounded-lg border border-gray-200 p-3 dark:border-dark-700"
        role="tabpanel"
        :aria-labelledby="`codex-method-tab-${activeMethod}`"
      >
        <template v-if="activeMethod === 'guide'">
          <h3 id="codex-guide-title" class="text-base font-semibold text-gray-900 dark:text-white">
            {{ t('keys.oneClick.guideTitle') }}
          </h3>
          <div class="mt-2 flex flex-wrap gap-2" role="radiogroup" aria-labelledby="codex-guide-title">
            <button
              v-for="os in operatingSystems"
              :key="os.id"
              :id="`guide-os-radio-${os.id}`"
              type="button"
              role="radio"
              :aria-checked="activeOs === os.id"
              :tabindex="activeOs === os.id ? 0 : -1"
              :data-testid="`guide-os-${os.id}`"
              :class="['btn min-h-8 py-1', activeOs === os.id ? 'btn-primary' : 'btn-secondary']"
              @click="activeOs = os.id"
              @keydown="handleOsKeydown($event, os.id, 'guide')"
            >
              {{ os.label }}
            </button>
          </div>
          <div class="mt-2 divide-y divide-gray-200 overflow-hidden rounded-lg border border-gray-200 dark:divide-dark-700 dark:border-dark-700">
            <div class="flex flex-col gap-2 p-3 sm:flex-row sm:items-center sm:gap-3">
              <span class="flex h-7 w-7 shrink-0 items-center justify-center rounded-full border border-gray-200 text-sm font-medium text-gray-600 dark:border-dark-600 dark:text-gray-300">1</span>
              <div class="min-w-0 flex-1">
                <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ codexDownload.title }}</h4>
                <p class="text-sm text-gray-500 dark:text-gray-400">{{ codexDownload.description }}</p>
                <p class="mt-0.5 text-xs text-gray-400 dark:text-gray-500">{{ t('keys.oneClick.officialAddressHint') }}</p>
              </div>
              <a
                :href="codexDownload.url"
                target="_blank"
                rel="noopener noreferrer"
                class="btn btn-secondary max-w-full shrink-0 whitespace-normal text-center"
                data-testid="download-codex-app"
              >
                <Icon :name="activeOs === 'linux' ? 'externalLink' : 'download'" size="sm" />
                {{ codexDownload.button }}
              </a>
            </div>
            <div class="flex flex-col gap-2 p-3 sm:flex-row sm:items-center sm:gap-3">
              <span class="flex h-7 w-7 shrink-0 items-center justify-center rounded-full border border-gray-200 text-sm font-medium text-gray-600 dark:border-dark-600 dark:text-gray-300">2</span>
              <div class="min-w-0 flex-1">
                <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('keys.oneClick.installCcSwitchTitle') }}</h4>
                <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('keys.oneClick.installCcSwitchDescription') }}</p>
                <p class="mt-0.5 text-xs text-gray-400 dark:text-gray-500">{{ t('keys.oneClick.officialAddressHint') }}</p>
              </div>
              <button
                type="button"
                :disabled="ccSwitchDownloadStatus === 'loading'"
                class="btn btn-secondary max-w-full shrink-0 whitespace-normal text-center disabled:cursor-wait disabled:opacity-60"
                data-testid="download-cc-switch"
                @click="downloadCcSwitch"
              >
                <Icon name="download" size="sm" />
                {{ ccSwitchDownloadStatus === 'loading' ? t('keys.oneClick.resolvingCcSwitch') : t('keys.oneClick.downloadCcSwitch') }}
              </button>
            </div>
            <div class="flex flex-wrap items-center justify-end gap-2 px-3 pb-2" data-testid="cc-switch-architecture">
              <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('keys.oneClick.architecture') }}</span>
              <button
                v-for="arch in ccSwitchArchitectures"
                :key="arch.id"
                type="button"
                :aria-pressed="activeArch === arch.id"
                :class="['btn min-h-7 px-2 py-0.5 text-xs', activeArch === arch.id ? 'btn-primary' : 'btn-secondary']"
                :data-testid="`cc-switch-arch-${arch.id}`"
                @click="activeArch = arch.id"
              >
                {{ arch.label }}
              </button>
              <a
                :href="CC_SWITCH_RELEASE_URL"
                target="_blank"
                rel="noopener noreferrer"
                class="ml-1 text-xs font-medium text-gray-500 underline dark:text-gray-400"
                data-testid="cc-switch-release-fallback"
              >{{ t('keys.oneClick.openCcSwitchReleases') }}</a>
            </div>
            <div
              v-if="ccSwitchDownloadStatus === 'error'"
              class="flex flex-wrap items-center justify-end gap-2 px-3 pb-3 text-xs text-red-600 dark:text-red-400"
              role="alert"
              data-testid="cc-switch-download-error"
            >
              <span>{{ t('keys.oneClick.ccSwitchDownloadFailed') }}</span>
            </div>
            <div class="flex flex-col gap-2 p-3 sm:flex-row sm:items-center sm:gap-3">
              <span class="flex h-7 w-7 shrink-0 items-center justify-center rounded-full border border-gray-200 text-sm font-medium text-gray-600 dark:border-dark-600 dark:text-gray-300">3</span>
              <div class="min-w-0 flex-1">
                <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('keys.oneClick.importCodexTitle') }}</h4>
                <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('keys.oneClick.importCodexDescription') }}</p>
              </div>
              <button type="button" class="btn btn-primary max-w-full shrink-0 whitespace-normal text-center" data-testid="guide-open-ccswitch" @click="openCcSwitch">
                <Icon name="upload" size="sm" />
                {{ t('keys.oneClick.openCcSwitch') }}
              </button>
            </div>
          </div>
        </template>

        <template v-else-if="activeMethod === 'ccswitch'">
          <h3 class="text-base font-semibold text-gray-900 dark:text-white">
            {{ t('keys.oneClick.ccswitchTitle') }}
          </h3>
          <p class="mt-1 text-sm text-gray-600 dark:text-gray-300">
            {{ t('keys.oneClick.ccswitchDescription') }}
          </p>
          <div class="mt-4 grid grid-cols-2 gap-3 rounded-lg bg-gray-50 p-3 text-sm dark:bg-dark-800">
            <div>
              <span class="text-gray-500 dark:text-gray-400">{{ t('keys.oneClick.client') }}</span>
              <p class="mt-1 font-medium text-gray-900 dark:text-white">Codex</p>
            </div>
            <div>
              <span class="text-gray-500 dark:text-gray-400">{{ t('keys.oneClick.model') }}</span>
              <p class="mt-1 font-medium text-gray-900 dark:text-white">{{ DEFAULT_CODEX_MODEL }}</p>
            </div>
          </div>
          <button type="button" class="btn btn-primary mt-5" data-testid="open-ccswitch" @click="openCcSwitch">
            <Icon name="upload" size="sm" />
            {{ t('keys.oneClick.openCcSwitch') }}
          </button>
        </template>

        <template v-else>
          <div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between" data-testid="codex-script-heading-row">
            <h3 id="codex-script-title" class="text-base font-semibold text-gray-900 dark:text-white">
              {{ t('keys.oneClick.scriptTitle') }}
            </h3>
            <div class="flex flex-wrap gap-2" role="radiogroup" aria-labelledby="codex-script-title">
              <button
                v-for="os in operatingSystems"
                :key="os.id"
                :id="`script-os-radio-${os.id}`"
                type="button"
                role="radio"
                :aria-checked="activeOs === os.id"
                :tabindex="activeOs === os.id ? 0 : -1"
                :data-testid="`codex-os-${os.id}`"
                :class="[
                  'btn min-h-8 py-1',
                  activeOs === os.id ? 'btn-primary' : 'btn-secondary'
                ]"
                @click="activeOs = os.id"
                @keydown="handleOsKeydown($event, os.id, 'script')"
              >
                {{ os.label }}
              </button>
            </div>
          </div>
          <p class="mt-1 text-sm text-gray-600 dark:text-gray-300">
            {{ t('keys.oneClick.scriptDescription') }}
          </p>
          <div class="mt-2 overflow-hidden rounded-lg bg-gray-950">
            <div class="flex min-h-9 items-center justify-end border-b border-gray-800 px-2 py-1" data-testid="codex-script-toolbar">
              <button
                type="button"
                class="inline-flex min-h-7 items-center gap-1.5 rounded-md px-2 text-xs font-medium text-gray-200 hover:bg-gray-800 hover:text-white"
                data-testid="copy-codex-script"
                @click="copyScript"
              >
                <Icon :name="copyStatus === 'success' ? 'checkCircle' : 'clipboard'" size="sm" />
                {{ copyStatus === 'success' ? t('keys.oneClick.scriptCopied') : t('keys.oneClick.copyScript') }}
              </button>
            </div>
            <pre class="max-h-32 overflow-auto whitespace-pre-wrap break-all p-3 text-xs leading-5 text-gray-100" data-testid="codex-script-preview"><code>{{ scriptPreview }}</code></pre>
          </div>
          <p
            v-if="copyStatus === 'error'"
            class="mt-1.5 text-xs text-red-600 dark:text-red-400"
            role="status"
            data-testid="copy-script-error"
          >
            {{ t('keys.oneClick.scriptCopyFailed') }}
          </p>
          <div class="mt-2 flex flex-col gap-2 sm:flex-row sm:items-stretch" data-testid="codex-script-run-row">
            <button type="button" class="btn btn-primary shrink-0 sm:self-stretch" data-testid="download-codex-script" @click="downloadScript">
              <Icon name="download" size="sm" />
              {{ t('keys.oneClick.downloadScript') }}
            </button>
            <div class="min-w-0 flex-1 rounded-lg border border-gray-200 bg-gray-50 px-3 py-2 dark:border-dark-700 dark:bg-dark-800">
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('keys.oneClick.runCommand') }}</p>
              <code class="mt-0.5 block max-w-full whitespace-pre-wrap break-all text-xs text-gray-800 dark:text-gray-200" data-testid="codex-script-run-command">{{ runCommand }}</code>
            </div>
          </div>
          <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">{{ t('keys.oneClick.rollbackHint') }}</p>
        </template>
      </section>
    </div>

    <template #footer>
      <button type="button" class="btn btn-secondary" @click="emit('close')">
        {{ t('common.close') }}
      </button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, nextTick, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { maskApiKey } from '@/utils/maskApiKey'
import { buildCcSwitchImportDeeplink, buildCcSwitchUsageUrl } from '@/utils/ccswitchImport'
import { useClipboard } from '@/composables/useClipboard'
import { resolveCCSwitchDownload, startCCSwitchDownload, type CCSwitchArchitecture } from '@/api/downloads'
import {
  DEFAULT_CODEX_MODEL,
  buildCodexSetupScript,
  buildCodexSetupScriptPreview,
  getCodexSetupFilename,
  type CodexOperatingSystem
} from '@/utils/codexOneClick'

type AccessMethod = 'guide' | 'ccswitch' | 'script'

const CODEX_DOWNLOAD_URLS: Record<CodexOperatingSystem, string> = {
  windows: 'https://get.microsoft.com/installer/download/9PLM9XGG6VKS?cid=website_cta_psi',
  macos: 'https://persistent.oaistatic.com/codex-app-prod/Codex.dmg',
  linux: 'https://learn.chatgpt.com/docs/codex/cli'
}
const CC_SWITCH_RELEASE_URL = 'https://github.com/farion1231/cc-switch/releases/latest'

const props = defineProps<{
  show: boolean
  apiKey: string
  keyName: string
  baseUrl: string
  providerName: string
}>()

const emit = defineEmits<{
  (event: 'close'): void
  (event: 'protocol-failed'): void
}>()

const { t } = useI18n()
const activeMethod = ref<AccessMethod>('guide')
const activeOs = ref<CodexOperatingSystem>('windows')
const activeArch = ref<CCSwitchArchitecture>('amd64')
const ccSwitchDownloadStatus = ref<'idle' | 'loading' | 'error'>('idle')
const copyStatus = ref<'idle' | 'success' | 'error'>('idle')
const { copyToClipboard: clipboardCopy } = useClipboard()
const PROTOCOL_FAILURE_DELAY_MS = 1800
let protocolCheckTimer: ReturnType<typeof setTimeout> | null = null
let protocolListenersActive = false
let ccSwitchDownloadController: AbortController | null = null
let ccSwitchDownloadRequestId = 0

const methods = computed(() => [
  { id: 'guide' as const, label: t('keys.oneClick.guide') },
  { id: 'ccswitch' as const, label: t('keys.oneClick.ccswitch') },
  { id: 'script' as const, label: t('keys.oneClick.script') }
])
const operatingSystems: Array<{ id: CodexOperatingSystem; label: string }> = [
  { id: 'macos', label: 'macOS' },
  { id: 'windows', label: 'Windows' },
  { id: 'linux', label: 'Linux' }
]
const ccSwitchArchitectures: Array<{ id: CCSwitchArchitecture; label: string }> = [
  { id: 'amd64', label: 'x64' },
  { id: 'arm64', label: 'ARM64' }
]
const codexDownload = computed(() => ({
  url: CODEX_DOWNLOAD_URLS[activeOs.value],
  title: activeOs.value === 'linux'
    ? t('keys.oneClick.installCodexCliTitle')
    : t('keys.oneClick.installCodexAppTitle'),
  description: activeOs.value === 'linux'
    ? t('keys.oneClick.installCodexCliDescription')
    : t('keys.oneClick.installCodexAppDescription', { os: operatingSystems.find((os) => os.id === activeOs.value)?.label }),
  button: activeOs.value === 'windows'
    ? t('keys.oneClick.downloadCodexWindows')
    : activeOs.value === 'macos'
      ? t('keys.oneClick.downloadCodexMacos')
      : t('keys.oneClick.openCodexLinuxGuide')
}))
const maskedKey = computed(() => maskApiKey(props.apiKey))
const scriptPreview = computed(() => buildCodexSetupScriptPreview(activeOs.value, props.baseUrl))
const runCommand = computed(() => activeOs.value === 'windows'
  ? `powershell -ExecutionPolicy Bypass -File "$env:USERPROFILE\\Downloads\\${getCodexSetupFilename(activeOs.value)}"`
  : `sh ~/Downloads/${getCodexSetupFilename(activeOs.value)}`
)

watch(() => props.show, (show) => {
  if (show) {
    activeMethod.value = 'guide'
    activeOs.value = 'windows'
    activeArch.value = 'amd64'
    copyStatus.value = 'idle'
    ccSwitchDownloadStatus.value = 'idle'
  } else {
    clearProtocolCheck()
    cancelCcSwitchDownload()
  }
})

watch(activeOs, () => {
  copyStatus.value = 'idle'
  cancelCcSwitchDownload()
})

watch(activeArch, () => {
  cancelCcSwitchDownload()
})

function cancelCcSwitchDownload(): void {
  ccSwitchDownloadRequestId += 1
  ccSwitchDownloadController?.abort()
  ccSwitchDownloadController = null
  ccSwitchDownloadStatus.value = 'idle'
}

async function downloadCcSwitch(): Promise<void> {
  if (ccSwitchDownloadStatus.value === 'loading') return
  const requestedOs = activeOs.value
  const requestedArch = activeArch.value
  const requestId = ++ccSwitchDownloadRequestId
  const controller = new AbortController()
  ccSwitchDownloadController = controller
  ccSwitchDownloadStatus.value = 'loading'
  try {
    const download = await resolveCCSwitchDownload(requestedOs, requestedArch, controller.signal)
    if (requestId !== ccSwitchDownloadRequestId || !props.show || activeOs.value !== requestedOs || activeArch.value !== requestedArch) return
    startCCSwitchDownload(download.download_url)
    ccSwitchDownloadStatus.value = 'idle'
  } catch {
    if (requestId !== ccSwitchDownloadRequestId || !props.show) return
    ccSwitchDownloadStatus.value = 'error'
  } finally {
    if (requestId === ccSwitchDownloadRequestId) ccSwitchDownloadController = null
  }
}

function getKeyboardTargetIndex(event: KeyboardEvent, currentIndex: number, length: number): number | null {
  switch (event.key) {
    case 'ArrowRight':
    case 'ArrowDown':
      return (currentIndex + 1) % length
    case 'ArrowLeft':
    case 'ArrowUp':
      return (currentIndex - 1 + length) % length
    case 'Home':
      return 0
    case 'End':
      return length - 1
    default:
      return null
  }
}

function focusControl(id: string): void {
  void nextTick(() => document.getElementById(id)?.focus())
}

function handleMethodKeydown(event: KeyboardEvent, current: AccessMethod): void {
  const currentIndex = methods.value.findIndex((method) => method.id === current)
  const targetIndex = getKeyboardTargetIndex(event, currentIndex, methods.value.length)
  if (targetIndex === null) return

  event.preventDefault()
  const target = methods.value[targetIndex]
  activeMethod.value = target.id
  focusControl(`codex-method-tab-${target.id}`)
}

function handleOsKeydown(
  event: KeyboardEvent,
  current: CodexOperatingSystem,
  prefix: 'guide' | 'script'
): void {
  const currentIndex = operatingSystems.findIndex((os) => os.id === current)
  const targetIndex = getKeyboardTargetIndex(event, currentIndex, operatingSystems.length)
  if (targetIndex === null) return

  event.preventDefault()
  const target = operatingSystems[targetIndex]
  activeOs.value = target.id
  focusControl(`${prefix}-os-radio-${target.id}`)
}

function removeProtocolCheckListeners(): void {
  if (!protocolListenersActive) return
  window.removeEventListener('blur', handleProtocolBlur)
  document.removeEventListener('visibilitychange', handleProtocolVisibilityChange)
  protocolListenersActive = false
}

function clearProtocolCheck(): void {
  if (protocolCheckTimer) {
    clearTimeout(protocolCheckTimer)
    protocolCheckTimer = null
  }
  removeProtocolCheckListeners()
}

function handleProtocolBlur(): void {
  clearProtocolCheck()
}

function handleProtocolVisibilityChange(): void {
  if (document.visibilityState === 'hidden') clearProtocolCheck()
}

function startProtocolCheck(): void {
  clearProtocolCheck()
  window.addEventListener('blur', handleProtocolBlur)
  document.addEventListener('visibilitychange', handleProtocolVisibilityChange)
  protocolListenersActive = true
  protocolCheckTimer = setTimeout(() => {
    protocolCheckTimer = null
    removeProtocolCheckListeners()
    if (props.show) emit('protocol-failed')
  }, PROTOCOL_FAILURE_DELAY_MS)
}

function openCcSwitch(): void {
  const usageUrl = buildCcSwitchUsageUrl(props.baseUrl)
  const usageScript = `({ request: { url: "${usageUrl}", method: "GET", headers: { "Authorization": "Bearer {{apiKey}}" } }, extractor: function(response) { return { isValid: response?.is_active ?? response?.isValid ?? true, remaining: response?.remaining ?? response?.quota?.remaining ?? response?.balance, unit: response?.unit ?? response?.quota?.unit ?? "USD" }; } })`
  const deeplink = buildCcSwitchImportDeeplink({
    baseUrl: props.baseUrl,
    platform: 'openai',
    clientType: 'claude',
    providerName: props.providerName,
    apiKey: props.apiKey,
    usageScript
  })

  startProtocolCheck()
  try {
    window.open(deeplink, '_self')
  } catch {
    clearProtocolCheck()
    emit('protocol-failed')
  }
}

function downloadScript(): void {
  const content = buildCodexSetupScript(activeOs.value, props.baseUrl, props.apiKey)
  const url = URL.createObjectURL(new Blob([content], { type: 'text/plain;charset=utf-8' }))
  try {
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = getCodexSetupFilename(activeOs.value)
    anchor.click()
  } finally {
    setTimeout(() => URL.revokeObjectURL(url), 1000)
  }
}

async function copyScript(): Promise<void> {
  const content = buildCodexSetupScript(activeOs.value, props.baseUrl, props.apiKey)
  const success = await clipboardCopy(content, t('keys.oneClick.scriptCopied'))
  copyStatus.value = success ? 'success' : 'error'
}

onUnmounted(() => {
  clearProtocolCheck()
  cancelCcSwitchDownload()
})
</script>
