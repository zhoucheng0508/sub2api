<template>
  <div class="space-y-6" data-testid="quick-connect-panel">
    <section class="rounded-2xl border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800 sm:p-6" aria-labelledby="quick-connect-app-title">
      <div class="flex items-start gap-3">
        <span class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-primary-100 text-sm font-semibold text-primary-700 dark:bg-primary-900/40 dark:text-primary-300" aria-hidden="true">1</span>
        <div>
          <h2 id="quick-connect-app-title" class="text-base font-semibold text-gray-900 dark:text-white">{{ t('quickConnect.chooseApp') }}</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('quickConnect.chooseAppHint') }}</p>
        </div>
      </div>
      <div class="mt-5 grid grid-cols-1 gap-3 sm:grid-cols-3" :aria-label="t('quickConnect.app')" data-testid="ccswitch-app-selector">
        <button
          v-for="app in primaryApps"
          :key="app.id"
          type="button"
          :disabled="!app.importable || creationBusy"
          :aria-pressed="selectedApp === app.id"
          :data-testid="`connect-app-${app.id}`"
          :class="['flex min-h-16 min-w-0 items-center gap-2 rounded-xl border px-3 py-3 text-left transition-colors disabled:cursor-not-allowed disabled:opacity-45', selectedApp === app.id ? 'border-primary-500 bg-primary-50 text-primary-800 dark:bg-primary-950/40 dark:text-primary-200' : 'border-gray-200 text-gray-700 hover:border-primary-300 dark:border-dark-600 dark:text-gray-200']"
          @click="selectApp(app.id)"
        >
          <CcSwitchAppIcon :app="app.id" :label="app.label" size="lg" />
          <span class="min-w-0 text-sm font-medium">
            {{ app.label }}
            <span class="mt-1 block text-xs font-normal leading-relaxed">{{ t(`quickConnect.appDescriptions.${app.id}`) }}</span>
          </span>
        </button>
      </div>
      <details class="mt-3" :open="isMoreAppSelected || undefined" data-testid="connect-more-apps">
        <summary class="cursor-pointer text-sm text-gray-600 dark:text-gray-300">{{ t('quickConnect.moreApps') }}</summary>
        <div class="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-3">
          <button v-for="app in moreApps" :key="app.id" type="button" :disabled="!app.importable || creationBusy" :aria-pressed="selectedApp === app.id" :data-testid="`connect-app-${app.id}`" :class="['flex min-h-16 min-w-0 items-center gap-2 rounded-xl border p-3 text-left disabled:cursor-not-allowed disabled:opacity-45', selectedApp === app.id ? 'border-primary-500 bg-primary-50 text-primary-800 dark:bg-primary-950/40 dark:text-primary-200' : 'border-gray-200 text-gray-700 dark:border-dark-600 dark:text-gray-200']" @click="selectApp(app.id)">
            <CcSwitchAppIcon :app="app.id" :label="app.label" size="lg" />
            <span class="text-sm font-medium">{{ app.label }}<span v-if="!app.importable" class="mt-1 block text-xs font-normal">{{ t('quickConnect.unsupported') }}</span></span>
          </button>
        </div>
        <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ t('quickConnect.unsupportedHint') }}</p>
      </details>
      <div class="mt-5 flex flex-wrap items-center gap-2" role="radiogroup" :aria-label="t('quickConnect.system')">
        <span class="mr-1 w-full text-sm font-medium text-gray-700 dark:text-gray-300 sm:w-auto">{{ t('quickConnect.system') }}</span>
        <button
          v-for="os in operatingSystems"
          :id="`guide-os-radio-${os.id}`"
          :key="os.id"
          type="button"
          role="radio"
          :aria-checked="activeOs === os.id"
          :disabled="creationBusy"
          :tabindex="activeOs === os.id ? 0 : -1"
          :data-testid="`guide-os-${os.id}`"
          :data-quick-connect-platform="os.id"
          :class="['btn min-h-10 px-4', activeOs === os.id ? 'btn-primary' : 'btn-secondary']"
          @click="activeOs = os.id"
          @keydown="handleOsKeydown($event, os.id)"
        >{{ os.label }}</button>
      </div>
    </section>

    <section class="rounded-2xl border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800 sm:p-6" aria-labelledby="quick-connect-key-title">
      <div class="flex items-start gap-3">
        <span class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-primary-100 text-sm font-semibold text-primary-700 dark:bg-primary-900/40 dark:text-primary-300" aria-hidden="true">2</span>
        <div class="min-w-0 flex-1">
          <h2 id="quick-connect-key-title" class="text-base font-semibold text-gray-900 dark:text-white">{{ t(showCreateKey ? 'quickConnect.startPage.createInPlace' : 'quickConnect.chooseKey') }}</h2>
          <p v-if="!showCreateKey" class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('quickConnect.chooseKeyHint', { app: appLabel }) }}</p>
        </div>
      </div>
      <p v-if="loading" class="mt-5 text-sm text-gray-600 dark:text-gray-300" role="status" data-testid="connect-keys-loading">{{ t('quickConnect.loading') }}</p>
      <div v-else-if="loadError" class="mt-5 flex flex-wrap items-center gap-3" role="alert" data-testid="connect-keys-error">
        <p class="text-sm text-red-600 dark:text-red-400">{{ t('quickConnect.loadError') }}</p>
        <button type="button" class="btn btn-secondary" @click="emit('retry')">{{ t('quickConnect.retry') }}</button>
      </div>
      <div v-else-if="showCreateKey" class="mt-5" data-testid="connect-inline-create">
        <slot name="create-key" :app="selectedApp" :cancel="closeCreateKey" />
        <button v-if="selectableKeys.length" type="button" class="mt-3 text-sm font-medium text-primary-600 hover:underline disabled:opacity-50 dark:text-primary-400" :disabled="creationBusy" data-testid="connect-use-existing" @click="closeCreateKey">{{ t('quickConnect.startPage.useExisting') }}</button>
      </div>
      <template v-else-if="selectableKeys.length || currentKey">
        <p v-if="selectedUnavailable" class="mt-4 text-sm text-amber-700 dark:text-amber-300" role="status">{{ t('quickConnect.selectedUnavailable') }}</p>
        <div class="mt-5 space-y-2">
          <div class="flex items-center justify-between gap-3 text-xs text-gray-500 dark:text-gray-400">
            <span>{{ t(singleSelectedKey ? 'quickConnect.selectedKeyLabel' : 'quickConnect.selectKey') }}</span>
            <span v-if="selectableKeys.length">{{ t('quickConnect.keyCount', { count: selectableKeys.length }) }}</span>
          </div>
          <div v-if="singleSelectedKey && currentKey" class="flex min-w-0 items-center gap-3 rounded-xl border border-primary-200 bg-primary-50/50 p-4 dark:border-primary-800 dark:bg-primary-950/20" data-testid="connect-selected-key">
            <span class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-primary-100 text-primary-700 dark:bg-primary-900/40 dark:text-primary-300"><Icon name="key" size="md" /></span>
            <div class="min-w-0 flex-1">
              <p class="truncate text-sm font-semibold text-gray-900 dark:text-white">{{ currentKey.name || `#${currentKey.id}` }}</p>
              <p class="mt-1 truncate text-xs text-gray-500 dark:text-gray-400">{{ t('quickConnect.keyGroup') }} · {{ currentKey.group?.name || effectivePlatform }}</p>
            </div>
            <span class="shrink-0 rounded-md bg-white px-2 py-1 font-mono text-xs text-gray-600 dark:bg-dark-800 dark:text-gray-300">{{ t('quickConnect.keySuffix', { suffix: keySuffix(currentKey.key) }) }}</span>
            <Icon name="checkCircle" size="sm" class="shrink-0 text-primary-600 dark:text-primary-400" />
          </div>
          <Select v-else :model-value="selectedKeyId" :options="keyOptions" :aria-label="t('quickConnect.selectKey')" :placeholder="t('quickConnect.selectKey')" :search-placeholder="t('quickConnect.searchKeys')" :empty-text="t('quickConnect.noMatchingKey')" class="connect-key-picker" data-testid="ccswitch-key-select" @update:model-value="chooseExistingKey">
            <template #selected="{ option }">
              <span v-if="option" class="flex min-w-0 items-center gap-3 py-1">
                <span class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300"><Icon name="key" size="md" /></span>
                <span class="min-w-0 flex-1"><span class="block truncate text-sm font-semibold text-gray-900 dark:text-white">{{ option.name }}</span><span class="mt-1 block truncate text-xs text-gray-500 dark:text-gray-400">{{ t('quickConnect.keyGroup') }} · {{ option.group }}</span></span>
                <span class="shrink-0 rounded-md bg-gray-50 px-2 py-1 font-mono text-xs text-gray-600 dark:bg-dark-900 dark:text-gray-300">{{ t('quickConnect.keySuffix', { suffix: option.suffix }) }}</span>
              </span>
              <span v-else class="text-sm text-gray-500 dark:text-gray-400">{{ t('quickConnect.selectKey') }}</span>
            </template>
            <template #option="{ option, selected }">
              <span class="flex w-full min-w-0 items-center gap-3 py-1" :data-testid="`connect-key-option-${option.value}`">
                <span class="min-w-0 flex-1"><span class="block truncate text-sm font-medium">{{ option.name }}</span><span class="mt-1 block truncate text-xs text-gray-500 dark:text-gray-400">{{ t('quickConnect.keyGroup') }} · {{ option.group }}</span><span v-if="option.disabled" class="mt-1 block whitespace-normal text-xs text-amber-700 dark:text-amber-300">{{ t('quickConnect.incompatibleOption') }}</span></span>
                <span class="shrink-0 font-mono text-xs text-gray-500 dark:text-gray-400">{{ t('quickConnect.keySuffix', { suffix: option.suffix }) }}</span>
                <Icon v-if="selected" name="check" size="sm" class="shrink-0 text-primary-600 dark:text-primary-400" />
              </span>
            </template>
          </Select>
        </div>
        <p v-if="currentKey && !currentKeyCompatible" class="mt-3 text-sm text-amber-700 dark:text-amber-300" role="status" data-testid="connect-incompatible-key">{{ t('quickConnect.incompatibleKey', { app: appLabel }) }}</p>
      </template>
      <div v-else class="mt-5 rounded-xl bg-gray-50 p-4 dark:bg-dark-900/50" data-testid="connect-no-keys">
        <p class="font-medium text-gray-900 dark:text-white">{{ t('quickConnect.noKeys') }}</p>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('quickConnect.noKeysHint') }}</p>
      </div>
      <div v-if="!showCreateKey" class="mt-5 flex flex-wrap items-center justify-between gap-3 border-t border-gray-100 pt-4 dark:border-dark-700">
        <button v-if="$slots['create-key']" type="button" class="btn btn-secondary" data-testid="connect-create-key" @click="createKeyOpen = true"><Icon name="plus" size="sm" />{{ t('quickConnect.createKeyShort') }}</button>
        <button type="button" class="inline-flex items-center gap-1 text-sm font-medium text-gray-500 hover:text-primary-600 dark:text-gray-400 dark:hover:text-primary-300" data-testid="connect-manage-keys" @click="emit('manage-keys')">{{ t('quickConnect.startPage.manageLater') }}<Icon name="arrowRight" size="sm" /></button>
      </div>
    </section>

    <section class="rounded-2xl border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800 sm:p-6" aria-labelledby="quick-connect-install-title">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div class="flex items-start gap-3">
          <span class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-primary-100 text-sm font-semibold text-primary-700 dark:bg-primary-900/40 dark:text-primary-300" aria-hidden="true">3</span>
          <div>
            <h2 id="quick-connect-install-title" class="text-base font-semibold text-gray-900 dark:text-white">{{ t('quickConnect.configure') }}</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('quickConnect.configureHint') }}</p>
          </div>
        </div>
        <button type="button" class="text-sm font-medium text-primary-600 hover:underline dark:text-primary-400" :aria-expanded="showInstallHelp" aria-controls="quick-connect-install-help" data-testid="connect-toggle-install" @click="showInstallHelp = !showInstallHelp">
          {{ showInstallHelp ? t('quickConnect.alreadyInstalled') : t('quickConnect.showInstall') }}
        </button>
      </div>
      <div v-if="showInstallHelp" id="quick-connect-install-help" class="mt-5 space-y-3" data-testid="connect-install-help">
        <div class="flex flex-col gap-3 rounded-xl bg-gray-50 p-4 dark:bg-dark-900/50 sm:flex-row sm:items-center sm:justify-between">
          <div class="min-w-0">
            <h3 class="text-sm font-semibold text-gray-900 dark:text-white">3.1 · {{ t('quickConnect.installApp', { app: appLabel }) }}</h3>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('quickConnect.installAppHint') }}</p>
          </div>
          <a :href="installStep.url" target="_blank" rel="noopener noreferrer" class="btn btn-secondary min-h-10 shrink-0 whitespace-normal text-center" data-testid="download-codex-app"><Icon name="download" size="sm" />{{ installStep.button }}</a>
        </div>
        <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900/50">
          <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div class="min-w-0">
              <h3 class="text-sm font-semibold text-gray-900 dark:text-white">3.2 · {{ t('quickConnect.installSwitcher') }}</h3>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('quickConnect.installSwitcherHint') }}</p>
            </div>
            <button type="button" class="btn btn-secondary min-h-10 shrink-0 whitespace-normal text-center disabled:opacity-60" :disabled="ccSwitchDownloadStatus === 'loading'" data-testid="download-cc-switch" @click="downloadCcSwitch"><Icon name="download" size="sm" />{{ t(ccSwitchDownloadStatus === 'loading' ? 'quickConnect.downloading' : 'quickConnect.downloadSwitcher') }}</button>
          </div>
          <details class="mt-4 border-t border-gray-200 pt-3 dark:border-dark-700" data-testid="connect-download-advanced">
            <summary class="cursor-pointer text-sm text-gray-600 dark:text-gray-300">{{ t('quickConnect.otherDownloads') }}</summary>
            <div class="mt-3 flex flex-wrap items-end gap-4">
              <label class="min-w-0 text-sm text-gray-600 dark:text-gray-300"><span class="mb-1 block">{{ t('quickConnect.version') }}</span><input v-model="ccSwitchVersion" list="ccswitch-version-options" class="input h-10 w-40" :placeholder="t('quickConnect.latest')" data-testid="ccswitch-version-input" /></label>
              <datalist id="ccswitch-version-options"><option value="latest">{{ t('quickConnect.latest') }}</option><option v-for="version in ccSwitchVersions" :key="version.tag_name" :value="version.tag_name">{{ version.name || version.tag_name }}</option></datalist>
              <div><p class="mb-1 text-sm text-gray-600 dark:text-gray-300">{{ t('quickConnect.architecture') }}</p><div class="flex gap-2"><button v-for="arch in ccSwitchArchitectures" :key="arch.id" type="button" :aria-pressed="activeArch === arch.id" :class="['btn min-h-10', activeArch === arch.id ? 'btn-primary' : 'btn-secondary']" :data-testid="`cc-switch-arch-${arch.id}`" @click="activeArch = arch.id">{{ arch.label }}</button></div></div>
            </div>
            <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ t('quickConnect.downloadHelp') }}</p>
            <a :href="CC_SWITCH_RELEASE_URL" target="_blank" rel="noopener noreferrer" class="mt-2 inline-block text-sm text-primary-600 underline dark:text-primary-400" data-testid="cc-switch-release-fallback">{{ t('quickConnect.releases') }}</a>
          </details>
          <p v-if="ccSwitchDownloadStatus === 'error'" class="mt-3 text-sm text-red-600 dark:text-red-400" role="alert" data-testid="cc-switch-download-error">{{ t('quickConnect.downloadFailed') }} <a :href="CC_SWITCH_RELEASE_URL" target="_blank" rel="noopener noreferrer" class="underline">{{ t('quickConnect.releases') }}</a></p>
        </div>
      </div>
      <div v-if="isCnOaiSetup" class="mt-5 rounded-xl border border-primary-200 bg-primary-50 p-4 dark:border-primary-800 dark:bg-primary-950/30" data-testid="cn-oai-setup">
        <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ showInstallHelp ? '3.3 · ' : '' }}{{ t('quickConnect.cnOaiTitle') }}</h3>
        <p class="mt-1 text-sm text-gray-600 dark:text-gray-300">{{ t('quickConnect.cnOaiHint') }}</p>
        <template v-if="activeOs === 'windows'">
          <ol class="mt-3 list-decimal space-y-2 pl-5 text-sm text-gray-600 dark:text-gray-300" data-testid="cn-oai-run-steps">
            <li>{{ t('quickConnect.cnOaiRun') }}</li>
            <li>{{ t('quickConnect.cnOaiBackup') }}</li>
            <li>{{ t('quickConnect.cnOaiRestart') }}</li>
          </ol>
          <button type="button" class="btn btn-primary mt-4 min-h-11 w-full whitespace-normal px-5 sm:w-auto" :disabled="!canImport" data-testid="download-cn-oai-script" @click="downloadScript(true)"><Icon name="download" size="sm" />{{ t('quickConnect.cnOaiDownload') }}</button>
        </template>
        <p v-else class="mt-3 text-sm text-amber-700 dark:text-amber-300" data-testid="cn-oai-windows-only">{{ t('quickConnect.cnOaiWindows') }}</p>
        <p v-if="setupError" class="mt-3 text-sm text-red-600 dark:text-red-400" role="alert">{{ setupError }}</p>
      </div>
      <div v-else class="mt-5 rounded-xl border border-primary-200 bg-primary-50 p-4 dark:border-primary-800 dark:bg-primary-950/30">
        <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ showInstallHelp ? '3.3 · ' : '' }}{{ t('quickConnect.importTitle', { app: appLabel }) }}</h3>
        <p class="mt-1 text-sm text-gray-600 dark:text-gray-300">{{ t('quickConnect.importHint') }}</p>
        <button type="button" class="btn btn-primary mt-4 min-h-11 w-full whitespace-normal px-5 sm:w-auto" :disabled="!canImport" data-testid="guide-open-ccswitch" data-quick-connect-import="guide" @click="openCcSwitch"><Icon name="upload" size="sm" />{{ t(importStatus === 'idle' ? 'quickConnect.importButton' : 'quickConnect.importAgain') }}</button>
        <div v-if="importStatus !== 'idle'" class="mt-4 rounded-lg bg-white/75 p-3 text-sm dark:bg-dark-800" role="status" aria-live="polite" data-testid="connect-import-status">
          <p class="font-medium text-gray-900 dark:text-white">{{ t(importStatus === 'error' ? 'quickConnect.importError' : importStatus === 'uncertain' ? 'quickConnect.importUncertain' : 'quickConnect.importPending') }}</p>
          <p v-if="importStatus !== 'error'" class="mt-1 text-gray-600 dark:text-gray-300">{{ t(importStatus === 'uncertain' ? 'quickConnect.importUncertainHint' : 'quickConnect.importPendingHint') }}</p>
          <button v-if="importStatus === 'uncertain' || importStatus === 'error'" type="button" class="mt-2 font-medium text-primary-600 underline dark:text-primary-400" @click="showInstallHelp = true">{{ t('quickConnect.showInstall') }}</button>
        </div>
      </div>
      <div class="mt-5 border-l-2 border-gray-200 pl-4 dark:border-dark-600" data-testid="connect-verify">
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('quickConnect.verifyTitle') }}</h3>
        <p class="mt-1 text-sm text-gray-600 dark:text-gray-300">{{ t(isCnOaiSetup ? 'quickConnect.cnOaiVerify' : 'quickConnect.verifyHint', { app: appLabel }) }}</p>
        <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ t('quickConnect.verifyNote') }}</p>
      </div>
    </section>

    <details class="rounded-xl border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800" data-testid="connect-advanced">
      <summary class="cursor-pointer text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('quickConnect.advanced') }}</summary>
      <p class="mt-3 text-sm text-gray-500 dark:text-gray-400">{{ t('quickConnect.advancedHint') }}</p>
      <div class="mt-4 space-y-4">
        <label v-if="canImport && !isCnOaiSetup && selectedImportConfig?.model" class="block max-w-md text-sm text-gray-600 dark:text-gray-300"><span class="mb-1 block">{{ t('quickConnect.model') }}</span><input v-model="selectedModel" class="input w-full" :placeholder="selectedImportConfig.model || t('quickConnect.modelDefault')" data-testid="connect-model" /><span class="mt-1 block text-xs">{{ t('quickConnect.modelHint') }}</span></label>
        <div v-if="canImport && !isCnOaiSetup && selectedImportConfig" class="text-sm text-gray-500 dark:text-gray-400"><p>{{ t('quickConnect.endpoint') }}</p><code class="mt-1 block break-all text-xs">{{ selectedImportConfig.endpoint }}</code></div>
        <button type="button" class="btn btn-secondary" :disabled="!canUseKey" data-testid="connect-manual-config" @click="showManualConfig = true">{{ t('quickConnect.manualConfig') }}</button>
        <template v-if="selectedApp === 'codex' && currentKey && currentKeyCompatible">
          <label v-if="isCnOaiSetup" class="block max-w-md text-sm text-gray-600 dark:text-gray-300"><span class="mb-1 block">{{ t('quickConnect.cnOaiModel') }}</span><input v-model="cnOaiModel" class="input w-full" data-testid="cn-oai-model" /></label>
          <div v-else class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900/50" data-testid="connect-codex-script">
            <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('quickConnect.scriptTitle') }}</h3>
            <p class="mt-1 text-sm text-gray-600 dark:text-gray-300">{{ t('quickConnect.scriptHint') }}</p>
            <div class="mt-3 flex flex-wrap gap-2"><button type="button" class="btn btn-secondary" :disabled="!canImport" data-testid="download-codex-script" @click="downloadScript(false)">{{ t('quickConnect.downloadScript') }}</button><button type="button" class="btn btn-secondary" :disabled="!canImport" data-testid="copy-codex-script" @click="copyScript">{{ t(copyStatus === 'success' ? 'quickConnect.copied' : copyStatus === 'error' ? 'quickConnect.copyFailed' : 'quickConnect.copyScript') }}</button></div>
            <p class="mt-3 text-sm text-gray-500 dark:text-gray-400">{{ t('quickConnect.runScript') }}</p><code class="mt-1 block overflow-x-auto rounded-lg bg-gray-100 p-3 text-xs dark:bg-dark-800">{{ runCommand }}</code>
            <details class="mt-3"><summary class="cursor-pointer text-sm text-gray-600 dark:text-gray-300">{{ t('quickConnect.scriptPreview') }}</summary><pre class="mt-2 max-h-64 overflow-auto rounded-lg bg-gray-100 p-3 text-xs dark:bg-dark-800">{{ scriptPreview }}</pre></details>
          </div>
        </template>
        <p v-if="setupError && !isCnOaiSetup" class="text-sm text-red-600 dark:text-red-400" role="alert">{{ setupError }}</p>
      </div>
    </details>
    <UseKeyModal v-if="currentKey && showManualConfig" :show="showManualConfig" :api-key="currentKey.key" :base-url="baseUrl" :platform="effectivePlatform" :allow-messages-dispatch="currentKey.group?.allow_messages_dispatch" @close="showManualConfig = false" />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onUnmounted, ref, useSlots, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import CcSwitchAppIcon from './CcSwitchAppIcon.vue'
import UseKeyModal from './UseKeyModal.vue'
import Select from '@/components/common/Select.vue'
import { buildCnOaiSetupScript, CN_OAI_SETUP_FILENAME, isCnOaiGroup } from '@/utils/cnOaiSetup'
import { CC_SWITCH_APP_CATALOG, buildCcSwitchImportDeeplink, CC_SWITCH_USAGE_SCRIPT, resolveCcSwitchImportConfig, type CcSwitchAppType } from '@/utils/ccswitchImport'
import type { ApiKey, GroupPlatform } from '@/types'
import { useClipboard } from '@/composables/useClipboard'
import { buildCCSwitchDirectDownloadURL, listCCSwitchVersions, resolveCCSwitchDownload, startCCSwitchDownload, type CCSwitchArchitecture, type CCSwitchReleaseVersion } from '@/api/downloads'
import { buildCodexSetupScript, buildCodexSetupScriptPreview, getCodexSetupFilename, isCodexOneClickEligible, type CodexOperatingSystem } from '@/utils/codexOneClick'
import { supportsAutomaticConfig } from '@/custom/vote-ai/quick-connect/auto-config'

type AccessMethod = 'guide' | 'ccswitch' | 'script' | 'cn-oai'
type QuickConnectKey = Pick<ApiKey, 'id' | 'name' | 'key' | 'status' | 'group' | 'group_id'>
const props = withDefaults(defineProps<{
  show?: boolean
  apiKey: string
  keyName: string
  baseUrl: string
  providerName: string
  platform?: GroupPlatform | null
  defaultApp?: CcSwitchAppType
  initialMethod?: AccessMethod
  availableKeys?: ApiKey[]
  initialKeyId?: number | null
  loading?: boolean
  loadError?: boolean
  creationBusy?: boolean
}>(), { show: true, loading: false, loadError: false, creationBusy: false })
const emit = defineEmits<{ (event: 'protocol-failed'): void; (event: 'manage-keys'): void; (event: 'retry'): void; (event: 'key-ready', ready: boolean): void }>()
const slots = useSlots()
const createKeyOpen = ref(false)
function closeCreateKey() { if (!props.creationBusy) createKeyOpen.value = false }
const { t } = useI18n()
const { copyToClipboard: clipboardCopy } = useClipboard()
const selectedKeyId = ref<number | null>(props.initialKeyId ?? null)
const selectedApp = ref<CcSwitchAppType>(props.defaultApp || 'codex')
const appChosenByUser = ref(false)
const selectedModel = ref('')
const cnOaiModel = ref('')
const activeOs = ref<CodexOperatingSystem>('windows')
const activeArch = ref<CCSwitchArchitecture>('amd64')
const showInstallHelp = ref(true)
const showManualConfig = ref(false)
const importStatus = ref<'idle' | 'pending' | 'uncertain' | 'error'>('idle')
const ccSwitchDownloadStatus = ref<'idle' | 'loading' | 'error'>('idle')
const ccSwitchVersion = ref('latest')
const ccSwitchVersions = ref<CCSwitchReleaseVersion[]>([])
const copyStatus = ref<'idle' | 'success' | 'error'>('idle')
const setupError = ref('')
const PROTOCOL_FAILURE_DELAY_MS = 1800
let protocolCheckTimer: ReturnType<typeof setTimeout> | null = null
let protocolListenersActive = false
let ccSwitchDownloadController: AbortController | null = null
let ccSwitchDownloadRequestId = 0
let ccSwitchVersionController: AbortController | null = null
let ccSwitchVersionRequestId = 0

const CODEX_DOWNLOAD_URLS: Record<CodexOperatingSystem, string> = {
  windows: 'https://get.microsoft.com/installer/download/9PLM9XGG6VKS?cid=website_cta_psi',
  macos: 'https://persistent.oaistatic.com/codex-app-prod/Codex.dmg',
  linux: 'https://learn.chatgpt.com/docs/codex/cli'
}
const CC_SWITCH_RELEASE_URL = 'https://github.com/farion1231/cc-switch/releases/latest'
const CC_SWITCH_CLIENT_INSTALL_URLS: Record<CcSwitchAppType, string> = {
  claude: 'https://docs.anthropic.com/en/docs/claude-code/setup',
  'claude-desktop': 'https://claude.ai/download', codex: '',
  gemini: 'https://github.com/google-gemini/gemini-cli', grokbuild: 'https://x.ai/cli',
  opencode: 'https://opencode.ai/download', openclaw: 'https://docs.openclaw.ai/install',
  hermes: 'https://hermes-agent.nousresearch.com/docs/getting-started/installation/', pi: 'https://github.com/badlogic/pi-mono'
}
const operatingSystems: Array<{ id: CodexOperatingSystem; label: string }> = [{ id: 'macos', label: 'macOS' }, { id: 'windows', label: 'Windows' }, { id: 'linux', label: 'Linux' }]
const ccSwitchArchitectures: Array<{ id: CCSwitchArchitecture; label: string }> = [{ id: 'amd64', label: 'x64' }, { id: 'arm64', label: 'ARM64' }]
const selectableKeys = computed(() => (props.availableKeys || []).filter(isCodexOneClickEligible))
const showCreateKey = computed(() => !!slots['create-key'] && !props.loading && !props.loadError && (createKeyOpen.value || !selectableKeys.value.length))
// Creating from an application's guide is an explicit choice of that app.
// Do not change it implicitly when the new key's group arrives.
watch(showCreateKey, open => { if (open) appChosenByUser.value = true }, { immediate: true })
// A supplied list is authoritative, including an empty or newly filtered list.
// Legacy standalone callers must provide an explicit platform to use raw props.
const fallbackKey = computed<QuickConnectKey | null>(() => props.availableKeys === undefined && props.apiKey.trim() && props.platform
  ? { id: props.initialKeyId ?? -1, name: props.keyName, key: props.apiKey, status: 'active', group_id: -1, group: { platform: props.platform } as ApiKey['group'] }
  : null)
const currentKey = computed<QuickConnectKey | null>(() => selectableKeys.value.find((key) => key.id === selectedKeyId.value) || fallbackKey.value)
const singleSelectedKey = computed(() => !!currentKey.value && (props.availableKeys === undefined || (selectableKeys.value.length === 1 && selectableKeys.value[0].id === currentKey.value.id)))
function keySuffix(key: string): string { return key.length > 4 ? key.slice(-4) : '••••' }
const keyOptions = computed(() => selectableKeys.value.map(key => ({
  value: key.id,
  name: key.name || `#${key.id}`,
  group: key.group?.name || key.group?.platform || '',
  suffix: keySuffix(key.key),
  label: `${key.name || `#${key.id}`} · ${key.group?.name || ''} · ${keySuffix(key.key)}`,
  disabled: !isKeyCompatible(key, selectedApp.value),
})))
function chooseExistingKey(value: string | number | boolean | null): void {
  if (typeof value !== 'number' || props.creationBusy) return
  if (keyOptions.value.some(option => option.value === value && !option.disabled)) selectedKeyId.value = value
}
const selectedUnavailable = computed(() => !currentKey.value && selectedKeyId.value !== null)
const effectivePlatform = computed(() => currentKey.value?.group?.platform || null)
const selectedImportConfig = computed(() => effectivePlatform.value ? resolveCcSwitchImportConfig(effectivePlatform.value, 'claude', props.baseUrl, selectedApp.value) : null)
const ccSwitchApps = computed(() => CC_SWITCH_APP_CATALOG.map((app) => ({ ...app, importable: resolveCcSwitchImportConfig(effectivePlatform.value, 'claude', props.baseUrl, app.id).importable })))
const primaryApps = computed(() => ['codex', 'claude', 'opencode'].map((id) => ccSwitchApps.value.find((app) => app.id === id)!))
const moreApps = computed(() => ccSwitchApps.value.filter((app) => !['codex', 'claude', 'opencode'].includes(app.id)))
const isMoreAppSelected = computed(() => !['codex', 'claude', 'opencode'].includes(selectedApp.value))
const appLabel = computed(() => CC_SWITCH_APP_CATALOG.find((app) => app.id === selectedApp.value)?.label || selectedApp.value)
const canUseKey = computed(() => props.show && !props.loading && !props.loadError && !showCreateKey.value && !!currentKey.value)
watch(canUseKey, ready => emit('key-ready', ready), { immediate: true })
watch(() => props.initialKeyId, () => { createKeyOpen.value = false })
const currentKeyCompatible = computed(() => !!currentKey.value && isKeyCompatible(currentKey.value, selectedApp.value))
const canImport = computed(() => canUseKey.value && currentKeyCompatible.value && !!selectedImportConfig.value?.importable)
const isCnOaiSetup = computed(() => selectedApp.value === 'codex' && isCnOaiGroup(currentKey.value?.group))
const installStep = computed(() => ({
  url: selectedApp.value === 'codex' ? CODEX_DOWNLOAD_URLS[activeOs.value] : CC_SWITCH_CLIENT_INSTALL_URLS[selectedApp.value],
  button: selectedApp.value === 'codex' && activeOs.value !== 'linux' ? t('quickConnect.downloadApp', { app: 'Codex' }) : t('quickConnect.openAppGuide')
}))
const scriptPreview = computed(() => buildCodexSetupScriptPreview(activeOs.value, props.baseUrl))
const runCommand = computed(() => activeOs.value === 'windows' ? `powershell -ExecutionPolicy Bypass -File "$env:USERPROFILE\\Downloads\\${getCodexSetupFilename(activeOs.value)}"` : `sh ~/Downloads/${getCodexSetupFilename(activeOs.value)}`)

function syncSelectedKey(preferInitial = false): void {
  if (preferInitial && props.initialKeyId != null) {
    selectedKeyId.value = props.initialKeyId
    return
  }
  // Do not silently replace a removed, invalid, or explicitly requested key.
  if (selectedKeyId.value !== null) return
  const matchingPropKey = selectableKeys.value.find((key) => key.key === props.apiKey)
  selectedKeyId.value = matchingPropKey?.id ?? selectableKeys.value[0]?.id ?? null
}
function resetImport(): void {
  clearProtocolCheck()
  importStatus.value = 'idle'
  setupError.value = ''
  copyStatus.value = 'idle'
  showManualConfig.value = false
}
function selectApp(app: CcSwitchAppType): void {
  if (props.creationBusy) return
  if (ccSwitchApps.value.find((item) => item.id === app)?.importable) {
    appChosenByUser.value = true
    selectedApp.value = app
  }
}

// The protocol catalog describes deeplink support, not model compatibility.
// Only offer configurations that the unchanged import generator can represent.
// Broader routed Codex and provider-specific OpenCode setups remain in manual setup.
function isKeyCompatible(key: QuickConnectKey, app: CcSwitchAppType): boolean {
  return supportsAutomaticConfig(key.group, app)
}

watch(() => props.show, (show) => {
  if (show) {
    syncSelectedKey(true)
    showInstallHelp.value = props.initialMethod !== 'ccswitch'
    resetImport()
    void loadCCSwitchVersions()
  } else {
    resetImport()
    cancelCcSwitchDownload()
    cancelCCSwitchVersionLoad()
  }
}, { immediate: true })
watch(() => [props.initialKeyId, props.apiKey], () => { if (props.show) syncSelectedKey(true) })
watch(() => props.availableKeys, () => { if (props.show) syncSelectedKey() })
watch(currentKey, () => {
  resetImport()
  selectedModel.value = ''
  cnOaiModel.value = ''
  if (!effectivePlatform.value || appChosenByUser.value) return
  const preferred = props.defaultApp || resolveCcSwitchImportConfig(effectivePlatform.value, 'claude', props.baseUrl).requestedApp
  if (preferred && ccSwitchApps.value.find((app) => app.id === preferred)?.importable) selectedApp.value = preferred
}, { immediate: true })
watch(selectedApp, () => { resetImport(); selectedModel.value = ''; cnOaiModel.value = '' })
watch(() => [props.baseUrl, props.providerName], resetImport)
watch(activeOs, () => { resetImport(); cancelCcSwitchDownload() })
watch(activeArch, cancelCcSwitchDownload)
watch(ccSwitchVersion, cancelCcSwitchDownload)

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
}
async function loadCCSwitchVersions(): Promise<void> {
  cancelCCSwitchVersionLoad()
  const requestId = ++ccSwitchVersionRequestId
  const controller = new AbortController()
  ccSwitchVersionController = controller
  try {
    const result = await listCCSwitchVersions(20, controller.signal)
    if (requestId !== ccSwitchVersionRequestId || !props.show) return
    ccSwitchVersions.value = result.versions || []
  } catch {
    // Downloads and the official releases link remain usable without this list.
    if (requestId === ccSwitchVersionRequestId && props.show) ccSwitchVersions.value = []
  } finally {
    if (requestId === ccSwitchVersionRequestId) ccSwitchVersionController = null
  }
}
async function downloadCcSwitch(): Promise<void> {
  if (ccSwitchDownloadStatus.value === 'loading' || !props.show) return
  const requestedOs = activeOs.value
  const requestedArch = activeArch.value
  const requestedVersion = ccSwitchVersion.value.trim()
  const version = requestedVersion && requestedVersion.toLowerCase() !== 'latest' ? requestedVersion : undefined
  const requestId = ++ccSwitchDownloadRequestId
  const controller = new AbortController()
  ccSwitchDownloadController = controller
  ccSwitchDownloadStatus.value = 'loading'
  try {
    if (version) await resolveCCSwitchDownload(requestedOs, requestedArch, version, controller.signal)
    else await resolveCCSwitchDownload(requestedOs, requestedArch, controller.signal)
    if (requestId !== ccSwitchDownloadRequestId || !props.show || activeOs.value !== requestedOs || activeArch.value !== requestedArch || ccSwitchVersion.value.trim() !== requestedVersion) return
    startCCSwitchDownload(buildCCSwitchDirectDownloadURL(requestedOs, requestedArch, version))
    ccSwitchDownloadStatus.value = 'idle'
  } catch {
    if (requestId === ccSwitchDownloadRequestId && props.show) ccSwitchDownloadStatus.value = 'error'
  } finally {
    if (requestId === ccSwitchDownloadRequestId) ccSwitchDownloadController = null
  }
}
function handleOsKeydown(event: KeyboardEvent, current: CodexOperatingSystem): void {
  const index = operatingSystems.findIndex((os) => os.id === current)
  const targetIndex = event.key === 'Home' ? 0 : event.key === 'End' ? operatingSystems.length - 1 : ['ArrowRight', 'ArrowDown'].includes(event.key) ? (index + 1) % operatingSystems.length : ['ArrowLeft', 'ArrowUp'].includes(event.key) ? (index - 1 + operatingSystems.length) % operatingSystems.length : null
  if (targetIndex === null) return
  event.preventDefault()
  activeOs.value = operatingSystems[targetIndex].id
  void nextTick(() => document.getElementById(`guide-os-radio-${activeOs.value}`)?.focus())
}
function removeProtocolCheckListeners(): void {
  if (!protocolListenersActive) return
  window.removeEventListener('blur', clearProtocolCheck)
  document.removeEventListener('visibilitychange', handleProtocolVisibilityChange)
  protocolListenersActive = false
}
function clearProtocolCheck(): void {
  if (protocolCheckTimer) clearTimeout(protocolCheckTimer)
  protocolCheckTimer = null
  removeProtocolCheckListeners()
}
function handleProtocolVisibilityChange(): void {
  if (document.visibilityState === 'hidden') clearProtocolCheck()
}
function startProtocolCheck(): void {
  clearProtocolCheck()
  window.addEventListener('blur', clearProtocolCheck)
  document.addEventListener('visibilitychange', handleProtocolVisibilityChange)
  protocolListenersActive = true
  protocolCheckTimer = setTimeout(() => {
    protocolCheckTimer = null
    removeProtocolCheckListeners()
    if (props.show) {
      importStatus.value = 'uncertain'
      emit('protocol-failed')
    }
  }, PROTOCOL_FAILURE_DELAY_MS)
}
function openCcSwitch(): void {
  if (!canImport.value || !currentKey.value || !effectivePlatform.value || isCnOaiSetup.value) return
  const deeplink = buildCcSwitchImportDeeplink({
    baseUrl: props.baseUrl, platform: effectivePlatform.value, clientType: 'claude', app: selectedApp.value,
    model: selectedModel.value.trim() || selectedImportConfig.value?.model || undefined,
    providerName: props.providerName, apiKey: currentKey.value.key, usageScript: CC_SWITCH_USAGE_SCRIPT
  })
  importStatus.value = 'pending'
  startProtocolCheck()
  try { window.open(deeplink, '_self') } catch {
    clearProtocolCheck()
    importStatus.value = 'error'
    emit('protocol-failed')
  }
}
function downloadScript(dedicated: boolean): void {
  if (!canImport.value || !currentKey.value || selectedApp.value !== 'codex') return
  if (dedicated && (!isCnOaiSetup.value || activeOs.value !== 'windows')) return
  if (!dedicated && isCnOaiSetup.value) return
  setupError.value = ''
  let content: string
  try {
    content = dedicated
      ? buildCnOaiSetupScript({ baseUrl: props.baseUrl, apiKey: currentKey.value.key, providerName: props.providerName, groupId: currentKey.value.group!.id, model: cnOaiModel.value })
      : buildCodexSetupScript(activeOs.value, props.baseUrl, currentKey.value.key)
  } catch { setupError.value = t('keys.oneClick.cnOaiGenerateFailed'); return }
  const url = URL.createObjectURL(new Blob([content], { type: 'text/plain;charset=utf-8' }))
  try {
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = dedicated ? CN_OAI_SETUP_FILENAME : getCodexSetupFilename(activeOs.value)
    anchor.click()
  } finally { setTimeout(() => URL.revokeObjectURL(url), 1000) }
}
async function copyScript(): Promise<void> {
  if (!canImport.value || !currentKey.value || selectedApp.value !== 'codex' || isCnOaiSetup.value) return
  const key = currentKey.value.key
  const os = activeOs.value
  const success = await clipboardCopy(buildCodexSetupScript(os, props.baseUrl, key), t('keys.oneClick.scriptCopied'))
  if (props.show && currentKey.value?.key === key && activeOs.value === os) copyStatus.value = success ? 'success' : 'error'
}
onUnmounted(() => { clearProtocolCheck(); cancelCcSwitchDownload(); cancelCCSwitchVersionLoad() })
</script>

<style scoped>
.connect-key-picker :deep(.select-trigger) {
  min-height: 72px;
  padding: 12px;
  border-radius: 12px;
}
.connect-key-picker :deep(.select-value) { min-width: 0; }
</style>
