<template>
  <BaseDialog
    :show="show"
    :title="t('keys.oneClick.title')"
    width="extra-wide"
    @close="emit('close')"
  >
    <div class="space-y-3">
      <div class="flex flex-col gap-3 rounded-lg border border-primary-200 bg-primary-50 px-3 py-2.5 dark:border-primary-800 dark:bg-primary-950/30 sm:flex-row sm:items-center sm:justify-between">
        <div class="min-w-0 flex-1">
          <div class="flex flex-wrap items-center gap-2">
            <label for="ccswitch-key-select" class="shrink-0 text-sm font-medium text-gray-900 dark:text-white">
              {{ t('keys.oneClick.currentKeyLabel') }}
            </label>
            <select
              v-if="selectableKeys.length > 0"
              id="ccswitch-key-select"
              v-model.number="selectedKeyId"
              class="min-w-0 max-w-full rounded-md border border-primary-200 bg-white px-2 py-1 text-sm font-medium text-gray-900 outline-none focus:border-primary-500 focus:ring-1 focus:ring-primary-500 dark:border-primary-700 dark:bg-dark-800 dark:text-white"
              data-testid="ccswitch-key-select"
              :aria-label="t('keys.oneClick.selectKey')"
            >
              <option v-for="key in selectableKeys" :key="key.id" :value="key.id">
                {{ key.name || `#${key.id}` }} · {{ maskApiKey(key.key) }}
              </option>
            </select>
            <span v-else class="truncate text-sm font-medium text-gray-900 dark:text-white">
              {{ currentKey?.name || keyName }}
            </span>
          </div>
          <p class="text-xs text-gray-600 dark:text-gray-300">
            {{ t('keys.oneClick.currentKeyDescription') }}
          </p>
        </div>
        <code class="self-start rounded-md bg-white px-2 py-1 text-xs text-primary-700 shadow-sm sm:self-auto dark:bg-dark-800 dark:text-primary-300">
          {{ maskedKey }}
        </code>
      </div>

      <div
        :class="[
          'grid gap-1 rounded-lg bg-gray-100 p-1 dark:bg-dark-800',
          methods.length === 3 ? 'grid-cols-3' : 'grid-cols-2'
        ]"
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
          :data-quick-connect-mode="method.id"
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

      <section aria-labelledby="ccswitch-app-selector-title" data-testid="ccswitch-app-selector">
        <div class="flex flex-wrap items-end justify-between gap-2">
          <div>
            <h3 id="ccswitch-app-selector-title" class="text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('keys.oneClick.selectClient') }}
            </h3>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t('keys.oneClick.selectClientDescription') }}
            </p>
          </div>
          <span class="text-xs font-medium text-primary-700 dark:text-primary-300">
            {{ selectedAppMeta?.label || selectedApp }}
          </span>
        </div>
        <div class="mt-2 grid grid-cols-2 gap-2 sm:grid-cols-3 md:grid-cols-5" role="radiogroup" aria-labelledby="ccswitch-app-selector-title">
          <button
            v-for="app in ccSwitchApps"
            :key="app.id"
            type="button"
            role="radio"
            :aria-checked="selectedApp === app.id"
            :aria-disabled="!app.importable"
            :disabled="!app.importable"
            :title="app.importable
              ? (app.aliasOf ? t('keys.oneClick.sharedProvider') : app.label)
              : (app.id === 'pi' ? t('keys.oneClick.piManualReason') : t('keys.oneClick.appUnavailable'))"
            :data-testid="`ccswitch-app-${app.id}`"
            :data-quick-connect-client="app.id"
            :class="[
              'flex min-h-12 min-w-0 items-center justify-center gap-2 rounded-lg border px-2 py-2 text-left transition-colors',
              selectedApp === app.id
                ? 'border-primary-500 bg-primary-50 text-primary-800 shadow-sm ring-1 ring-primary-500 dark:border-primary-400 dark:bg-primary-950/40 dark:text-primary-200'
                : app.importable
                  ? 'border-gray-200 bg-white text-gray-700 hover:border-primary-300 hover:bg-primary-50/40 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300 dark:hover:border-primary-700'
                  : 'cursor-not-allowed border-gray-200 bg-gray-50 text-gray-400 opacity-60 dark:border-dark-700 dark:bg-dark-900 dark:text-dark-500'
            ]"
            @click="selectApp(app.id)"
          >
            <span :class="['flex h-8 w-8 shrink-0 items-center justify-center rounded-md', appIconBackground(app.id)]">
              <CcSwitchAppIcon :app="app.id" :label="app.label" size="md" />
            </span>
            <span class="min-w-0">
              <span class="block truncate text-xs font-semibold">{{ app.label }}</span>
              <span v-if="app.aliasOf" class="mt-0.5 block truncate text-[10px] opacity-70">{{ t('keys.oneClick.sharedProvider') }}</span>
              <span v-else-if="!app.importable" class="mt-0.5 block truncate text-[10px] opacity-70">{{ t('keys.oneClick.manualOnly') }}</span>
            </span>
          </button>
        </div>
      </section>

      <section
        :id="`codex-method-panel-${activeMethod}`"
        class="rounded-lg border border-gray-200 p-3 dark:border-dark-700"
        role="tabpanel"
        :aria-labelledby="`codex-method-tab-${activeMethod}`"
      >
        <template v-if="activeMethod === 'guide'">
          <h3 id="codex-guide-title" class="text-base font-semibold text-gray-900 dark:text-white">
            {{ selectedApp === 'codex' ? t('keys.oneClick.guideTitle') : t('keys.oneClick.guideClientTitle', { app: selectedAppMeta?.label || selectedApp }) }}
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
              :data-quick-connect-platform="os.id"
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
                <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ installStep.title }}</h4>
                <p class="text-sm text-gray-500 dark:text-gray-400">{{ installStep.description }}</p>
                <p class="mt-0.5 text-xs text-gray-400 dark:text-gray-500">{{ t('keys.oneClick.officialAddressHint') }}</p>
              </div>
              <a
                :href="installStep.url"
                target="_blank"
                rel="noopener noreferrer"
                class="btn btn-secondary max-w-full shrink-0 whitespace-normal text-center"
                data-testid="download-codex-app"
              >
                <Icon :name="installStep.external ? 'externalLink' : 'download'" size="sm" />
                {{ installStep.button }}
              </a>
            </div>
            <div class="flex flex-col gap-2 p-3 sm:flex-row sm:items-center sm:gap-3">
              <span class="flex h-7 w-7 shrink-0 items-center justify-center rounded-full border border-gray-200 text-sm font-medium text-gray-600 dark:border-dark-600 dark:text-gray-300">2</span>
              <div class="min-w-0 flex-1">
                <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('keys.oneClick.installCcSwitchTitle') }}</h4>
                <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('keys.oneClick.installCcSwitchDescriptionFor', { app: selectedAppMeta?.label || selectedApp }) }}</p>
                <p class="mt-0.5 text-xs text-gray-400 dark:text-gray-500">{{ t('keys.oneClick.ccSwitchDirectDownloadHint') }}</p>
              </div>
              <div class="flex w-full flex-wrap items-end justify-end gap-2 sm:w-auto sm:shrink-0">
                <label class="min-w-[10rem] flex-1 text-xs text-gray-600 dark:text-gray-300 sm:flex-none">
                  <span class="mb-1 block font-medium">{{ t('keys.oneClick.ccSwitchVersion') }}</span>
                  <input
                    v-model="ccSwitchVersion"
                    list="ccswitch-version-options"
                    type="text"
                    class="input h-9 w-full min-w-0 text-sm sm:w-40"
                    :placeholder="t('keys.oneClick.latestVersion')"
                    :aria-label="t('keys.oneClick.ccSwitchVersion')"
                    data-testid="ccswitch-version-input"
                    @input="lastResolvedDownload = null"
                  />
                  <datalist id="ccswitch-version-options">
                    <option value="latest">{{ t('keys.oneClick.latestVersion') }}</option>
                    <option v-for="version in ccSwitchVersions" :key="version.tag_name" :value="version.tag_name">
                      {{ version.name || version.tag_name }}
                    </option>
                  </datalist>
                </label>
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
            </div>
            <div class="flex flex-wrap items-center justify-end gap-2 px-3 py-2" data-testid="cc-switch-architecture">
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
              <span class="ml-1 text-xs text-gray-400 dark:text-gray-500">
                {{ ccSwitchVersionsStatus === 'loading' ? t('keys.oneClick.loadingVersions') : t('keys.oneClick.versionSourceHint') }}
              </span>
              <a
                :href="CC_SWITCH_RELEASE_URL"
                target="_blank"
                rel="noopener noreferrer"
                class="text-xs font-medium text-gray-500 underline dark:text-gray-400"
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
                <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ importStep.title }}</h4>
                <p class="text-sm text-gray-500 dark:text-gray-400">{{ importStep.description }}</p>
              </div>
              <button type="button" class="btn btn-primary max-w-full shrink-0 whitespace-normal text-center" :disabled="!selectedAppImportable" data-testid="guide-open-ccswitch" data-quick-connect-import="guide" @click="openCcSwitch">
                <Icon name="upload" size="sm" />
                {{ importStep.button }}
              </button>
            </div>
          </div>
        </template>

        <template v-else-if="activeMethod === 'ccswitch'">
          <h3 class="text-base font-semibold text-gray-900 dark:text-white">
            {{ t('keys.oneClick.ccswitchTitle') }}
          </h3>
          <p class="mt-1 text-sm text-gray-600 dark:text-gray-300">
            {{ t('keys.oneClick.ccswitchDescriptionFor', { app: selectedAppMeta?.label || selectedApp }) }}
          </p>
          <div class="mt-4 grid grid-cols-2 gap-3 rounded-lg bg-gray-50 p-3 text-sm dark:bg-dark-800">
            <div>
              <span class="text-gray-500 dark:text-gray-400">{{ t('keys.oneClick.client') }}</span>
              <p class="mt-1 font-medium text-gray-900 dark:text-white">{{ selectedAppMeta?.label || selectedApp }}</p>
            </div>
            <div>
              <span class="text-gray-500 dark:text-gray-400">{{ t('keys.oneClick.model') }}</span>
              <p class="mt-1 font-medium text-gray-900 dark:text-white">{{ effectiveImportModel || t('keys.oneClick.clientDefaultModel') }}</p>
            </div>
          </div>
          <label v-if="selectedImportModel" class="mt-4 block max-w-md text-sm text-gray-700 dark:text-gray-300">
            <span class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('keys.oneClick.model') }}</span>
            <input
              v-model="selectedModel"
              type="text"
              class="input w-full"
              :placeholder="selectedImportModel"
              data-testid="ccswitch-model-input"
            />
          </label>
          <button
            type="button"
            class="btn btn-primary mt-5"
            :disabled="!selectedAppImportable"
            data-testid="open-ccswitch"
            data-quick-connect-import="ccswitch"
            @click="openCcSwitch"
          >
            <Icon name="upload" size="sm" />
            {{ t('keys.oneClick.openCcSwitchFor', { app: selectedAppMeta?.label || selectedApp }) }}
          </button>
          <p v-if="!selectedAppImportable" class="mt-2 text-xs text-amber-700 dark:text-amber-300" role="status">
            {{ selectedAppMeta?.reason || t('keys.oneClick.appUnavailable') }}
          </p>
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
                :data-quick-connect-platform="os.id"
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
            <button type="button" class="btn btn-primary shrink-0 sm:self-stretch" data-testid="download-codex-script" data-quick-connect-download-script @click="downloadScript">
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
      <div class="flex w-full justify-end gap-2">
        <button type="button" class="btn btn-secondary" @click="emit('manage-keys')">
          {{ t('keys.oneClick.manageKeys') }}
        </button>
        <button type="button" class="btn btn-secondary" @click="emit('close')">
          {{ t('keys.oneClick.later') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, nextTick, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import CcSwitchAppIcon from '@/components/keys/CcSwitchAppIcon.vue'
import { maskApiKey } from '@/utils/maskApiKey'
import {
  CC_SWITCH_APP_CATALOG,
  buildCcSwitchImportDeeplink,
  buildCcSwitchUsageUrl,
  resolveCcSwitchImportConfig,
  type CcSwitchAppType
} from '@/utils/ccswitchImport'
import type { ApiKey, GroupPlatform } from '@/types'
import { useClipboard } from '@/composables/useClipboard'
import {
  buildCCSwitchDirectDownloadURL,
  listCCSwitchVersions,
  resolveCCSwitchDownload,
  startCCSwitchDownload,
  type CCSwitchArchitecture,
  type CCSwitchDownload,
  type CCSwitchReleaseVersion
} from '@/api/downloads'
import {
  buildCodexSetupScript,
  buildCodexSetupScriptPreview,
  getCodexSetupFilename,
  type CodexOperatingSystem
} from '@/utils/codexOneClick'

type AccessMethod = 'guide' | 'ccswitch' | 'script'
type QuickConnectKey = Pick<ApiKey, 'id' | 'name' | 'key' | 'status' | 'group'>

const CODEX_DOWNLOAD_URLS: Record<CodexOperatingSystem, string> = {
  windows: 'https://get.microsoft.com/installer/download/9PLM9XGG6VKS?cid=website_cta_psi',
  macos: 'https://persistent.oaistatic.com/codex-app-prod/Codex.dmg',
  linux: 'https://learn.chatgpt.com/docs/codex/cli'
}
const CC_SWITCH_RELEASE_URL = 'https://github.com/farion1231/cc-switch/releases/latest'
const CC_SWITCH_CLIENT_INSTALL_URLS: Record<CcSwitchAppType, string> = {
  claude: 'https://docs.anthropic.com/en/docs/claude-code/setup',
  'claude-desktop': 'https://claude.ai/download',
  codex: '',
  gemini: 'https://github.com/google-gemini/gemini-cli',
  // Grok Build's official CLI entry point; this is intentionally separate
  // from CC Switch's own release page.
  grokbuild: 'https://x.ai/cli',
  opencode: 'https://opencode.ai/download',
  openclaw: 'https://docs.openclaw.ai/install',
  hermes: 'https://hermes-agent.nousresearch.com/docs/getting-started/installation/',
  pi: 'https://github.com/badlogic/pi-mono'
}

const props = defineProps<{
  show: boolean
  apiKey: string
  keyName: string
  baseUrl: string
  providerName: string
  platform?: GroupPlatform | null
  defaultApp?: CcSwitchAppType
  initialMethod?: AccessMethod
  availableKeys?: ApiKey[]
  initialKeyId?: number | null
}>()

const emit = defineEmits<{
  (event: 'close'): void
  (event: 'protocol-failed'): void
  (event: 'manage-keys'): void
}>()

const { t } = useI18n()
const activeMethod = ref<AccessMethod>(props.initialMethod || 'guide')
const activeOs = ref<CodexOperatingSystem>('windows')
const activeArch = ref<CCSwitchArchitecture>('amd64')
const selectedKeyId = ref<number | null>(props.initialKeyId ?? null)
const selectedApp = ref<CcSwitchAppType>(props.defaultApp || defaultAppForPlatform(props.platform))
const selectedModel = ref('')
if (selectedApp.value !== 'codex' && activeMethod.value === 'script') {
  activeMethod.value = 'guide'
}
const ccSwitchDownloadStatus = ref<'idle' | 'loading' | 'error'>('idle')
const ccSwitchVersion = ref('latest')
const ccSwitchVersions = ref<CCSwitchReleaseVersion[]>([])
const ccSwitchVersionsStatus = ref<'idle' | 'loading' | 'error'>('idle')
const lastResolvedDownload = ref<CCSwitchDownload | null>(null)
const copyStatus = ref<'idle' | 'success' | 'error'>('idle')
const { copyToClipboard: clipboardCopy } = useClipboard()
const PROTOCOL_FAILURE_DELAY_MS = 1800
let protocolCheckTimer: ReturnType<typeof setTimeout> | null = null
let protocolListenersActive = false
let ccSwitchDownloadController: AbortController | null = null
let ccSwitchDownloadRequestId = 0
let ccSwitchVersionController: AbortController | null = null
let ccSwitchVersionRequestId = 0

const methods = computed(() => {
  const availableMethods: Array<{ id: AccessMethod; label: string }> = [
    { id: 'guide', label: t('keys.oneClick.guide') },
    { id: 'ccswitch', label: t('keys.oneClick.ccswitch') }
  ]
  // The generated setup script configures Codex specifically. Keeping it out
  // of other client flows avoids presenting a runnable script for the wrong app.
  if (selectedApp.value === 'codex') {
    availableMethods.push({ id: 'script', label: t('keys.oneClick.script') })
  }
  return availableMethods
})
const operatingSystems: Array<{ id: CodexOperatingSystem; label: string }> = [
  { id: 'macos', label: 'macOS' },
  { id: 'windows', label: 'Windows' },
  { id: 'linux', label: 'Linux' }
]
const ccSwitchArchitectures: Array<{ id: CCSwitchArchitecture; label: string }> = [
  { id: 'amd64', label: 'x64' },
  { id: 'arm64', label: 'ARM64' }
]
const ccSwitchApps = CC_SWITCH_APP_CATALOG
const appIconBackgrounds: Record<CcSwitchAppType, string> = {
  claude: 'bg-orange-50 text-orange-600 dark:bg-orange-950/40 dark:text-orange-300',
  'claude-desktop': 'bg-orange-50 text-orange-600 dark:bg-orange-950/40 dark:text-orange-300',
  codex: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300',
  gemini: 'bg-blue-50 text-blue-600 dark:bg-blue-950/40 dark:text-blue-300',
  grokbuild: 'bg-zinc-100 text-zinc-700 dark:bg-zinc-800 dark:text-zinc-200',
  opencode: 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-200',
  openclaw: 'bg-rose-50 text-rose-600 dark:bg-rose-950/40 dark:text-rose-300',
  hermes: 'bg-slate-100 text-slate-700 dark:bg-slate-800 dark:text-slate-200',
  pi: 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400'
}

function appIconBackground(app: CcSwitchAppType): string {
  return appIconBackgrounds[app]
}
const selectableKeys = computed<QuickConnectKey[]>(() =>
  (props.availableKeys || []).filter((key) => key.status === 'active' && key.key.trim().length > 0)
)
const fallbackKey = computed<QuickConnectKey | null>(() => props.apiKey
  ? {
      id: props.initialKeyId ?? -1,
      name: props.keyName,
      key: props.apiKey,
      status: 'active',
      group: props.platform ? ({ platform: props.platform } as ApiKey['group']) : undefined
    }
  : null
)
const currentKey = computed<QuickConnectKey | null>(() =>
  selectableKeys.value.find((key) => key.id === selectedKeyId.value) || fallbackKey.value
)
const effectivePlatform = computed<GroupPlatform>(() =>
  currentKey.value?.group?.platform || props.platform || 'openai'
)

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

function defaultAppForPlatform(platform: GroupPlatform | null | undefined): CcSwitchAppType {
  switch (platform) {
    case 'anthropic':
      return 'claude'
    case 'gemini':
      return 'gemini'
    case 'grok':
      return 'grokbuild'
    case 'antigravity':
      return 'claude'
    case 'openai':
    default:
      return 'codex'
  }
}

const selectedAppMeta = computed(() => ccSwitchApps.find((app) => app.id === selectedApp.value))
const selectedImportConfig = computed(() => resolveCcSwitchImportConfig(
  effectivePlatform.value,
  'claude',
  props.baseUrl,
  selectedApp.value
))
const selectedAppImportable = computed(() => selectedImportConfig.value.importable)
const selectedImportModel = computed(() => selectedImportConfig.value.model || '')
const effectiveImportModel = computed(() => selectedModel.value.trim() || selectedImportModel.value)

const installStep = computed(() => {
  if (selectedApp.value === 'codex') {
    return {
      ...codexDownload.value,
      external: activeOs.value === 'linux'
    }
  }

  const app = selectedAppMeta.value
  return {
    url: CC_SWITCH_CLIENT_INSTALL_URLS[selectedApp.value] || CC_SWITCH_RELEASE_URL,
    title: t('keys.oneClick.installClientTitle', { app: app?.label || selectedApp.value }),
    description: t('keys.oneClick.installClientDescription', { app: app?.label || selectedApp.value }),
    button: t('keys.oneClick.openClientGuide'),
    external: true
  }
})

const importStep = computed(() => ({
  title: selectedApp.value === 'codex'
    ? t('keys.oneClick.importCodexTitle')
    : t('keys.oneClick.importClientTitle', { app: selectedAppMeta.value?.label || selectedApp.value }),
  description: selectedApp.value === 'codex'
    ? t('keys.oneClick.importCodexDescription')
    : t('keys.oneClick.importClientDescription', { app: selectedAppMeta.value?.label || selectedApp.value }),
  button: selectedApp.value === 'codex'
    ? t('keys.oneClick.openCcSwitch')
    : t('keys.oneClick.openCcSwitchFor', { app: selectedAppMeta.value?.label || selectedApp.value })
}))

function syncSelectedApp(): void {
  const requested = props.defaultApp || defaultAppForPlatform(effectivePlatform.value)
  const candidate = ccSwitchApps.find((app) => app.id === requested)
  selectedApp.value = candidate?.importable ? candidate.id : defaultAppForPlatform(effectivePlatform.value)
  selectedModel.value = ''
  if (selectedApp.value !== 'codex' && activeMethod.value === 'script') {
    activeMethod.value = 'guide'
  }
}

function syncSelectedKey(preferInitial = true): void {
  const preferredId = props.initialKeyId
  if (preferInitial && preferredId !== null && preferredId !== undefined && selectableKeys.value.some((key) => key.id === preferredId)) {
    selectedKeyId.value = preferredId
    return
  }
  if (selectedKeyId.value !== null && selectableKeys.value.some((key) => key.id === selectedKeyId.value)) return
  const matchingPropKey = selectableKeys.value.find((key) => key.key === props.apiKey)
  selectedKeyId.value = matchingPropKey?.id ?? selectableKeys.value[0]?.id ?? null
}

function selectApp(app: CcSwitchAppType): void {
  const candidate = ccSwitchApps.find((item) => item.id === app)
  if (!candidate?.importable) return
  selectedApp.value = app
  selectedModel.value = ''
  copyStatus.value = 'idle'
  if (app !== 'codex' && activeMethod.value === 'script') {
    activeMethod.value = 'guide'
  }
}
const maskedKey = computed(() => maskApiKey(currentKey.value?.key || props.apiKey))
const scriptPreview = computed(() => buildCodexSetupScriptPreview(activeOs.value, props.baseUrl))
const runCommand = computed(() => activeOs.value === 'windows'
  ? `powershell -ExecutionPolicy Bypass -File "$env:USERPROFILE\\Downloads\\${getCodexSetupFilename(activeOs.value)}"`
  : `sh ~/Downloads/${getCodexSetupFilename(activeOs.value)}`
)

watch(() => props.show, (show) => {
  if (show) {
    activeMethod.value = props.initialMethod || 'guide'
    activeOs.value = 'windows'
    activeArch.value = 'amd64'
    syncSelectedKey()
    syncSelectedApp()
    copyStatus.value = 'idle'
    ccSwitchDownloadStatus.value = 'idle'
    ccSwitchVersion.value = 'latest'
    lastResolvedDownload.value = null
    void loadCCSwitchVersions()
  } else {
    clearProtocolCheck()
    cancelCcSwitchDownload()
    cancelCCSwitchVersionLoad()
  }
})

watch(() => [props.platform, props.defaultApp, props.initialKeyId, props.apiKey], () => {
  if (props.show) {
    syncSelectedKey()
    syncSelectedApp()
  }
})

// The parent may append more active keys after the modal opens. Preserve a
// user's current selection while still recovering the initial key if it was
// not present in the first page of results.
watch(() => props.availableKeys, () => {
  if (props.show) syncSelectedKey(false)
})

watch(selectedKeyId, () => {
  if (props.show) syncSelectedApp()
})

watch(activeOs, () => {
  copyStatus.value = 'idle'
  cancelCcSwitchDownload()
})

watch(activeArch, () => {
  cancelCcSwitchDownload()
})

watch(ccSwitchVersion, () => {
  lastResolvedDownload.value = null
  cancelCcSwitchDownload()
})

function cancelCcSwitchDownload(): void {
  ccSwitchDownloadRequestId += 1
  ccSwitchDownloadController?.abort()
  ccSwitchDownloadController = null
  ccSwitchDownloadStatus.value = 'idle'
}

function cancelCCSwitchVersionLoad(): void {
  ccSwitchVersionRequestId += 1
  ccSwitchVersionController?.abort()
  ccSwitchVersionController = null
  ccSwitchVersionsStatus.value = 'idle'
}

async function loadCCSwitchVersions(): Promise<void> {
  cancelCCSwitchVersionLoad()
  const requestId = ++ccSwitchVersionRequestId
  const controller = new AbortController()
  ccSwitchVersionController = controller
  ccSwitchVersionsStatus.value = 'loading'
  try {
    const result = await listCCSwitchVersions(20, controller.signal)
    if (requestId !== ccSwitchVersionRequestId || !props.show) return
    ccSwitchVersions.value = result.versions || []
    ccSwitchVersionsStatus.value = 'idle'
  } catch {
    if (requestId !== ccSwitchVersionRequestId || !props.show) return
    ccSwitchVersionsStatus.value = 'error'
  } finally {
    if (requestId === ccSwitchVersionRequestId) ccSwitchVersionController = null
  }
}

async function downloadCcSwitch(): Promise<void> {
  if (ccSwitchDownloadStatus.value === 'loading') return
  const requestedOs = activeOs.value
  const requestedArch = activeArch.value
  const requestedVersion = ccSwitchVersion.value.trim()
  const version = requestedVersion && requestedVersion.toLowerCase() !== 'latest' ? requestedVersion : undefined
  const requestId = ++ccSwitchDownloadRequestId
  const controller = new AbortController()
  ccSwitchDownloadController = controller
  ccSwitchDownloadStatus.value = 'loading'
  try {
    const download = version
      ? await resolveCCSwitchDownload(requestedOs, requestedArch, version, controller.signal)
      : await resolveCCSwitchDownload(requestedOs, requestedArch, controller.signal)
    if (requestId !== ccSwitchDownloadRequestId || !props.show || activeOs.value !== requestedOs || activeArch.value !== requestedArch || ccSwitchVersion.value.trim() !== requestedVersion) return
    lastResolvedDownload.value = download
    // Build the same-origin endpoint from the current frontend API base. The
    // backend's metadata is also consumed by deployments mounted below a
    // custom API prefix, so a hard-coded legacy `direct_url` must not win here.
    const directURL = buildCCSwitchDirectDownloadURL(requestedOs, requestedArch, version)
    startCCSwitchDownload(directURL)
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
  if (!selectedAppImportable.value) return
  const usageUrl = buildCcSwitchUsageUrl(props.baseUrl)
  const usageScript = `({ request: { url: ${JSON.stringify(usageUrl)}, method: "GET", headers: { "Authorization": "Bearer {{apiKey}}" } }, extractor: function(response) { return { isValid: response?.is_active ?? response?.isValid ?? true, remaining: response?.remaining ?? response?.quota?.remaining ?? response?.balance, unit: response?.unit ?? response?.quota?.unit ?? "USD" }; } })`
  const deeplink = buildCcSwitchImportDeeplink({
    baseUrl: props.baseUrl,
    platform: effectivePlatform.value,
    clientType: 'claude',
    app: selectedApp.value,
    model: effectiveImportModel.value || undefined,
    providerName: props.providerName,
    apiKey: currentKey.value?.key || props.apiKey,
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
  const content = buildCodexSetupScript(activeOs.value, props.baseUrl, currentKey.value?.key || props.apiKey)
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
  const content = buildCodexSetupScript(activeOs.value, props.baseUrl, currentKey.value?.key || props.apiKey)
  const success = await clipboardCopy(content, t('keys.oneClick.scriptCopied'))
  copyStatus.value = success ? 'success' : 'error'
}

onUnmounted(() => {
  clearProtocolCheck()
  cancelCcSwitchDownload()
  cancelCCSwitchVersionLoad()
})
</script>
