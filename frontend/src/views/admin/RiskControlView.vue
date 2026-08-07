<template>
  <AppLayout>
    <div class="space-y-6">
      <div v-if="loading" class="flex items-center justify-center py-16">
        <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></div>
      </div>

      <template v-else>
        <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
          <div>
            <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.title') }}</h1>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.description') }}</p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <button type="button" class="btn btn-secondary inline-flex items-center gap-2" :disabled="statusLoading" @click="loadStatus(false)">
              <Icon name="refresh" size="sm" :class="statusLoading ? 'animate-spin' : ''" />
              {{ t('admin.riskControl.refreshStatus') }}
            </button>
            <button type="button" class="btn btn-primary inline-flex items-center gap-2" @click="openSettings">
              <Icon name="cog" size="sm" />
              {{ t('admin.riskControl.openSettings') }}
            </button>
          </div>
        </div>

        <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-4">
          <div
            v-for="item in overviewItems"
            :key="item.key"
            class="rounded-lg border border-gray-100 bg-white px-4 py-3 shadow-sm dark:border-dark-700 dark:bg-dark-800"
          >
            <div class="flex min-w-0 items-center gap-3">
              <div class="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg" :class="item.iconClass">
                <Icon :name="item.icon" size="sm" />
              </div>
              <div class="min-w-0 flex-1">
                <div class="flex min-w-0 items-center justify-between gap-2">
                  <p class="truncate text-xs font-medium text-gray-500 dark:text-gray-400">{{ item.label }}</p>
                  <span
                    v-if="item.badge"
                    class="inline-flex flex-shrink-0 items-center rounded-full px-2 py-0.5 text-xs font-medium"
                    :class="item.badgeClass"
                  >
                    {{ item.badge }}
                  </span>
                </div>
                <div class="mt-1 flex min-w-0 items-baseline gap-2">
                  <p class="truncate text-xl font-semibold leading-7 text-gray-900 dark:text-white">{{ item.value }}</p>
                  <p v-if="item.meta" class="truncate text-xs text-gray-500 dark:text-gray-400">{{ item.meta }}</p>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div
          v-if="showPreBlockRuntimeCard"
          data-test="pre-block-runtime-cards"
          class="grid grid-cols-1 gap-6 xl:grid-cols-[minmax(0,520px)_minmax(0,1fr)]"
        >
          <div data-test="pre-block-sync-card" class="card">
            <div class="flex flex-col gap-4 border-b border-gray-100 px-6 py-4 dark:border-dark-700 lg:flex-row lg:items-center lg:justify-between">
              <div>
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.preBlockSyncStatus') }}</h2>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.preBlockSyncHint') }}</p>
              </div>
              <span class="inline-flex w-fit items-center rounded-full bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-600 dark:bg-dark-700 dark:text-gray-300">
                {{ modeLabel(status?.mode ?? configForm.mode) }}
              </span>
            </div>

            <div class="p-6">
              <div data-test="pre-block-metric-grid" class="grid grid-cols-2 gap-3 md:grid-cols-3">
                <div
                  v-for="item in preBlockMetricItems"
                  :key="item.key"
                  class="rounded-lg p-4"
                  :class="item.class"
                >
                  <p class="text-xs text-gray-500 dark:text-gray-400">{{ item.label }}</p>
                  <p class="mt-2 truncate text-2xl font-semibold leading-8" :class="item.valueClass">{{ item.value }}</p>
                  <p v-if="item.meta" class="mt-1 truncate text-xs text-gray-500 dark:text-gray-400">{{ item.meta }}</p>
                </div>
              </div>
            </div>
          </div>

          <div data-test="pre-block-api-key-load-card" class="card">
            <div class="flex flex-col gap-4 border-b border-gray-100 px-6 py-4 dark:border-dark-700 lg:flex-row lg:items-center lg:justify-between">
              <div>
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.preBlockAPIKeyLoad') }}</h2>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                  {{ t('admin.riskControl.preBlockAPIKeyLoadHint') }}
                </p>
              </div>
              <span class="inline-flex w-fit items-center rounded-full bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-600 dark:bg-dark-700 dark:text-gray-300">
                {{ preBlockAPIKeyLoadSummaryText }}
              </span>
            </div>

            <div class="p-6">
              <div
                v-if="preBlockAPIKeyLoads.length > 0"
                data-test="pre-block-api-key-load-list"
                class="max-h-[280px] space-y-3 overflow-y-auto pr-1"
              >
                <div
                  v-for="item in preBlockAPIKeyLoads"
                  :key="item.key_hash || item.index"
                  class="rounded-lg bg-gray-50 p-3 dark:bg-dark-700/50"
                >
                  <div class="flex min-w-0 flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                    <div class="min-w-0">
                      <div class="flex min-w-0 items-center gap-2">
                        <span class="font-mono text-sm font-semibold text-gray-900 dark:text-white">#{{ item.index + 1 }}</span>
                        <span class="truncate font-mono text-sm text-gray-700 dark:text-gray-200">{{ item.masked || '-' }}</span>
                        <span class="h-2 w-2 flex-shrink-0 rounded-full" :class="apiKeyStatusDotClass(item.status)"></span>
                      </div>
                      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                        {{ t('admin.riskControl.preBlockAPIKeyTotals', { total: formatNumber(item.total), success: formatNumber(item.success), errors: formatNumber(item.errors) }) }}
                      </p>
                    </div>
                    <div class="grid grid-cols-4 gap-2 text-right text-xs text-gray-500 dark:text-gray-400 sm:min-w-[280px]">
                      <div>
                        <p>{{ t('admin.riskControl.preBlockKeyActiveShort') }}</p>
                        <p class="mt-1 text-sm font-semibold text-sky-700 dark:text-sky-300">{{ formatNumber(item.active) }}</p>
                      </div>
                      <div>
                        <p>{{ t('admin.riskControl.preBlockKeyTotalShort') }}</p>
                        <p class="mt-1 text-sm font-semibold text-gray-900 dark:text-white">{{ formatNumber(item.total) }}</p>
                      </div>
                      <div>
                        <p>{{ t('admin.riskControl.preBlockKeyAvgShort') }}</p>
                        <p class="mt-1 text-sm font-semibold text-gray-900 dark:text-white">{{ formatNumber(item.avg_latency_ms) }} ms</p>
                      </div>
                      <div>
                        <p>{{ t('admin.riskControl.preBlockKeyLastShort') }}</p>
                        <p class="mt-1 text-sm font-semibold text-gray-900 dark:text-white">{{ formatNumber(item.last_latency_ms) }} ms</p>
                      </div>
                    </div>
                  </div>
                  <div class="mt-3 h-1.5 overflow-hidden rounded-full bg-white dark:bg-dark-900">
                    <div class="h-full rounded-full bg-sky-500" :style="{ width: preBlockAPIKeyLoadWidth(item.total) }"></div>
                  </div>
                </div>
              </div>
              <p v-else class="rounded-lg bg-gray-50 p-4 text-sm text-gray-500 dark:bg-dark-700/50 dark:text-gray-400">
                {{ t('admin.riskControl.preBlockAPIKeyLoadEmpty') }}
              </p>
            </div>
          </div>
        </div>

        <div v-if="showWorkerRuntimeCard" class="card">
          <div class="flex flex-col gap-4 border-b border-gray-100 px-6 py-4 dark:border-dark-700 lg:flex-row lg:items-center lg:justify-between">
            <div>
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.workerStatus') }}</h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.workerStatusHint') }}</p>
            </div>
            <div class="flex flex-wrap items-center gap-2 text-sm text-gray-500 dark:text-gray-400">
              <span>{{ t('admin.riskControl.autoRefresh') }}</span>
              <span v-if="status?.last_cleanup_at">
                {{ t('admin.riskControl.lastCleanup', { time: formatDateTime(status.last_cleanup_at) }) }}
              </span>
            </div>
          </div>

          <div class="grid grid-cols-1 gap-6 p-6 xl:grid-cols-[minmax(0,360px)_1fr]">
            <div class="space-y-4">
              <div class="rounded-lg border border-gray-100 p-4 dark:border-dark-700">
                <div class="flex items-center justify-between gap-3">
                  <div>
                    <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.queueUsage') }}</p>
                    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                      {{ formatNumber(status?.queue_length ?? 0) }} / {{ formatNumber(status?.queue_size ?? configForm.queue_size) }}
                    </p>
                  </div>
                  <span class="text-sm font-semibold text-gray-900 dark:text-white">{{ queueUsagePercent }}</span>
                </div>
                <div class="mt-4 h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                  <div class="h-full rounded-full bg-primary-500 transition-all duration-300" :style="queueUsageStyle"></div>
                </div>
              </div>

              <div class="grid grid-cols-2 gap-3">
                <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-700/50">
                  <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.activeWorkers') }}</p>
                  <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ status?.active_workers ?? 0 }}</p>
                </div>
                <div class="rounded-lg bg-emerald-50 p-4 dark:bg-emerald-900/10">
                  <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.idleWorkers') }}</p>
                  <p class="mt-2 text-2xl font-semibold text-emerald-700 dark:text-emerald-300">{{ status?.idle_workers ?? configForm.worker_count }}</p>
                </div>
                <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-700/50">
                  <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.processed') }}</p>
                  <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ formatNumber(status?.processed ?? 0) }}</p>
                </div>
                <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-700/50">
                  <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.droppedErrors') }}</p>
                  <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ formatNumber((status?.dropped ?? 0) + (status?.errors ?? 0)) }}</p>
                </div>
              </div>
            </div>

            <div>
              <div class="mb-3 flex items-center justify-between gap-3">
                <div>
                  <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.workerPool') }}</p>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                    {{ t('admin.riskControl.workerPoolMeta', { active: status?.active_workers ?? 0, idle: status?.idle_workers ?? configForm.worker_count, total: status?.worker_count ?? configForm.worker_count }) }}
                  </p>
                </div>
                <span class="inline-flex items-center rounded-full bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-600 dark:bg-dark-700 dark:text-gray-300">
                  {{ modeLabel(status?.mode ?? configForm.mode) }}
                </span>
              </div>
              <div class="grid grid-cols-2 gap-2 sm:grid-cols-4 md:grid-cols-6 xl:grid-cols-8 2xl:grid-cols-10">
                <div
                  v-for="worker in workerSlots"
                  :key="worker.id"
                  class="flex h-12 items-center justify-between rounded-lg border px-3 transition-colors"
                  :class="workerSlotClass(worker.state)"
                  :title="worker.label"
                >
                  <span class="text-sm font-semibold">#{{ worker.id }}</span>
                  <span class="h-2.5 w-2.5 rounded-full" :class="workerDotClass(worker.state)"></span>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div class="card">
          <div class="flex flex-col gap-4 border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
              <div>
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.records') }}</h2>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.recordsHint') }}</p>
              </div>
              <button type="button" class="btn btn-secondary inline-flex items-center gap-2" :disabled="logsLoading" @click="loadLogs">
                <Icon name="refresh" size="sm" :class="logsLoading ? 'animate-spin' : ''" />
                {{ t('admin.riskControl.refresh') }}
              </button>
            </div>

            <div class="flex flex-col gap-2 rounded-lg border border-gray-100 bg-gray-50 px-3 py-2 dark:border-dark-700 dark:bg-dark-900/30 sm:flex-row sm:items-center sm:justify-between">
              <div class="flex min-w-0 items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
                <Icon name="filter" size="sm" class="flex-shrink-0 text-gray-400" />
                <span class="font-medium">{{ t('admin.riskControl.modelFilter') }}</span>
                <span class="truncate text-gray-500 dark:text-gray-400">{{ modelFilterSummary }}</span>
              </div>
              <div v-if="modelFilterPreviewModels.length > 0" class="flex flex-wrap gap-1.5">
                <span
                  v-for="model in modelFilterPreviewModels"
                  :key="model"
                  class="inline-flex max-w-[180px] items-center truncate rounded-md bg-white px-2 py-1 font-mono text-xs text-gray-600 shadow-sm dark:bg-dark-800 dark:text-gray-300"
                >
                  {{ model }}
                </span>
                <span v-if="hiddenModelFilterModelCount > 0" class="inline-flex rounded-md bg-white px-2 py-1 text-xs text-gray-500 shadow-sm dark:bg-dark-800 dark:text-gray-400">
                  +{{ hiddenModelFilterModelCount }}
                </span>
              </div>
            </div>

            <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-6">
              <Select v-model="filters.result" :options="resultOptions" @change="reloadLogsFromFirstPage" />
              <Select v-model="filters.group_id" :options="groupFilterOptions" @change="reloadLogsFromFirstPage" />
              <Select v-model="filters.endpoint" :options="endpointOptions" @change="reloadLogsFromFirstPage" />
              <input v-model.trim="filters.search" type="search" class="input" :placeholder="t('admin.riskControl.filters.search')" @keyup.enter="reloadLogsFromFirstPage" />
              <input v-model="filters.from" type="datetime-local" class="input" :title="t('admin.riskControl.filters.from')" @change="reloadLogsFromFirstPage" />
              <input v-model="filters.to" type="datetime-local" class="input" :title="t('admin.riskControl.filters.to')" @change="reloadLogsFromFirstPage" />
            </div>
          </div>

          <div class="overflow-x-auto">
            <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
              <thead class="bg-gray-50 dark:bg-dark-800">
                <tr>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.time') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.group') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.user') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.apiKey') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.endpoint') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.result') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.highest') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.actionMeta') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.latency') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.input') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-800 dark:bg-dark-800">
                <tr v-if="logsLoading">
                  <td colspan="10" class="px-5 py-12 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('common.loading') }}</td>
                </tr>
                <tr v-else-if="logs.length === 0">
                  <td colspan="10" class="px-5 py-12 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.emptyLogs') }}</td>
                </tr>
                <template v-else>
                  <tr v-for="row in logs" :key="row.id" class="hover:bg-gray-50 dark:hover:bg-dark-700/60">
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">{{ formatDateTime(row.created_at) }}</td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">{{ row.group_name || '-' }}</td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
                      <div>{{ row.user_email || '-' }}</div>
                      <div v-if="row.user_id" class="text-xs text-gray-400">UID {{ row.user_id }}</div>
                    </td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">{{ row.api_key_name || '-' }}</td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
                      <div>{{ row.endpoint || '-' }}</div>
                      <div class="text-xs text-gray-400">{{ row.provider || '-' }} / {{ row.model || '-' }}</div>
                    </td>
                    <td class="whitespace-nowrap px-5 py-4">
                      <!-- CUSTOM(VOTE-AI-RISK-SIDE-EFFECTS): structured audit state; error text is diagnostic only. -->
                      <ModerationAuditStatusBadge
                        :status="row.audit_status"
                        :action="row.action"
                        :flagged="row.flagged"
                        :code="row.audit_code"
                        :retryable="row.audit_retryable"
                      />
                    </td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
                      <div>{{ row.highest_category || '-' }}</div>
                      <div class="text-xs text-gray-400">{{ percent(row.highest_score) }}</div>
                      <div v-if="row.matched_keyword" class="mt-0.5 text-xs font-medium text-red-600 dark:text-red-300" :title="t('admin.riskControl.matchedKeyword') + ': ' + row.matched_keyword">
                        {{ t('admin.riskControl.matchedKeyword') }}: {{ row.matched_keyword }}
                      </div>
                    </td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
                      <div>{{ violationCountText(row) }}</div>
                      <ModerationSideEffectsStatus
                        :side-effect-status="row.side_effect_status"
                        :notification-status="row.notification_status"
                        :error="row.side_effect_error"
                        :moderation-ban-active="row.moderation_ban_active"
                      />
                      <button
                        v-if="canUnbanRow(row)"
                        type="button"
                        class="mt-2 inline-flex items-center gap-1 rounded-md border border-emerald-200 bg-emerald-50 px-2 py-1 text-xs font-medium text-emerald-700 transition-colors hover:bg-emerald-100 disabled:cursor-not-allowed disabled:opacity-60 dark:border-emerald-900/60 dark:bg-emerald-900/20 dark:text-emerald-300 dark:hover:bg-emerald-900/30"
                        :disabled="unbanningUserID === row.user_id"
                        data-test="open-moderation-unban"
                        @click="openUnbanDialog(row)"
                      >
                        <Icon name="checkCircle" size="xs" :class="unbanningUserID === row.user_id ? 'animate-spin' : ''" />
                        {{ unbanningUserID === row.user_id ? t('common.processing') : t('admin.riskControl.unbanUser') }}
                      </button>
                      <p
                        v-else-if="unbanUnavailableReason(row)"
                        class="mt-2 max-w-56 whitespace-normal text-xs leading-4 text-amber-600 dark:text-amber-300"
                        :title="unbanUnavailableReason(row)"
                        data-test="unban-unavailable-reason"
                      >
                        {{ unbanUnavailableReason(row) }}
                      </p>
                    </td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
                      <div>{{ latencyText(auditTotalLatency(row)) }}</div>
                      <div v-if="row.queue_delay_ms !== null && row.queue_delay_ms !== undefined" class="text-xs text-gray-400">
                        {{ t('admin.riskControl.queueDelay', { ms: row.queue_delay_ms }) }}
                      </div>
                    </td>
                    <td class="w-[320px] max-w-sm px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
                      <button
                        type="button"
                        class="group flex w-full min-w-0 items-center gap-2 rounded-lg px-2 py-1.5 text-left transition-colors hover:bg-gray-100 dark:hover:bg-dark-700"
                        :title="inputSummaryText(row)"
                        data-test="open-input-detail"
                        @click="openInputDetail(row)"
                      >
                        <span class="min-w-0 flex-1 truncate">{{ inputSummaryText(row) }}</span>
                        <Icon name="eye" size="xs" class="flex-shrink-0 text-gray-300 transition-colors group-hover:text-primary-500 dark:text-gray-500" />
                      </button>
                    </td>
                  </tr>
                </template>
              </tbody>
            </table>
          </div>

          <Pagination
            v-if="pagination.total > 0"
            :page="pagination.page"
            :total="pagination.total"
            :page-size="pagination.page_size"
            @update:page="onPageChange"
            @update:pageSize="onPageSizeChange"
          />
        </div>
      </template>

      <BaseDialog :show="settingsOpen" :title="t('admin.riskControl.settingsTitle')" width="extra-wide" @close="closeSettings">
        <div class="space-y-6">
          <div class="flex gap-2 overflow-x-auto border-b border-gray-100 pb-3 dark:border-dark-700">
            <button
              v-for="tab in settingsTabs"
              :key="tab.id"
              type="button"
              class="inline-flex whitespace-nowrap rounded-lg px-3 py-2 text-sm font-medium transition-colors"
              :class="activeSettingsTab === tab.id ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300' : 'text-gray-500 hover:bg-gray-50 hover:text-gray-900 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-white'"
              @click="activeSettingsTab = tab.id"
            >
              {{ tab.label }}
            </button>
          </div>

          <div v-if="activeSettingsTab === 'basic'" class="space-y-5">
            <AuditProviderSelector
              :model-value="configForm.audit_provider"
              @update:model-value="switchAuditProvider"
            />

            <div class="grid grid-cols-1 gap-5 lg:grid-cols-2">
              <div class="flex items-center justify-between rounded-lg border border-gray-100 p-4 dark:border-dark-700">
                <div>
                  <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.enabled') }}</p>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.enabledHint') }}</p>
                </div>
                <Toggle v-model="configForm.enabled" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.mode') }}</label>
                <Select v-model="configForm.mode" :options="modeOptions" />
                <p class="mt-2 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ modeDescription(configForm.mode) }}</p>
              </div>
              <div>
                <label class="input-label">{{ configForm.audit_provider === 'ai_chat' ? t('admin.riskControl.aiBaseUrl') : t('admin.riskControl.baseUrl') }}</label>
                <input v-model.trim="configForm.base_url" type="url" class="input" :placeholder="configForm.audit_provider === 'ai_chat' ? 'https://api.deepseek.com' : 'https://api.openai.com'" />
              </div>
              <div>
                <label class="input-label">{{ configForm.audit_provider === 'ai_chat' ? t('admin.riskControl.aiModel') : t('admin.riskControl.model') }}</label>
                <input v-model.trim="configForm.model" type="text" class="input" :placeholder="configForm.audit_provider === 'ai_chat' ? 'deepseek-v4-flash' : 'omni-moderation-latest'" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.timeoutMs') }}</label>
                <input v-model.number="configForm.timeout_ms" type="number" min="500" max="30000" class="input" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.retryCount') }}</label>
                <input v-model.number="configForm.retry_count" type="number" min="0" max="5" class="input" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.sampleRate') }}</label>
                <div class="relative">
                  <input v-model.number="configForm.sample_rate" type="number" min="0" max="100" step="1" class="input pr-8" />
                  <span class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-gray-400">%</span>
                </div>
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.proxy') }}</label>
                <ProxySelector v-model="configForm.proxy_id" :proxies="proxies" />
                <p class="mt-2 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.proxyHint') }}</p>
              </div>
              <template v-if="configForm.audit_provider === 'ai_chat'">
                <div>
                  <label class="input-label">{{ configForm.ai_risk_levels_enabled ? t('admin.riskControl.aiBlockThreshold') : t('admin.riskControl.aiConfidenceThreshold') }}</label>
                  <input v-model.number="configForm.ai_confidence_threshold" type="number" min="0.01" max="1" step="0.01" class="input" />
                  <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ configForm.ai_risk_levels_enabled ? t('admin.riskControl.aiBlockThresholdHint') : t('admin.riskControl.aiConfidenceThresholdHint') }}</p>
                </div>
                <div>
                  <label class="input-label">{{ t('admin.riskControl.aiFailurePolicy') }}</label>
                  <Select v-model="configForm.ai_failure_policy" :options="aiFailurePolicyOptions" />
                  <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.aiFailurePolicyHint') }}</p>
                </div>
                <div class="flex items-center justify-between rounded-lg border border-gray-100 p-4 dark:border-dark-700">
                  <div>
                    <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.aiCacheEnabled') }}</p>
                    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.aiCacheEnabledHint') }}</p>
                  </div>
                  <Toggle v-model="configForm.ai_cache_enabled" />
                </div>
                <div>
                  <label class="input-label">{{ t('admin.riskControl.aiCacheTTL') }}</label>
                  <input v-model.number="configForm.ai_cache_ttl_seconds" type="number" min="1" max="86400" class="input" :disabled="!configForm.ai_cache_enabled" />
                </div>
                <div>
                  <label class="input-label">{{ t('admin.riskControl.aiMaxInputChars') }}</label>
                  <input v-model.number="configForm.ai_max_input_chars" type="number" min="1000" max="1000000" step="1000" class="input" />
                  <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.aiMaxInputCharsHint') }}</p>
                </div>
                <!-- CUSTOM(VOTE-AI-RISK-PERFORMANCE): bounded synchronous fast path and supplemental-review sizing. -->
                <ModerationPerformanceSettings
                  v-model:synchronous-budget-ms="configForm.ai_synchronous_budget_ms"
                  v-model:fast-stage-budget-ms="configForm.ai_fast_stage_budget_ms"
                  v-model:fast-input-chars="configForm.ai_fast_input_chars"
                  v-model:fallback-input-chars="configForm.ai_fallback_input_chars"
                  :max-input-chars="configForm.ai_max_input_chars"
                  class="lg:col-span-2"
                />
                <!-- CUSTOM(VOTE-AI-AUDIT-COST/CONTEXT): isolated incremental audit and full-review controls. -->
                <IncrementalAuditSettings
                  ref="incrementalAuditSettingsRef"
                  v-model:incremental-audit-enabled="configForm.ai_incremental_audit_enabled"
                  v-model:input-provenance-v2-enabled="configForm.ai_input_provenance_v2_enabled"
                  v-model:deterministic-risk-v2-enabled="configForm.ai_deterministic_risk_v2_enabled"
                  v-model:recent-user-turns="configForm.ai_recent_user_turns"
                  v-model:summary-max-chars="configForm.ai_summary_max_chars"
                  v-model:full-review-threshold="configForm.ai_full_review_threshold"
                  v-model:full-review-risk-delta="configForm.ai_full_review_risk_delta"
                  v-model:periodic-full-review-turns="configForm.ai_periodic_full_review_turns"
                  v-model:full-review-max-input-chars="configForm.ai_full_review_max_input_chars"
                  v-model:fast-max-output-tokens="configForm.ai_fast_max_output_tokens"
                  v-model:full-max-output-tokens="configForm.ai_full_max_output_tokens"
                  v-model:max-review-max-output-tokens="configForm.ai_max_review_max_output_tokens"
                  v-model:audit-context-ttl-minutes="configForm.ai_audit_context_ttl_minutes"
                  v-model:pricing-configured="configForm.ai_pricing_configured"
                  v-model:pricing-version="configForm.ai_pricing_version"
                  v-model:uncached-input-usd-per-million-tokens="configForm.ai_uncached_input_usd_per_million_tokens"
                  v-model:cached-input-usd-per-million-tokens="configForm.ai_cached_input_usd_per_million_tokens"
                  v-model:output-usd-per-million-tokens="configForm.ai_output_usd_per_million_tokens"
                  :max-input-chars="configForm.ai_max_input_chars"
                  :block-threshold="configForm.ai_confidence_threshold"
                  :runtime-status="status"
                  class="lg:col-span-2"
                />
                <div class="flex items-center justify-between rounded-lg border border-gray-100 p-4 dark:border-dark-700">
                  <div class="min-w-0 pr-4">
                    <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.aiThinkingMode') }}</p>
                    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.aiThinkingModeHint') }}</p>
                  </div>
                  <Toggle v-model="aiThinkingEnabled" />
                </div>
                <div v-if="configForm.ai_thinking_mode === 'enabled'" class="lg:col-span-2">
                  <label class="input-label">{{ t('admin.riskControl.aiReasoningEffort') }}</label>
                  <div class="grid grid-cols-2 overflow-hidden rounded-lg border border-gray-200 bg-gray-50 p-1 sm:grid-cols-4 dark:border-dark-600 dark:bg-dark-900/40">
                    <button
                      v-for="option in aiReasoningEffortOptions"
                      :key="String(option.value)"
                      type="button"
                      class="min-h-10 px-3 text-sm font-medium transition-colors"
                      :class="configForm.ai_reasoning_effort === option.value
                        ? 'rounded-md bg-white text-primary-600 shadow-sm dark:bg-dark-700 dark:text-primary-300'
                        : 'text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white'"
                      @click="setAIReasoningEffort(String(option.value))"
                    >
                      {{ option.label }}
                    </button>
                  </div>
                  <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.aiReasoningEffortHint') }}</p>
                </div>
                <div class="flex items-center justify-between border-t border-gray-100 pt-4 dark:border-dark-700 lg:col-span-2">
                  <div class="min-w-0 pr-4">
                    <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.aiRiskLevelsEnabled') }}</p>
                    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.aiRiskLevelsEnabledHint') }}</p>
                  </div>
                  <Toggle v-model="configForm.ai_risk_levels_enabled" />
                </div>
                <template v-if="configForm.ai_risk_levels_enabled">
                  <div>
                    <label class="input-label">{{ t('admin.riskControl.aiObserveThreshold') }}</label>
                    <input v-model.number="configForm.ai_observe_threshold" type="number" min="0.01" :max="Math.max(0.01, configForm.ai_confidence_threshold - 0.01)" step="0.01" class="input" />
                    <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.aiObserveThresholdHint') }}</p>
                  </div>
                  <div class="flex items-center justify-between rounded-lg border border-gray-100 p-4 dark:border-dark-700">
                    <div class="min-w-0 pr-4">
                      <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.aiSessionRiskEnabled') }}</p>
                      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.aiSessionRiskEnabledHint') }}</p>
                    </div>
                    <Toggle v-model="configForm.ai_session_risk_enabled" />
                  </div>
                  <template v-if="configForm.ai_session_risk_enabled">
                    <div>
                      <label class="input-label">{{ t('admin.riskControl.aiSessionRiskTTL') }}</label>
                      <input v-model.number="configForm.ai_session_risk_ttl_minutes" type="number" min="1" max="1440" class="input" />
                    </div>
                    <div>
                      <label class="input-label">{{ t('admin.riskControl.aiSessionRiskHalfLife') }}</label>
                      <input v-model.number="configForm.ai_session_risk_half_life_minutes" type="number" min="1" max="720" class="input" />
                    </div>
                    <div>
                      <label class="input-label">{{ t('admin.riskControl.aiSessionRiskCooldown') }}</label>
                      <input v-model.number="configForm.ai_session_risk_block_cooldown_minutes" type="number" min="0" max="1440" class="input" />
                    </div>
                    <div class="flex items-center justify-between rounded-lg border border-gray-100 p-4 dark:border-dark-700">
                      <div class="min-w-0 pr-4">
                        <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.aiActorRiskEnabled') }}</p>
                        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.aiActorRiskEnabledHint') }}</p>
                      </div>
                      <Toggle v-model="configForm.ai_actor_risk_enabled" />
                    </div>
                    <p class="text-xs leading-5 text-gray-500 dark:text-gray-400 lg:col-span-2">{{ t('admin.riskControl.aiSessionIdentityHint') }}</p>
                  </template>
                </template>
              </template>
            </div>

            <!-- CUSTOM(VOTE-AI-RISK-PROMPT): backend recommendation is the only prompt authority. -->
            <RecommendedPromptControl
              v-if="configForm.audit_provider === 'ai_chat'"
              :model-value="configForm.ai_system_prompt"
              :recommended-system-prompt="recommendedAIChatSystemPrompt"
              :recommended-prompt-version="recommendedAIChatPromptVersion"
              :system-prompt-version="activeAIChatPromptVersion"
              :uses-recommended-system-prompt="usesRecommendedAIChatSystemPrompt"
              @update:model-value="updateAIChatPrompt"
              @apply-recommended="applyRecommendedAIChatPrompt"
            />

            <div class="overflow-hidden rounded-xl border border-gray-100 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
              <div class="flex flex-col gap-4 border-b border-gray-100 bg-gray-50 px-4 py-4 dark:border-dark-700 dark:bg-dark-800/60 lg:flex-row lg:items-center lg:justify-between">
                <div class="flex items-start gap-3">
                  <span class="mt-0.5 flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg bg-primary-50 text-primary-600 dark:bg-primary-900/30 dark:text-primary-300">
                    <Icon name="key" size="md" />
                  </span>
                  <div>
                    <label class="text-sm font-semibold text-gray-900 dark:text-white">{{ configForm.audit_provider === 'ai_chat' ? t('admin.riskControl.aiApiKeys') : t('admin.riskControl.apiKeys') }}</label>
                    <p class="mt-1 max-w-3xl text-xs leading-5 text-gray-500 dark:text-gray-400">
                      {{ t('admin.riskControl.apiKeysHint', { count: configForm.api_key_count }) }}
                    </p>
                  </div>
                </div>
                <div class="flex flex-wrap items-center gap-2">
                  <button
                    type="button"
                    class="btn btn-secondary inline-flex items-center gap-2"
                    :disabled="apiKeyTesting || inputApiKeyCount === 0 || configForm.clear_api_key"
                    @click="testApiKeys(true)"
                  >
                    <Icon name="beaker" size="sm" :class="apiKeyTesting ? 'animate-pulse' : ''" />
                    {{ apiKeyTesting ? t('admin.riskControl.testingApiKeys') : t('admin.riskControl.testInputApiKeys') }}
                  </button>
                  <button
                    type="button"
                    class="btn btn-secondary inline-flex items-center gap-2"
                    :disabled="apiKeyTesting || effectiveStoredApiKeyCount === 0 || pendingDeletedApiKeyCount > 0 || configForm.clear_api_key || configForm.api_keys_mode === 'replace'"
                    @click="testApiKeys(false)"
                  >
                    <Icon name="shield" size="sm" />
                    {{ storedApiKeyTestButtonText }}
                  </button>
                  <button
                    v-if="configForm.api_key_configured"
                    type="button"
                    class="btn btn-secondary inline-flex items-center gap-2"
                    @click="toggleClearApiKey"
                  >
                    <Icon :name="configForm.clear_api_key ? 'x' : 'trash'" size="sm" />
                    {{ configForm.clear_api_key ? t('admin.riskControl.keepApiKey') : t('admin.riskControl.clearApiKey') }}
                  </button>
                </div>
              </div>

              <div class="grid grid-cols-1 gap-4 p-4 xl:grid-cols-[minmax(0,1fr)_minmax(360px,440px)]">
                <div class="space-y-3">
                  <div class="flex flex-col gap-2 rounded-lg border border-gray-100 bg-gray-50 p-2 dark:border-dark-700 dark:bg-dark-900/30 sm:flex-row sm:items-center sm:justify-between">
                    <div class="text-xs leading-5 text-gray-500 dark:text-gray-400">
                      <span class="font-medium text-gray-700 dark:text-gray-200">{{ t('admin.riskControl.apiKeysWriteMode') }}</span>
                      <span class="ml-2">{{ apiKeysModeHint }}</span>
                    </div>
                    <div class="inline-flex rounded-lg bg-white p-1 shadow-sm dark:bg-dark-800">
                      <button
                        type="button"
                        class="rounded-md px-3 py-1.5 text-xs font-medium transition-colors"
                        :class="configForm.api_keys_mode === 'append' ? 'bg-primary-500 text-white shadow-sm' : 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700'"
                        :disabled="configForm.clear_api_key"
                        @click="setAPIKeysMode('append')"
                      >
                        {{ t('admin.riskControl.apiKeysModeAppend') }}
                      </button>
                      <button
                        type="button"
                        class="rounded-md px-3 py-1.5 text-xs font-medium transition-colors"
                        :class="configForm.api_keys_mode === 'replace' ? 'bg-amber-500 text-white shadow-sm' : 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700'"
                        :disabled="configForm.clear_api_key"
                        @click="setAPIKeysMode('replace')"
                      >
                        {{ t('admin.riskControl.apiKeysModeReplace') }}
                      </button>
                    </div>
                  </div>
                  <textarea
                    v-model="configForm.api_keys_text"
                    class="input min-h-44 resize-y font-mono text-sm"
                    :placeholder="apiKeysPlaceholder"
                    autocomplete="new-password"
                    :disabled="configForm.clear_api_key"
                  ></textarea>
                  <div class="flex flex-wrap items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
                    <span class="inline-flex rounded-md bg-gray-100 px-2 py-1 dark:bg-dark-700">
                      {{ t('admin.riskControl.inputApiKeyCount', { count: inputApiKeyCount }) }}
                    </span>
                    <span v-if="configForm.api_key_configured" class="inline-flex rounded-md bg-gray-100 px-2 py-1 dark:bg-dark-700">
                      {{ t('admin.riskControl.storedApiKeyCount', { count: configForm.api_key_count }) }}
                    </span>
                    <span v-if="configForm.clear_api_key" class="inline-flex rounded-md bg-red-50 px-2 py-1 text-red-700 dark:bg-red-900/20 dark:text-red-300">
                      {{ t('admin.riskControl.apiKeyWillClear') }}
                    </span>
                    <span v-else-if="pendingDeletedApiKeyCount > 0" class="inline-flex rounded-md bg-amber-50 px-2 py-1 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300">
                      {{ t('admin.riskControl.apiKeyPendingDeleteCount', { count: pendingDeletedApiKeyCount }) }}
                    </span>
                    <span v-if="configForm.api_keys_mode === 'replace'" class="inline-flex rounded-md bg-amber-50 px-2 py-1 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300">
                      {{ t('admin.riskControl.apiKeysReplaceWarning') }}
                    </span>
                  </div>

                  <div class="rounded-lg border border-gray-100 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-900/30" @paste="handleModerationImagePaste">
                    <div class="mb-3 flex items-center justify-between gap-3">
                      <div>
                        <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.auditTestInput') }}</p>
                        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.auditTestInputHint') }}</p>
                      </div>
                      <button
                        v-if="moderationTestPrompt || moderationTestImages.length > 0 || moderationTestResult"
                        type="button"
                        class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-gray-500 hover:bg-white hover:text-gray-900 dark:text-gray-400 dark:hover:bg-dark-800 dark:hover:text-white"
                        @click="clearModerationTestInput"
                      >
                        <Icon name="x" size="xs" />
                        {{ t('admin.riskControl.clearAuditTest') }}
                      </button>
                    </div>
                    <textarea
                      v-model="moderationTestPrompt"
                      class="input min-h-24 resize-y text-sm"
                      :placeholder="t('admin.riskControl.auditTestPromptPlaceholder')"
                    ></textarea>
                    <div
                      v-if="configForm.audit_provider === 'openai_moderations'"
                      class="mt-3 rounded-lg border border-dashed border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-800"
                      @dragover.prevent
                      @drop.prevent="handleModerationImageDrop"
                    >
                      <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                        <div class="flex items-start gap-2">
                          <Icon name="upload" size="md" class="mt-0.5 text-gray-400" />
                          <div>
                            <p class="text-sm font-medium text-gray-800 dark:text-gray-100">{{ t('admin.riskControl.auditTestImages') }}</p>
                            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.auditTestImagesHint') }}</p>
                          </div>
                        </div>
                        <label class="btn btn-secondary inline-flex cursor-pointer items-center gap-2">
                          <Icon name="plus" size="sm" />
                          {{ t('admin.riskControl.addAuditTestImage') }}
                          <input type="file" accept="image/*" multiple class="sr-only" @change="handleModerationImageUpload" />
                        </label>
                      </div>
                      <div v-if="moderationTestImages.length > 0" class="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-4">
                        <div
                          v-for="(image, index) in moderationTestImages"
                          :key="image.slice(0, 64) + index"
                          class="group relative aspect-square overflow-hidden rounded-lg border border-gray-100 bg-gray-100 dark:border-dark-700 dark:bg-dark-700"
                        >
                          <img :src="image" alt="" class="h-full w-full object-cover" />
                          <button
                            type="button"
                            class="absolute right-1.5 top-1.5 flex h-7 w-7 items-center justify-center rounded-full bg-black/60 text-white opacity-0 transition-opacity group-hover:opacity-100"
                            @click="removeModerationTestImage(index)"
                          >
                            <Icon name="x" size="xs" :stroke-width="2" />
                          </button>
                        </div>
                      </div>
                    </div>
                    <p v-else class="mt-3 rounded-lg border border-gray-100 bg-white px-3 py-2 text-xs leading-5 text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400">
                      {{ t('admin.riskControl.aiTextOnlyHint') }}
                    </p>
                  </div>
                </div>

                <div class="rounded-lg border border-gray-100 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-900/30">
                  <div class="mb-3 flex items-start justify-between gap-3">
                    <div class="min-w-0">
                      <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.apiKeyHealth') }}</p>
                      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.apiKeyFreezeRule') }}</p>
                    </div>
                    <span class="inline-flex shrink-0 items-center whitespace-nowrap rounded-full bg-white px-2 py-0.5 text-[11px] font-medium leading-5 text-gray-600 shadow-sm dark:bg-dark-800 dark:text-gray-300">
                      {{ t('admin.riskControl.apiKeyRows', { count: apiKeyRows.length }) }}
                    </span>
                  </div>

                  <div v-if="apiKeyRows.length === 0" class="flex min-h-32 flex-col items-center justify-center rounded-lg border border-dashed border-gray-200 bg-white px-4 py-6 text-center dark:border-dark-700 dark:bg-dark-800">
                    <Icon name="infoCircle" size="lg" class="text-gray-300 dark:text-dark-500" />
                    <p class="mt-2 text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('admin.riskControl.apiKeyHealthEmpty') }}</p>
                    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.apiKeyHealthEmptyHint') }}</p>
                  </div>
                  <div v-else class="space-y-2">
                    <div class="space-y-2" :class="apiKeyRowsExpanded ? 'max-h-72 overflow-y-auto pr-1' : ''">
                      <div
                        v-for="(row, index) in visibleApiKeyRows"
                        :key="apiKeyRowKey(row, index)"
                        class="rounded-lg border bg-white p-2.5 shadow-sm dark:bg-dark-800"
                        :class="isStoredApiKeyPendingDelete(row) ? 'border-amber-200 opacity-70 dark:border-amber-800/60' : 'border-gray-100 dark:border-dark-700'"
                      >
                        <div class="flex items-start justify-between gap-2">
                          <div class="min-w-0">
                            <div class="flex min-w-0 flex-wrap items-center gap-2">
                              <span class="truncate font-mono text-sm font-semibold text-gray-900 dark:text-white">{{ row.masked || '-' }}</span>
                              <span
                                class="inline-flex rounded-md px-1.5 py-0.5 text-[11px] font-medium"
                                :class="row.configured ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300' : 'bg-purple-50 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300'"
                              >
                                {{ isStoredApiKeyPendingDelete(row) ? t('admin.riskControl.apiKeyPendingDelete') : row.configured ? t('admin.riskControl.apiKeyConfigured') : t('admin.riskControl.apiKeyTemporary') }}
                              </span>
                            </div>
                            <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ apiKeyStatusMeta(row) }}</p>
                          </div>
                          <div class="flex flex-shrink-0 items-center gap-1.5">
                            <span class="inline-flex items-center gap-1.5 whitespace-nowrap rounded-full px-2 py-0.5 text-xs font-medium" :class="apiKeyStatusBadgeClass(row.status)">
                              <span class="h-1.5 w-1.5 rounded-full" :class="apiKeyStatusDotClass(row.status)"></span>
                              {{ apiKeyStatusLabel(row.status) }}
                            </span>
                            <button
                              v-if="row.configured && !configForm.clear_api_key"
                              type="button"
                              class="inline-flex h-7 w-7 items-center justify-center rounded-md text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-dark-700 dark:hover:text-gray-200"
                              :title="isStoredApiKeyPendingDelete(row) ? t('admin.riskControl.undoDeleteApiKey') : t('admin.riskControl.deleteApiKey')"
                              @click="toggleDeleteStoredApiKey(row)"
                            >
                              <Icon :name="isStoredApiKeyPendingDelete(row) ? 'refresh' : 'trash'" size="xs" />
                            </button>
                          </div>
                        </div>
                        <p v-if="row.last_error" class="mt-1.5 rounded-md bg-amber-50 px-2 py-1.5 text-xs leading-5 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300">
                          {{ row.last_error }}
                        </p>
                      </div>
                    </div>

                    <div v-if="canToggleApiKeyRows" class="flex items-center justify-between gap-3 rounded-lg border border-dashed border-gray-200 bg-white px-3 py-2 text-xs text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400">
                      <span class="min-w-0 truncate">
                        {{ apiKeyRowsExpanded ? t('admin.riskControl.apiKeyRowsExpanded', { count: apiKeyRows.length }) : t('admin.riskControl.apiKeyRowsCollapsed', { count: hiddenApiKeyRowCount }) }}
                      </span>
                      <button
                        type="button"
                        class="inline-flex shrink-0 items-center gap-1 rounded-md px-2 py-1 font-medium text-primary-600 transition-colors hover:bg-primary-50 hover:text-primary-700 dark:text-primary-300 dark:hover:bg-primary-900/20"
                        @click="apiKeyRowsExpanded = !apiKeyRowsExpanded"
                      >
                        <Icon :name="apiKeyRowsExpanded ? 'chevronUp' : 'chevronDown'" size="xs" />
                        {{ apiKeyRowsExpanded ? t('admin.riskControl.collapseApiKeyRows') : t('admin.riskControl.expandApiKeyRows') }}
                      </button>
                    </div>
                  </div>

                  <div v-if="moderationTestResult" class="mt-4 rounded-lg border border-gray-100 bg-white p-3 dark:border-dark-700 dark:bg-dark-800">
                    <!-- CUSTOM(VOTE-AI-RISK-TRIAL): structured risk tier, categories, signals, and review completeness. -->
                    <ModerationTestOutcome :result="moderationTestResult" />
                    <div class="mt-3">
                      <div class="mb-2 flex items-center justify-between text-xs text-gray-500 dark:text-gray-400">
                        <span>{{ t('admin.riskControl.auditTestComposite') }}</span>
                        <span class="font-semibold text-gray-900 dark:text-white">{{ percent(moderationTestResult.composite_score) }}</span>
                      </div>
                      <div class="h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                        <div class="h-full rounded-full" :class="moderationTestResult.flagged ? 'bg-red-500' : 'bg-emerald-500'" :style="{ width: percentWidth(moderationTestResult.composite_score) }"></div>
                      </div>
                    </div>
                    <p v-if="moderationTestResult.reason" class="mt-3 rounded-lg bg-gray-50 px-3 py-2 text-xs leading-5 text-gray-600 dark:bg-dark-900/40 dark:text-gray-300">
                      {{ moderationTestResult.reason }}
                    </p>
                    <div class="mt-3 max-h-52 space-y-2 overflow-y-auto pr-1">
                      <div v-for="score in moderationScoreRows" :key="score.category">
                        <div class="mb-1 flex items-center justify-between gap-3 text-xs">
                          <span class="truncate text-gray-600 dark:text-gray-300">{{ moderationCategoryLabel(score.category) }}</span>
                          <span class="font-mono text-gray-500 dark:text-gray-400">{{ percent(score.score) }} / {{ percent(score.threshold) }}</span>
                        </div>
                        <div class="h-1.5 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                          <div class="h-full rounded-full" :class="score.hit ? 'bg-red-500' : 'bg-primary-500'" :style="{ width: percentWidth(score.score) }"></div>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div v-else-if="activeSettingsTab === 'scope'" class="space-y-5">
            <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
              <div>
                <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.groupScope') }}</h3>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.groupScopeHint') }}</p>
              </div>
              <div class="inline-flex rounded-lg bg-gray-100 p-1 dark:bg-dark-700">
                <button
                  type="button"
                  class="rounded-md px-3 py-1.5 text-sm font-medium transition-colors"
                  :class="configForm.all_groups ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-800 dark:text-white' : 'text-gray-500 dark:text-gray-400'"
                  @click="configForm.all_groups = true"
                >
                  {{ t('admin.riskControl.allGroups') }}
                </button>
                <button
                  type="button"
                  class="rounded-md px-3 py-1.5 text-sm font-medium transition-colors"
                  :class="!configForm.all_groups ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-800 dark:text-white' : 'text-gray-500 dark:text-gray-400'"
                  @click="configForm.all_groups = false"
                >
                  {{ t('admin.riskControl.selectedGroups') }}
                </button>
              </div>
            </div>

            <div v-if="!configForm.all_groups" class="space-y-4">
              <div class="relative">
                <Icon name="search" size="sm" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
                <input v-model.trim="groupSearch" type="search" class="input pl-9" :placeholder="t('admin.riskControl.searchGroups')" />
              </div>
              <div class="grid max-h-[420px] grid-cols-1 gap-3 overflow-y-auto pr-1 md:grid-cols-2 xl:grid-cols-3">
                <button
                  v-for="group in filteredGroups"
                  :key="group.id"
                  type="button"
                  class="flex min-h-20 items-center justify-between rounded-lg border p-4 text-left transition-colors"
                  :class="isGroupSelected(group.id) ? 'border-primary-300 bg-primary-50 dark:border-primary-700 dark:bg-primary-900/20' : 'border-gray-100 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-700/60'"
                  @click="toggleGroup(group.id)"
                >
                  <span class="min-w-0">
                    <span class="block truncate text-sm font-semibold text-gray-900 dark:text-white">{{ group.name }}</span>
                    <span class="mt-1 inline-flex rounded-md bg-gray-100 px-2 py-0.5 text-xs text-gray-500 dark:bg-dark-700 dark:text-gray-400">{{ group.platform }}</span>
                  </span>
                  <span
                    class="flex h-5 w-5 flex-shrink-0 items-center justify-center rounded-full border"
                    :class="isGroupSelected(group.id) ? 'border-primary-500 bg-primary-500 text-white' : 'border-gray-300 text-transparent dark:border-dark-500'"
                  >
                    <Icon name="check" size="xs" :stroke-width="2" />
                  </span>
                </button>
                <p v-if="filteredGroups.length === 0" class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.noGroups') }}</p>
              </div>
            </div>

            <!-- CUSTOM(VOTE-AI-RISK-SCOPE): explicit user/account moderation boundaries. -->
            <div class="rounded-lg border border-sky-200 bg-sky-50 px-4 py-3 text-sm text-sky-800 dark:border-sky-800 dark:bg-sky-900/20 dark:text-sky-200">
              {{ t('admin.riskControl.internalProbeBypassNotice') }}
            </div>

            <div
              v-for="section in scopeFilterSections"
              :key="section.entity"
              class="space-y-4 rounded-lg border border-gray-100 p-4 dark:border-dark-700"
              :data-test="`${section.entity}-filter-section`"
            >
              <div class="flex flex-col gap-2 lg:flex-row lg:items-center lg:justify-between">
                <div>
                  <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ section.title }}</h3>
                  <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ section.hint }}</p>
                </div>
                <span class="inline-flex w-fit rounded-md bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-600 dark:bg-dark-700 dark:text-gray-300">
                  {{ scopeFilterSummary(section.entity) }}
                </span>
              </div>

              <div class="grid grid-cols-1 gap-2 md:grid-cols-3">
                <button
                  v-for="type in scopeFilterTypes"
                  :key="type"
                  type="button"
                  class="rounded-lg border p-3 text-left transition-colors"
                  :class="scopeFilterType(section.entity) === type
                    ? 'border-primary-300 bg-primary-50 text-primary-900 shadow-sm dark:border-primary-700 dark:bg-primary-900/20 dark:text-primary-100'
                    : 'border-gray-100 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-700/60'"
                  :data-test="`${section.entity}-filter-${type}`"
                  @click="setScopeFilterType(section.entity, type)"
                >
                  <div class="flex items-center justify-between gap-2">
                    <span class="text-sm font-semibold">{{ scopeFilterTypeLabel(type) }}</span>
                    <span
                      class="flex h-4 w-4 flex-shrink-0 items-center justify-center rounded-full border"
                      :class="scopeFilterType(section.entity) === type
                        ? 'border-primary-500 bg-primary-500 text-white'
                        : 'border-gray-300 text-transparent dark:border-dark-500'"
                    >
                      <Icon name="check" size="xs" :stroke-width="2" />
                    </span>
                  </div>
                  <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">
                    {{ scopeFilterTypeDescription(section.entity, type) }}
                  </p>
                </button>
              </div>

              <div v-if="scopeFilterType(section.entity) !== 'all'" class="space-y-2">
                <label class="input-label">{{ section.selectorLabel }}</label>
                <ScopeEntitySelector
                  :entity="section.entity"
                  :model-value="scopeFilterIds(section.entity)"
                  @update:model-value="updateScopeFilterIds(section.entity, $event)"
                />
                <p
                  v-if="scopeFilterType(section.entity) === 'include' && scopeFilterIds(section.entity).length === 0"
                  class="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs leading-5 text-amber-800 dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-200"
                  :data-test="`${section.entity}-filter-empty-warning`"
                >
                  {{ section.emptyIncludeWarning }}
                </p>
              </div>
            </div>

            <div class="space-y-4 rounded-lg border border-gray-100 p-4 dark:border-dark-700">
              <div class="flex flex-col gap-2 lg:flex-row lg:items-center lg:justify-between">
                <div>
                  <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.modelFilter') }}</h3>
                  <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.modelFilterHint') }}</p>
                </div>
                <span class="inline-flex w-fit rounded-md bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-600 dark:bg-dark-700 dark:text-gray-300">
                  {{ modelFilterSummary }}
                </span>
              </div>

              <div class="grid grid-cols-1 gap-2 md:grid-cols-3">
                <button
                  v-for="option in modelFilterOptions"
                  :key="option.value"
                  type="button"
                  class="rounded-lg border p-3 text-left transition-colors"
                  :class="configForm.model_filter_type === option.value
                    ? 'border-primary-300 bg-primary-50 text-primary-900 shadow-sm dark:border-primary-700 dark:bg-primary-900/20 dark:text-primary-100'
                    : 'border-gray-100 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-700/60'"
                  @click="setModelFilterType(option.value)"
                >
                  <div class="flex items-center justify-between gap-2">
                    <span class="text-sm font-semibold">{{ option.label }}</span>
                    <span
                      class="flex h-4 w-4 flex-shrink-0 items-center justify-center rounded-full border"
                      :class="configForm.model_filter_type === option.value
                        ? 'border-primary-500 bg-primary-500 text-white'
                        : 'border-gray-300 text-transparent dark:border-dark-500'"
                    >
                      <Icon name="check" size="xs" :stroke-width="2" />
                    </span>
                  </div>
                  <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ option.description }}</p>
                </button>
              </div>

              <div v-if="configForm.model_filter_type !== 'all'" class="space-y-2">
                <label class="input-label">{{ t('admin.riskControl.modelFilterModels') }}</label>
                <ModelWhitelistSelector v-model="configForm.model_filter_models" />
                <p class="text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.riskControl.modelFilterModelCount', { count: modelFilterModelCount }) }}
                </p>
              </div>
            </div>
          </div>

          <div v-else-if="activeSettingsTab === 'runtime'" class="grid grid-cols-1 gap-5 lg:grid-cols-2">
            <div>
              <label class="input-label">{{ t('admin.riskControl.workerCount') }}</label>
              <input v-model.number="configForm.worker_count" type="number" min="1" max="32" class="input" />
            </div>
            <div>
              <label class="input-label">{{ t('admin.riskControl.queueSize') }}</label>
              <input v-model.number="configForm.queue_size" type="number" min="100" max="100000" class="input" />
            </div>
            <div class="flex items-center justify-between rounded-lg border border-gray-100 p-4 dark:border-dark-700 lg:col-span-2">
              <div>
                <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.recordNonHits') }}</p>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.recordNonHitsHint') }}</p>
              </div>
              <Toggle v-model="configForm.record_non_hits" />
            </div>
            <div class="space-y-4 rounded-lg border border-gray-100 p-4 dark:border-dark-700 lg:col-span-2">
              <div class="flex items-center justify-between gap-4">
                <div>
                  <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.preHashCheck') }}</p>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.preHashCheckHint') }}</p>
                </div>
                <Toggle v-model="configForm.pre_hash_check_enabled" />
              </div>
              <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-900/30">
                <div class="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
                  <div>
                    <p class="text-sm font-medium text-gray-900 dark:text-white">
                      {{ t('admin.riskControl.flaggedHashCount', { count: formatNumber(status?.flagged_hash_count ?? 0) }) }}
                    </p>
                    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.flaggedHashHint') }}</p>
                  </div>
                  <button
                    type="button"
                    class="btn btn-secondary inline-flex items-center justify-center gap-2 text-red-600 hover:text-red-700 dark:text-red-300"
                    :disabled="hashActionLoading || (status?.flagged_hash_count ?? 0) === 0"
                    @click="clearFlaggedHashes"
                  >
                    <Icon name="trash" size="sm" :class="hashActionLoading ? 'animate-pulse' : ''" />
                    {{ t('admin.riskControl.clearFlaggedHashes') }}
                  </button>
                </div>
                <div class="mt-3 flex flex-col gap-2 sm:flex-row">
                  <input
                    v-model.trim="flaggedHashInput"
                    type="text"
                    class="input font-mono text-sm"
                    :placeholder="t('admin.riskControl.flaggedHashPlaceholder')"
                  />
                  <button
                    type="button"
                    class="btn btn-secondary inline-flex items-center justify-center gap-2"
                    :disabled="hashActionLoading || !isFlaggedHashInputValid"
                    @click="deleteFlaggedHash"
                  >
                    <Icon name="trash" size="sm" />
                    {{ t('admin.riskControl.deleteFlaggedHash') }}
                  </button>
                </div>
              </div>
            </div>
          </div>

          <div v-else-if="activeSettingsTab === 'response'" class="space-y-5">
            <div class="grid grid-cols-1 gap-5 lg:grid-cols-2">
              <div>
                <label class="input-label">{{ t('admin.riskControl.blockStatus') }}</label>
                <input v-model.number="configForm.block_status" type="number" min="400" max="599" class="input" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.blockMessage') }}</label>
                <input v-model.trim="configForm.block_message" type="text" class="input" />
              </div>
              <div class="flex items-center justify-between rounded-lg border border-gray-100 p-4 dark:border-dark-700">
                <div>
                  <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.emailOnHit') }}</p>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.emailOnHitHint') }}</p>
                </div>
                <Toggle v-model="configForm.email_on_hit" />
              </div>
              <div class="flex items-center justify-between rounded-lg border border-gray-100 p-4 dark:border-dark-700">
                <div>
                  <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.autoBan') }}</p>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.autoBanHint') }}</p>
                </div>
                <Toggle v-model="configForm.auto_ban_enabled" />
              </div>
              <div class="flex items-center justify-between rounded-lg border border-gray-100 p-4 dark:border-dark-700 lg:col-span-2">
                <div>
                  <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.cyberPolicyExcludeBan') }}</p>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.cyberPolicyExcludeBanHint') }}</p>
                </div>
                <Toggle v-model="configForm.cyber_policy_exclude_from_ban_count" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.banThreshold') }}</label>
                <input v-model.number="configForm.ban_threshold" type="number" min="1" max="1000" class="input" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.violationWindowHours') }}</label>
                <input v-model.number="configForm.violation_window_hours" type="number" min="1" max="8760" class="input" />
              </div>
            </div>
          </div>

          <div v-else-if="activeSettingsTab === 'riskThresholds'" class="space-y-5">
            <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
              <div>
                <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.riskThresholds') }}</h3>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.riskThresholdsHint') }}</p>
              </div>
              <button
                type="button"
                class="btn btn-secondary inline-flex items-center justify-center gap-2"
                @click="resetRiskThresholds"
              >
                <Icon name="refresh" size="sm" />
                {{ t('admin.riskControl.riskThresholdReset') }}
              </button>
            </div>

            <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
              <div
                v-for="row in riskThresholdRows"
                :key="row.category"
                class="rounded-lg border border-gray-100 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-900/30"
              >
                <div class="flex items-start justify-between gap-3">
                  <div class="min-w-0">
                    <label class="block truncate text-sm font-semibold text-gray-900 dark:text-white" :for="`risk-threshold-${row.category}`">
                      {{ row.category }}
                    </label>
                    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                      {{ t('admin.riskControl.riskThresholdDefault', { value: formatThresholdPercent(row.defaultValue) }) }}
                    </p>
                  </div>
                  <span class="inline-flex shrink-0 rounded-md bg-white px-2 py-1 font-mono text-xs font-medium text-gray-600 shadow-sm dark:bg-dark-800 dark:text-gray-300">
                    {{ formatThresholdPercent(row.value) }}
                  </span>
                </div>
                <div class="mt-3">
                  <label class="sr-only" :for="`risk-threshold-${row.category}`">
                    {{ t('admin.riskControl.riskThresholdPercent') }}
                  </label>
                  <div class="relative">
                    <input
                      :id="`risk-threshold-${row.category}`"
                      v-model.number="configForm.thresholds[row.category]"
                      :data-test="`risk-threshold-${row.category}`"
                      type="number"
                      min="0"
                      max="100"
                      step="0.1"
                      class="input pr-8 font-mono"
                    />
                    <span class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-gray-400">%</span>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div v-else-if="activeSettingsTab === 'keywords'" class="space-y-5">
            <div
              class="flex items-start gap-3 rounded-lg border p-4"
              :class="keywordNotice.toneClass"
            >
              <Icon
                :name="keywordNotice.icon"
                size="md"
                :class="keywordNotice.iconClass"
              />
              <div class="text-sm leading-6">
                <p class="font-medium" :class="keywordNotice.titleClass">{{ keywordNotice.title }}</p>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ keywordNotice.description }}</p>
              </div>
            </div>

            <div class="space-y-2">
              <label class="input-label">{{ t('admin.riskControl.keywordBlockingMode') }}</label>
              <div class="grid grid-cols-1 gap-2 sm:grid-cols-3">
                <button
                  v-for="option in keywordBlockingModeOptions"
                  :key="option.value"
                  type="button"
                  class="rounded-lg border p-3 text-left transition-colors"
                  :class="configForm.keyword_blocking_mode === option.value
                    ? 'border-primary-300 bg-primary-50 text-primary-900 shadow-sm dark:border-primary-700 dark:bg-primary-900/20 dark:text-primary-100'
                    : 'border-gray-100 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-700/60'"
                  @click="configForm.keyword_blocking_mode = option.value"
                >
                  <div class="flex items-center justify-between gap-2">
                    <span class="text-sm font-semibold">{{ option.label }}</span>
                    <span
                      class="flex h-4 w-4 flex-shrink-0 items-center justify-center rounded-full border"
                      :class="configForm.keyword_blocking_mode === option.value
                        ? 'border-primary-500 bg-primary-500 text-white'
                        : 'border-gray-300 text-transparent dark:border-dark-500'"
                    >
                      <Icon name="check" size="xs" :stroke-width="2" />
                    </span>
                  </div>
                  <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ option.description }}</p>
                </button>
              </div>
            </div>

            <div>
              <div class="mb-2 flex items-center justify-between">
                <label class="input-label mb-0">{{ t('admin.riskControl.blockedKeywords') }}</label>
                <span class="inline-flex rounded-md bg-gray-100 px-2 py-1 text-xs text-gray-500 dark:bg-dark-700 dark:text-gray-300">
                  {{ t('admin.riskControl.blockedKeywordCount', { count: blockedKeywordCount }) }}
                </span>
              </div>
              <textarea
                v-model="configForm.blocked_keywords_text"
                class="input min-h-52 resize-y font-mono text-sm"
                :placeholder="t('admin.riskControl.blockedKeywordsPlaceholder')"
                :disabled="configForm.keyword_blocking_mode === 'api_only'"
              ></textarea>
              <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.riskControl.blockedKeywordsLimit', { max: blockedKeywordMax }) }}
              </p>
            </div>
          </div>

          <div v-else class="grid grid-cols-1 gap-5 lg:grid-cols-2">
            <div>
              <label class="input-label">{{ t('admin.riskControl.hitRetentionDays') }}</label>
              <input v-model.number="configForm.hit_retention_days" type="number" min="1" max="3650" class="input" />
            </div>
            <div>
              <label class="input-label">{{ t('admin.riskControl.nonHitRetentionDays') }}</label>
              <input v-model.number="configForm.non_hit_retention_days" type="number" min="1" max="3" class="input" />
            </div>
            <div class="rounded-lg border border-gray-100 p-4 text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400 lg:col-span-2">
              <div class="flex flex-wrap items-center gap-3">
                <Icon name="database" size="md" class="text-gray-400" />
                <span>{{ t('admin.riskControl.cleanupStats', { hit: status?.last_cleanup_deleted_hit ?? 0, nonHit: status?.last_cleanup_deleted_non_hit ?? 0 }) }}</span>
              </div>
            </div>
          </div>
        </div>

        <template #footer>
          <div class="flex justify-end gap-2">
            <button type="button" class="btn btn-secondary" @click="closeSettings">{{ t('common.cancel') }}</button>
            <button type="button" class="btn btn-primary inline-flex items-center gap-2" :disabled="saving" @click="saveConfig">
              <Icon v-if="saving" name="refresh" size="sm" class="animate-spin" />
              <Icon v-else name="check" size="sm" />
              {{ saving ? t('common.saving') : t('admin.riskControl.saveConfig') }}
            </button>
          </div>
        </template>
      </BaseDialog>

      <BaseDialog
        :show="inputDetailRow !== null"
        :title="t('admin.riskControl.inputDetailTitle')"
        width="wide"
        @close="closeInputDetail"
      >
        <div v-if="inputDetailRow" class="space-y-5">
          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <div class="rounded-lg border border-gray-100 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/70">
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.time') }}</p>
              <p class="mt-1 truncate text-sm font-semibold text-gray-900 dark:text-white">{{ formatDateTime(inputDetailRow.created_at) }}</p>
            </div>
            <div class="rounded-lg border border-gray-100 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/70">
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.user') }}</p>
              <p class="mt-1 truncate text-sm font-semibold text-gray-900 dark:text-white">{{ inputDetailRow.user_email || '-' }}</p>
            </div>
            <div class="rounded-lg border border-gray-100 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/70">
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.result') }}</p>
              <ModerationAuditStatusBadge
                class="mt-1"
                :status="inputDetailRow.audit_status"
                :action="inputDetailRow.action"
                :flagged="inputDetailRow.flagged"
                :code="inputDetailRow.audit_code"
                :retryable="inputDetailRow.audit_retryable"
              />
            </div>
            <div class="rounded-lg border border-gray-100 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/70">
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.highest') }}</p>
              <p class="mt-1 truncate text-sm font-semibold text-gray-900 dark:text-white">
                {{ inputDetailRow.highest_category || '-' }} / {{ percent(inputDetailRow.highest_score) }}
              </p>
            </div>
            <div v-if="inputDetailRow.matched_keyword" class="rounded-lg border border-red-100 bg-red-50 p-4 dark:border-red-900/60 dark:bg-red-900/20">
              <p class="text-xs font-medium text-red-500 dark:text-red-300">{{ t('admin.riskControl.matchedKeyword') }}</p>
              <p class="mt-1 truncate text-sm font-semibold text-red-700 dark:text-red-200" :title="inputDetailRow.matched_keyword">{{ inputDetailRow.matched_keyword }}</p>
            </div>
          </div>

          <div class="rounded-lg border border-gray-100 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <div class="flex flex-wrap items-center justify-between gap-3">
              <div>
                <p class="text-sm font-semibold text-gray-900 dark:text-white">
                  {{ inputDetailAuditDetails?.audit_target_excerpt ? t('admin.riskControl.auditTarget') : t('admin.riskControl.inputDetailContent') }}
                </p>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  {{ inputDetailRow.endpoint || '-' }} · {{ inputDetailRow.provider || '-' }} / {{ inputDetailRow.model || '-' }}
                </p>
              </div>
              <span v-if="inputDetailRow.group_name" class="inline-flex rounded-md bg-sky-50 px-2.5 py-1 text-xs font-medium text-sky-700 dark:bg-sky-900/20 dark:text-sky-300">
                {{ inputDetailRow.group_name }}
              </span>
            </div>
            <pre class="mt-4 max-h-[420px] overflow-auto whitespace-pre-wrap break-words rounded-lg bg-gray-950 p-4 text-sm leading-6 text-gray-100 shadow-inner dark:bg-black/50">{{ inputDetailText }}</pre>
          </div>

          <div v-if="inputDetailAuditDetails" class="border-t border-gray-100 pt-4 dark:border-dark-700" data-test="supporting-context">
            <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.supportingContext') }}</p>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.supportingContextHint') }}</p>
            <pre class="mt-3 max-h-64 overflow-auto whitespace-pre-wrap break-words rounded-lg bg-gray-100 p-4 text-xs leading-5 text-gray-700 dark:bg-dark-900 dark:text-gray-200">{{ supportingContextText }}</pre>
          </div>

          <AuditStageDiagnostics :stages="inputDetailAuditDetails?.stages" />

          <div v-if="hasAuditDiagnostics" class="border-t border-gray-100 pt-4 dark:border-dark-700">
            <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.auditDiagnostics') }}</p>
            <dl class="mt-3 grid grid-cols-1 gap-x-6 gap-y-3 text-sm sm:grid-cols-2 lg:grid-cols-3">
              <div
                v-for="item in auditDiagnosticItems"
                :key="item.key"
                class="min-w-0"
                :data-test="`audit-diagnostic-${item.key}`"
              >
                <dt class="text-xs text-gray-500 dark:text-gray-400">{{ item.label }}</dt>
                <dd class="mt-1 break-words font-medium text-gray-900 dark:text-white">{{ item.value }}</dd>
              </div>
            </dl>
            <p v-if="inputDetailAuditDetails?.model_reason" class="mt-4 rounded-lg bg-gray-50 px-3 py-2 text-sm leading-6 text-gray-700 dark:bg-dark-900/60 dark:text-gray-200">
              {{ inputDetailAuditDetails?.model_reason }}
            </p>
          </div>
        </div>

        <template #footer>
          <div class="flex w-full flex-wrap items-center justify-between gap-3">
            <button
              v-if="inputDetailHash"
              type="button"
              class="btn btn-danger"
              :disabled="hashActionLoading || !isInputDetailHashValid"
              data-test="input-detail-delete-flagged-hash"
              @click="deleteInputDetailFlaggedHash"
            >
              <Icon name="trash" size="sm" :class="hashActionLoading ? 'animate-pulse' : ''" />
              {{ t('admin.riskControl.deleteRecordFlaggedHash') }}
            </button>
            <span v-else />
            <button type="button" class="btn btn-secondary" @click="closeInputDetail">{{ t('common.close') }}</button>
          </div>
        </template>
      </BaseDialog>

      <!-- CUSTOM(VOTE-AI-RISK-SIDE-EFFECTS): moderation-owned unban confirmation and risk-state cleanup mode. -->
      <ModerationUnbanDialog
        :show="unbanDialogRow !== null"
        :row="unbanDialogRow"
        :loading="unbanningUserID !== null"
        :warning="unbanWarning"
        @close="closeUnbanDialog"
        @confirm="confirmUnbanUser"
      />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import Pagination from '@/components/common/Pagination.vue'
import ModelWhitelistSelector from '@/components/account/ModelWhitelistSelector.vue'
import ProxySelector from '@/components/common/ProxySelector.vue'
// CUSTOM(VOTE-AI-AI-AUDIT): isolated audit-provider selector.
import AuditProviderSelector from '@/custom/vote-ai/risk-control/AuditProviderSelector.vue'
// CUSTOM(VOTE-AI-RISK-PROMPT/PERFORMANCE/TRIAL): isolated AI audit configuration and trial result UI.
import RecommendedPromptControl from '@/custom/vote-ai/risk-control/RecommendedPromptControl.vue'
import ModerationPerformanceSettings from '@/custom/vote-ai/risk-control/ModerationPerformanceSettings.vue'
import IncrementalAuditSettings from '@/custom/vote-ai/risk-control/IncrementalAuditSettings.vue'
import AuditStageDiagnostics from '@/custom/vote-ai/risk-control/AuditStageDiagnostics.vue'
import ModerationTestOutcome from '@/custom/vote-ai/risk-control/ModerationTestOutcome.vue'
// CUSTOM(VOTE-AI-RISK-SCOPE): isolated searchable user/account scope selector.
import ScopeEntitySelector from '@/custom/vote-ai/risk-control/ScopeEntitySelector.vue'
// CUSTOM(VOTE-AI-RISK-SIDE-EFFECTS): structured audit/effect status and guarded unban flow.
import ModerationAuditStatusBadge from '@/custom/vote-ai/risk-control/ModerationAuditStatusBadge.vue'
import ModerationSideEffectsStatus from '@/custom/vote-ai/risk-control/ModerationSideEffectsStatus.vue'
import ModerationUnbanDialog from '@/custom/vote-ai/risk-control/ModerationUnbanDialog.vue'
import { adminAPI } from '@/api/admin'
import type {
  ContentModerationAPIKeyLoad,
  ContentModerationAPIKeyStatus,
  ContentModerationAuditDetails,
  ContentModerationAuditProvider,
  ContentModerationProviderProfile,
  ContentModerationConfig,
  ContentModerationLog,
  ContentModerationModelFilter,
  ContentModerationModelFilterType,
  ContentModerationScopeFilterType,
  ContentModerationUserFilter,
  ContentModerationAccountFilter,
  ContentModerationRuntimeStatus,
  ContentModerationTestAuditResult,
  ContentModerationUnbanMode,
  KeywordBlockingMode,
  ModerationMode,
  UpdateContentModerationConfig,
  AIAuditFailurePolicy,
  AIAuditThinkingMode,
  AIAuditReasoningEffort,
} from '@/api/admin/riskControl'
import type { AdminGroup, Proxy, SelectOption } from '@/types'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime as formatDateTimeValue } from '@/utils/format'

type SettingsTab = 'basic' | 'scope' | 'runtime' | 'response' | 'riskThresholds' | 'retention' | 'keywords'
type WorkerSlotState = 'active' | 'idle' | 'disabled'
type APIKeysWriteMode = 'append' | 'replace'
type ScopeEntity = 'user' | 'account'
type ProviderDraft = ContentModerationProviderProfile & {
  api_keys_text: string
  api_key_statuses: ContentModerationAPIKeyStatus[]
  api_keys_mode: APIKeysWriteMode
  clear_api_key: boolean
}
type OverviewIcon = 'shield' | 'key' | 'users' | 'document'
type OverviewItem = {
  key: string
  label: string
  value: string
  meta: string
  icon: OverviewIcon
  iconClass: string
  badge?: string
  badgeClass?: string
}
type ModerationScoreRow = {
  category: string
  score: number
  threshold: number
  hit: boolean
}
type RiskThresholdRow = {
  category: string
  value: number
  defaultValue: number
}

const maxModerationTestImages = 1
const maxModerationTestImageSize = 8 * 1024 * 1024
const maxVisibleApiKeyRows: number = 3
const blockedKeywordMax = 10000
const moderationCategoryChineseLabels: Record<string, string> = {
  ai_risk: 'AI 综合风险',
  ai_current_risk: '当前请求风险',
  ai_session_risk: '会话累计风险',
  ai_actor_bonus: '跨会话风险加权',
  cyber_abuse: '网络攻击',
  credential_theft: '凭证窃取',
  malware: '恶意软件',
  phishing: '网络钓鱼',
  fraud: '欺诈',
  spam: '垃圾信息',
  policy_evasion: '规避安全策略',
  illicit: '违法活动',
  'illicit/violent': '暴力违法活动',
  hate: '仇恨内容',
  'hate/threatening': '仇恨威胁',
  harassment: '骚扰',
  'harassment/threatening': '骚扰威胁',
  sexual: '色情内容',
  'sexual/minors': '未成年人色情内容',
  sexual_minors: '未成年人色情内容',
  violence: '暴力内容',
  'violence/graphic': '血腥暴力内容',
  self_harm: '自残风险',
  'self-harm': '自残风险',
  'self-harm/intent': '自残意图',
  'self-harm/instructions': '自残指导',
  other: '其他风险',
}
const riskThresholdDefaults: Record<string, number> = {
  harassment: 98,
  'harassment/threatening': 90,
  hate: 65,
  'hate/threatening': 65,
  illicit: 95,
  'illicit/violent': 95,
  'self-harm': 65,
  'self-harm/intent': 85,
  'self-harm/instructions': 65,
  sexual: 65,
  'sexual/minors': 65,
  violence: 95,
  'violence/graphic': 95,
}
const riskThresholdCategories = Object.keys(riskThresholdDefaults)

const { t } = useI18n()
const appStore = useAppStore()
const defaultBlockMessage = () => t('admin.riskControl.defaultBlockMessage')

const loading = ref(true)
const saving = ref(false)
const logsLoading = ref(false)
const statusLoading = ref(false)
const apiKeyTesting = ref(false)
const hashActionLoading = ref(false)
const unbanningUserID = ref<number | null>(null)
const unbanDialogRow = ref<ContentModerationLog | null>(null)
const unbanWarning = ref('')
const settingsOpen = ref(false)
const activeSettingsTab = ref<SettingsTab>('basic')
const groupSearch = ref('')
const flaggedHashInput = ref('')
const groups = ref<AdminGroup[]>([])
const proxies = ref<Proxy[]>([])
const logs = ref<ContentModerationLog[]>([])
const status = ref<ContentModerationRuntimeStatus | null>(null)
const testedApiKeyStatuses = ref<ContentModerationAPIKeyStatus[]>([])
const pendingDeleteApiKeyHashes = ref<string[]>([])
const apiKeyRowsExpanded = ref<boolean>(false)
const moderationTestPrompt = ref('')
const moderationTestImages = ref<string[]>([])
const moderationTestResult = ref<ContentModerationTestAuditResult | null>(null)
const recommendedAIChatSystemPrompt = ref('')
const recommendedAIChatPromptVersion = ref('')
const activeAIChatPromptVersion = ref('')
const usesRecommendedAIChatSystemPrompt = ref(false)
const inputDetailRow = ref<ContentModerationLog | null>(null)
const savedConfigSnapshot = ref<ContentModerationConfig | null>(null)
const incrementalAuditSettingsRef = ref<{ validate: () => string | null } | null>(null)
let statusTimer: number | null = null

const configForm = reactive({
  enabled: false,
  mode: 'pre_block' as ModerationMode,
  audit_provider: 'openai_moderations' as ContentModerationAuditProvider,
  base_url: 'https://api.openai.com',
  model: 'omni-moderation-latest',
  proxy_id: null as number | null,
  api_keys_text: '',
  api_key_configured: false,
  api_key_masked: '',
  api_key_count: 0,
  api_key_masks: [] as string[],
  api_key_statuses: [] as ContentModerationAPIKeyStatus[],
  api_keys_mode: 'append' as APIKeysWriteMode,
  clear_api_key: false,
  timeout_ms: 3000,
  retry_count: 2,
  ai_confidence_threshold: 0.7,
  ai_cache_enabled: true,
  ai_cache_ttl_seconds: 300,
  ai_system_prompt: '',
  ai_failure_policy: 'allow' as AIAuditFailurePolicy,
  ai_max_input_chars: 200000,
  ai_synchronous_budget_ms: 4800,
  ai_fast_stage_budget_ms: 3000,
  ai_fast_input_chars: 3000,
  ai_fallback_input_chars: 3000,
  ai_thinking_mode: 'enabled' as AIAuditThinkingMode,
  ai_reasoning_effort: 'adaptive' as AIAuditReasoningEffort,
  ai_risk_levels_enabled: true,
  ai_observe_threshold: 0.35,
  ai_session_risk_enabled: true,
  ai_session_risk_ttl_minutes: 120,
  ai_session_risk_half_life_minutes: 30,
  ai_session_risk_block_cooldown_minutes: 30,
  ai_actor_risk_enabled: true,
  ai_incremental_audit_enabled: false,
  ai_input_provenance_v2_enabled: true,
  ai_deterministic_risk_v2_enabled: true,
  ai_recent_user_turns: 2,
  ai_summary_max_chars: 500,
  ai_full_review_threshold: 0.4,
  ai_full_review_risk_delta: 0.15,
  ai_periodic_full_review_turns: 10,
  ai_full_review_max_input_chars: 60000,
  ai_fast_max_output_tokens: 128,
  ai_full_max_output_tokens: 1024,
  ai_max_review_max_output_tokens: 1536,
  ai_audit_context_ttl_minutes: 120,
  ai_pricing_configured: false,
  ai_pricing_version: '',
  ai_uncached_input_usd_per_million_tokens: null as number | null,
  ai_cached_input_usd_per_million_tokens: null as number | null,
  ai_output_usd_per_million_tokens: null as number | null,
  sample_rate: 100,
  all_groups: true,
  group_ids: [] as number[],
  record_non_hits: false,
  worker_count: 4,
  queue_size: 32768,
  block_status: 403,
  block_message: defaultBlockMessage(),
  email_on_hit: true,
  auto_ban_enabled: true,
  cyber_policy_exclude_from_ban_count: false,
  ban_threshold: 10,
  violation_window_hours: 720,
  hit_retention_days: 180,
  non_hit_retention_days: 3,
  pre_hash_check_enabled: false,
  thresholds: { ...riskThresholdDefaults } as Record<string, number>,
  blocked_keywords_text: '',
  keyword_blocking_mode: 'keyword_and_api' as KeywordBlockingMode,
  model_filter_type: 'all' as ContentModerationModelFilterType,
  model_filter_models: [] as string[],
  user_filter_type: 'all' as ContentModerationScopeFilterType,
  user_filter_ids: [] as number[],
  account_filter_type: 'all' as ContentModerationScopeFilterType,
  account_filter_ids: [] as number[],
})

const providerDrafts = reactive<Record<ContentModerationAuditProvider, ProviderDraft>>({
  openai_moderations: createProviderDraft('https://api.openai.com', 'omni-moderation-latest', 3000, 2),
  ai_chat: createProviderDraft('https://api.deepseek.com', 'deepseek-v4-flash', 15000, 1),
})

const pagination = reactive({
  page: 1,
  page_size: 20,
  total: 0,
  pages: 1,
})

const filters = reactive({
  result: '',
  group_id: 0,
  endpoint: '',
  search: '',
  from: '',
  to: '',
})

const settingsTabs = computed<Array<{ id: SettingsTab; label: string }>>(() => [
  { id: 'basic', label: t('admin.riskControl.tabs.basic') },
  { id: 'scope', label: t('admin.riskControl.tabs.scope') },
  { id: 'runtime', label: t('admin.riskControl.tabs.runtime') },
  { id: 'response', label: t('admin.riskControl.tabs.response') },
  ...(configForm.audit_provider === 'openai_moderations'
    ? [{ id: 'riskThresholds' as SettingsTab, label: t('admin.riskControl.tabs.riskThresholds') }]
    : []),
  { id: 'keywords', label: t('admin.riskControl.tabs.keywords') },
  { id: 'retention', label: t('admin.riskControl.tabs.retention') },
])

const aiFailurePolicyOptions = computed<SelectOption[]>(() => [
  { value: 'allow', label: t('admin.riskControl.aiFailureAllow') },
  { value: 'block', label: t('admin.riskControl.aiFailureBlock') },
])

const aiReasoningEffortOptions = computed<SelectOption[]>(() => [
  { value: 'adaptive', label: t('admin.riskControl.aiReasoningAdaptive') },
  { value: 'low', label: t('admin.riskControl.aiReasoningLow') },
  { value: 'high', label: t('admin.riskControl.aiReasoningHigh') },
  { value: 'max', label: t('admin.riskControl.aiReasoningMax') },
])

const aiThinkingEnabled = computed({
  get: () => configForm.ai_thinking_mode === 'enabled',
  set: (enabled: boolean) => {
    configForm.ai_thinking_mode = enabled ? 'enabled' : 'disabled'
  },
})

function setAIReasoningEffort(value: string): void {
  if (value === 'adaptive' || value === 'low' || value === 'high' || value === 'max') {
    configForm.ai_reasoning_effort = value
  }
}

function createProviderDraft(baseUrl: string, model: string, timeoutMs: number, retryCount: number): ProviderDraft {
  return {
    base_url: baseUrl,
    model,
    proxy_id: null,
    api_key_configured: false,
    api_key_count: 0,
    api_key_masks: [],
    timeout_ms: timeoutMs,
    retry_count: retryCount,
    api_keys_text: '',
    api_key_statuses: [],
    api_keys_mode: 'append',
    clear_api_key: false,
  }
}

function setProviderProfile(provider: ContentModerationAuditProvider, profile: ContentModerationProviderProfile | undefined) {
  if (!profile) return
  Object.assign(providerDrafts[provider], {
    base_url: profile.base_url,
    model: profile.model,
    proxy_id: profile.proxy_id ?? null,
    api_key_configured: profile.api_key_configured,
    api_key_count: profile.api_key_count,
    api_key_masks: Array.isArray(profile.api_key_masks) ? [...profile.api_key_masks] : [],
    timeout_ms: profile.timeout_ms,
    retry_count: profile.retry_count,
    api_keys_text: '',
    api_key_statuses: [],
    api_keys_mode: 'append',
    clear_api_key: false,
  })
}

function captureProviderDraft(provider = configForm.audit_provider) {
  Object.assign(providerDrafts[provider], {
    base_url: configForm.base_url,
    model: configForm.model,
    proxy_id: configForm.proxy_id,
    api_key_configured: configForm.api_key_configured,
    api_key_count: configForm.api_key_count,
    api_key_masks: [...configForm.api_key_masks],
    timeout_ms: configForm.timeout_ms,
    retry_count: configForm.retry_count,
    api_keys_text: configForm.api_keys_text,
    api_key_statuses: [...configForm.api_key_statuses],
    api_keys_mode: configForm.api_keys_mode,
    clear_api_key: configForm.clear_api_key,
  })
}

function loadProviderDraft(provider: ContentModerationAuditProvider) {
  const draft = providerDrafts[provider]
  configForm.base_url = draft.base_url
  configForm.model = draft.model
  configForm.proxy_id = draft.proxy_id
  configForm.api_keys_text = draft.api_keys_text
  configForm.api_key_configured = draft.api_key_configured
  configForm.api_key_masked = draft.api_key_masks[0] || ''
  configForm.api_key_count = draft.api_key_count
  configForm.api_key_masks = [...draft.api_key_masks]
  configForm.api_key_statuses = [...draft.api_key_statuses]
  configForm.api_keys_mode = draft.api_keys_mode
  configForm.clear_api_key = draft.clear_api_key
  configForm.timeout_ms = draft.timeout_ms
  configForm.retry_count = draft.retry_count
  pendingDeleteApiKeyHashes.value = []
  testedApiKeyStatuses.value = []
  moderationTestResult.value = null
}

function switchAuditProvider(provider: ContentModerationAuditProvider) {
  if (provider === configForm.audit_provider) return
  captureProviderDraft()
  configForm.audit_provider = provider
  loadProviderDraft(provider)
  if (provider === 'ai_chat') {
    moderationTestImages.value = []
  }
  if (activeSettingsTab.value === 'riskThresholds' && provider === 'ai_chat') {
    activeSettingsTab.value = 'basic'
  }
}

function applyRecommendedAIChatPrompt() {
  if (!recommendedAIChatSystemPrompt.value.trim()) return
  configForm.ai_system_prompt = recommendedAIChatSystemPrompt.value
  activeAIChatPromptVersion.value = recommendedAIChatPromptVersion.value
  usesRecommendedAIChatSystemPrompt.value = true
}

function updateAIChatPrompt(prompt: string) {
  configForm.ai_system_prompt = prompt
  const usesRecommended = recommendedAIChatSystemPrompt.value.trim() !== ''
    && prompt.trim() === recommendedAIChatSystemPrompt.value.trim()
  usesRecommendedAIChatSystemPrompt.value = usesRecommended
  activeAIChatPromptVersion.value = usesRecommended
    ? recommendedAIChatPromptVersion.value
    : 'custom'
}

const modeOptions = computed<SelectOption[]>(() => [
  { value: 'pre_block', label: t('admin.riskControl.modePreBlock') },
  { value: 'observe', label: t('admin.riskControl.modeObserve') },
  { value: 'off', label: t('admin.riskControl.modeOff') },
])

const keywordBlockingModeOptions = computed<Array<{ value: KeywordBlockingMode; label: string; description: string }>>(() => [
  {
    value: 'keyword_and_api',
    label: t('admin.riskControl.keywordModeKeywordAndApi'),
    description: t('admin.riskControl.keywordModeKeywordAndApiDesc'),
  },
  {
    value: 'keyword_only',
    label: t('admin.riskControl.keywordModeKeywordOnly'),
    description: t('admin.riskControl.keywordModeKeywordOnlyDesc'),
  },
  {
    value: 'api_only',
    label: t('admin.riskControl.keywordModeApiOnly'),
    description: t('admin.riskControl.keywordModeApiOnlyDesc'),
  },
])

const scopeFilterTypes: ContentModerationScopeFilterType[] = ['all', 'include', 'exclude']
const scopeFilterSections = computed<Array<{
  entity: ScopeEntity
  title: string
  hint: string
  selectorLabel: string
  emptyIncludeWarning: string
}>>(() => [
  {
    entity: 'user',
    title: t('admin.riskControl.userFilter'),
    hint: t('admin.riskControl.userFilterHint'),
    selectorLabel: t('admin.riskControl.userFilterUsers'),
    emptyIncludeWarning: t('admin.riskControl.userFilterEmptyIncludeWarning'),
  },
  {
    entity: 'account',
    title: t('admin.riskControl.accountFilter'),
    hint: t('admin.riskControl.accountFilterHint'),
    selectorLabel: t('admin.riskControl.accountFilterAccounts'),
    emptyIncludeWarning: t('admin.riskControl.accountFilterEmptyIncludeWarning'),
  },
])

const modelFilterOptions = computed<Array<{ value: ContentModerationModelFilterType; label: string; description: string }>>(() => [
  {
    value: 'all',
    label: t('admin.riskControl.modelFilterAll'),
    description: t('admin.riskControl.modelFilterAllDesc'),
  },
  {
    value: 'include',
    label: t('admin.riskControl.modelFilterInclude'),
    description: t('admin.riskControl.modelFilterIncludeDesc'),
  },
  {
    value: 'exclude',
    label: t('admin.riskControl.modelFilterExclude'),
    description: t('admin.riskControl.modelFilterExcludeDesc'),
  },
])

type KeywordNoticeView = {
  title: string
  description: string
  icon: 'infoCircle' | 'exclamationTriangle'
  toneClass: string
  iconClass: string
  titleClass: string
}

const keywordNoticeTones = {
  info: {
    icon: 'infoCircle' as const,
    toneClass: 'border-primary-100 bg-primary-50/60 dark:border-primary-900/40 dark:bg-primary-900/10',
    iconClass: 'mt-0.5 flex-shrink-0 text-primary-500 dark:text-primary-300',
    titleClass: 'text-primary-700 dark:text-primary-200',
  },
  warning: {
    icon: 'exclamationTriangle' as const,
    toneClass: 'border-amber-200 bg-amber-50 dark:border-amber-900/40 dark:bg-amber-900/20',
    iconClass: 'mt-0.5 flex-shrink-0 text-amber-500 dark:text-amber-300',
    titleClass: 'text-amber-700 dark:text-amber-200',
  },
}

const keywordNotice = computed<KeywordNoticeView>(() => {
  const strategy = configForm.keyword_blocking_mode
  if (strategy === 'api_only') {
    return {
      ...keywordNoticeTones.info,
      title: t('admin.riskControl.keywordModeApiOnlyNotice'),
      description: t('admin.riskControl.keywordModeApiOnlyDesc'),
    }
  }
  if (configForm.mode !== 'pre_block') {
    return {
      ...keywordNoticeTones.warning,
      title: t('admin.riskControl.blockedKeywordsModeWarning', { mode: modeLabel(configForm.mode) }),
      description: t('admin.riskControl.blockedKeywordsDescription'),
    }
  }
  if (strategy === 'keyword_only') {
    return {
      ...keywordNoticeTones.info,
      title: t('admin.riskControl.keywordModeKeywordOnlyNotice'),
      description: t('admin.riskControl.keywordModeKeywordOnlyDesc'),
    }
  }
  return {
    ...keywordNoticeTones.info,
    title: t('admin.riskControl.blockedKeywordsPreBlockHint'),
    description: t('admin.riskControl.blockedKeywordsDescription'),
  }
})

const resultOptions = computed<SelectOption[]>(() => [
  { value: '', label: t('admin.riskControl.result.all') },
  { value: 'hit', label: t('admin.riskControl.result.hit') },
  { value: 'blocked', label: t('admin.riskControl.result.blocked') },
  { value: 'pass', label: t('admin.riskControl.result.pass') },
  { value: 'error', label: t('admin.riskControl.result.error') },
])

const endpointOptions = computed<SelectOption[]>(() => [
  { value: '', label: t('admin.riskControl.filters.allEndpoints') },
  { value: '/v1/messages', label: '/v1/messages' },
  { value: '/v1/responses', label: '/v1/responses' },
  { value: '/v1/chat/completions', label: '/v1/chat/completions' },
  { value: '/v1beta/models', label: '/v1beta/models' },
  { value: '/v1/images/generations', label: '/v1/images/generations' },
  { value: '/v1/images/edits', label: '/v1/images/edits' },
])

const groupFilterOptions = computed<SelectOption[]>(() => [
  { value: 0, label: t('admin.riskControl.filters.allGroups') },
  ...groups.value.map((group) => ({
    value: group.id,
    label: `${group.name} (${group.platform})`,
  })),
])

const selectedGroupCount = computed(() => String(configForm.group_ids.length))

const modelFilterModelCount = computed(() => configForm.model_filter_models.length)

const modelFilterSummary = computed(() => {
  if (configForm.model_filter_type === 'include') {
    return t('admin.riskControl.modelFilterIncludeSummary', { count: modelFilterModelCount.value })
  }
  if (configForm.model_filter_type === 'exclude') {
    return t('admin.riskControl.modelFilterExcludeSummary', { count: modelFilterModelCount.value })
  }
  return t('admin.riskControl.modelFilterAllSummary')
})

const modelFilterPreviewModels = computed(() => configForm.model_filter_models.slice(0, 6))

const hiddenModelFilterModelCount = computed(() => Math.max(0, configForm.model_filter_models.length - modelFilterPreviewModels.value.length))

const filteredGroups = computed(() => {
  const keyword = groupSearch.value.trim().toLowerCase()
  if (!keyword) return groups.value
  return groups.value.filter((group) => {
    return group.name.toLowerCase().includes(keyword) || String(group.platform).toLowerCase().includes(keyword)
  })
})

const inputApiKeyCount = computed(() => parseApiKeys(configForm.api_keys_text).length)

const blockedKeywordList = computed(() => parseBlockedKeywords(configForm.blocked_keywords_text))

const blockedKeywordCount = computed(() => blockedKeywordList.value.length)

const pendingDeletedApiKeyCount = computed(() => pendingDeleteApiKeyHashes.value.length)

const effectiveStoredApiKeyCount = computed(() => Math.max(0, configForm.api_key_count - pendingDeletedApiKeyCount.value))

const apiKeysPlaceholder = computed(() => (
  configForm.api_keys_mode === 'replace'
    ? t('admin.riskControl.apiKeysPlaceholderReplace')
    : t('admin.riskControl.apiKeysPlaceholder')
))

const apiKeysModeHint = computed(() => (
  configForm.api_keys_mode === 'replace'
    ? t('admin.riskControl.apiKeysModeReplaceHint')
    : t('admin.riskControl.apiKeysModeAppendHint')
))

const hasModerationAuditInput = computed(() => {
  return moderationTestPrompt.value.trim() !== '' || moderationTestImages.value.length > 0
})

const inputDetailAuditDetails = computed(() => {
  const details = inputDetailRow.value?.audit_details
  return hasMeaningfulAuditDetails(details) ? details : undefined
})
const inputDetailHash = computed(() => inputDetailAuditDetails.value?.input_hash?.trim() || '')
const isFlaggedHashInputValid = computed(() => isValidFlaggedHash(flaggedHashInput.value))
const isInputDetailHashValid = computed(() => isValidFlaggedHash(inputDetailHash.value))

const storedApiKeyTestButtonText = computed(() => {
  if (apiKeyTesting.value) return t('admin.riskControl.testingApiKeys')
  if (hasModerationAuditInput.value) return t('admin.riskControl.testContentWithStoredApiKey')
  return t('admin.riskControl.testStoredApiKeys')
})

const savedApiKeyRows = computed<ContentModerationAPIKeyStatus[]>(() => {
  const rows = status.value?.api_key_statuses?.length
    ? status.value.api_key_statuses
    : configForm.api_key_statuses
  return Array.isArray(rows) ? rows : []
})

const apiKeyRows = computed<ContentModerationAPIKeyStatus[]>(() => [
  ...savedApiKeyRows.value,
  ...testedApiKeyStatuses.value,
])

const visibleApiKeyRows = computed<ContentModerationAPIKeyStatus[]>(() => {
  if (apiKeyRowsExpanded.value) return apiKeyRows.value
  return apiKeyRows.value.slice(0, maxVisibleApiKeyRows)
})

const hiddenApiKeyRowCount = computed<number>(() => Math.max(0, apiKeyRows.value.length - visibleApiKeyRows.value.length))

const canToggleApiKeyRows = computed<boolean>(() => apiKeyRows.value.length > maxVisibleApiKeyRows)

const activeSavedApiKeyRows = computed<ContentModerationAPIKeyStatus[]>(() => (
  savedApiKeyRows.value.filter((row) => !isStoredApiKeyPendingDelete(row))
))

const apiKeyHealthBadges = computed<Array<{ status: ContentModerationAPIKeyStatus['status']; count: number }>>(() => {
  const counts: Record<ContentModerationAPIKeyStatus['status'], number> = {
    ok: 0,
    error: 0,
    frozen: 0,
    unknown: 0,
  }
  for (const row of activeSavedApiKeyRows.value) {
    counts[row.status] = (counts[row.status] ?? 0) + 1
  }
  if (activeSavedApiKeyRows.value.length === 0 && effectiveStoredApiKeyCount.value > 0) {
    counts.unknown = effectiveStoredApiKeyCount.value
  }
  return (['ok', 'frozen', 'error', 'unknown'] as Array<ContentModerationAPIKeyStatus['status']>)
    .map((item) => ({ status: item, count: counts[item] }))
    .filter((item) => item.count > 0)
})

const apiKeyHealthSummary = computed(() => {
  if (!configForm.api_key_configured) return ''
  if (apiKeyHealthBadges.value.length === 0) return t('admin.riskControl.apiKeyStatusUnknown')
  return apiKeyHealthBadges.value
    .map((badge) => `${apiKeyStatusLabel(badge.status)} ${badge.count}`)
    .join(' · ')
})

const overviewItems = computed<OverviewItem[]>(() => [
  {
    key: 'status',
    label: t('admin.riskControl.overview.status'),
    value: configForm.enabled ? t('admin.riskControl.overview.enabled') : t('admin.riskControl.overview.disabled'),
    meta: modeLabel(configForm.mode),
    icon: 'shield',
    iconClass: configForm.enabled
      ? 'bg-emerald-50 text-emerald-600 dark:bg-emerald-900/20 dark:text-emerald-300'
      : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400',
    badge: runtimeBadgeText.value,
    badgeClass: runtimeBadgeClass.value,
  },
  {
    key: 'api-key',
    label: t('admin.riskControl.overview.apiKey'),
    value: configForm.api_key_configured ? t('admin.riskControl.apiKeyCount', { count: configForm.api_key_count }) : t('admin.riskControl.notConfigured'),
    meta: configForm.api_key_configured ? apiKeyHealthSummary.value || configForm.model || '-' : configForm.model || '-',
    icon: 'key',
    iconClass: 'bg-sky-50 text-sky-600 dark:bg-sky-900/20 dark:text-sky-300',
  },
  {
    key: 'scope',
    label: t('admin.riskControl.overview.groupScope'),
    value: configForm.all_groups ? t('admin.riskControl.allGroups') : selectedGroupCount.value,
    meta: modelFilterSummary.value,
    icon: 'users',
    iconClass: 'bg-violet-50 text-violet-600 dark:bg-violet-900/20 dark:text-violet-300',
  },
  {
    key: 'logs',
    label: t('admin.riskControl.overview.logs'),
    value: formatNumber(pagination.total),
    meta: t('admin.riskControl.overview.currentFilter'),
    icon: 'document',
    iconClass: 'bg-amber-50 text-amber-600 dark:bg-amber-900/20 dark:text-amber-300',
  },
])

const moderationScoreRows = computed<ModerationScoreRow[]>(() => {
  const result = moderationTestResult.value
  if (!result) return []
  return Object.entries(result.category_scores || {})
    .map(([category, score]) => {
      const threshold = result.thresholds?.[category] ?? 1
      return {
        category,
        score,
        threshold,
        hit: score >= threshold,
      }
    })
    .sort((a, b) => b.score - a.score)
})

function moderationCategoryLabel(category?: string): string {
  const code = String(category || '').trim()
  if (!code) return '-'
  const label = moderationCategoryChineseLabels[code]
  return label ? `${label}（${code}）` : code
}

const riskThresholdRows = computed<RiskThresholdRow[]>(() => (
  riskThresholdCategories.map((category) => ({
    category,
    value: configForm.thresholds[category] ?? riskThresholdDefaults[category],
    defaultValue: riskThresholdDefaults[category],
  }))
))

const inputDetailText = computed(() => {
  if (!inputDetailRow.value) return '-'
  return inputDetailAuditDetails.value?.audit_target_excerpt || inputDetailRow.value.input_excerpt || inputDetailRow.value.error || '-'
})

const supportingContextText = computed(() => (
  diagnosticText(inputDetailAuditDetails.value?.supporting_context_excerpt)
))

const auditDiagnosticItems = computed(() => {
  const details = inputDetailAuditDetails.value
  if (!details) return []
  const localRule = details.local_rule_match
  const promptTokens = details.prompt_tokens
  const cachedTokens = details.cached_input_tokens
  const uncachedTokens = details.uncached_input_tokens
  const outputTokens = details.output_tokens
  const stages = details.stages ?? []
  const providerStages = stages.filter((stage) => stage.provider_called)
  const legacyLocalOrSkipped = details.provider_applicable === undefined && stages.length === 0 && (
    details.audit_stage === 'local'
    || Boolean(details.local_rule_level)
    || inputDetailRow.value?.audit_code === 'no_new_user_intent'
  )
  const noProviderUsage = details.provider_applicable === false
    || legacyLocalOrSkipped
    || (stages.length > 0 && providerStages.length === 0 && stages.every((stage) => !stage.failed))
  const stageUsageComplete = stages.length > 0
    && providerStages.length > 0
    && providerStages.every((stage) => stage.usage_known && !stage.failed)
  const tokenFieldsComplete = isKnownAuditCount(promptTokens)
    && isKnownAuditCount(cachedTokens)
    && isKnownAuditCount(uncachedTokens)
    && isKnownAuditCount(outputTokens)
    && promptTokens === cachedTokens + uncachedTokens
  const hasCompleteTokenUsage = tokenFieldsComplete
    && !noProviderUsage
    && (stages.length > 0 ? stageUsageComplete : details.usage_unknown !== true)
  let tokenText = noProviderUsage
    ? t('admin.riskControl.auditStageNoProviderUsage')
    : t('admin.riskControl.usageUnknown')
  if (hasCompleteTokenUsage) {
    tokenText = t('admin.riskControl.auditTokenSummary', {
      prompt: formatNumber(promptTokens),
      cached: formatNumber(cachedTokens),
      uncached: formatNumber(uncachedTokens),
      output: formatNumber(outputTokens),
    })
  }
  const resultCacheHit = details.sub2api_result_cache_hit ?? details.result_cache_hit
  const resultCacheApplicable = details.result_cache_applicable
  const providerCacheRatio = details.provider_prefix_cache_ratio
  const providerCacheText = typeof providerCacheRatio === 'number'
    && Number.isFinite(providerCacheRatio)
    && providerCacheRatio >= 0
    && providerCacheRatio <= 1
    ? percent(providerCacheRatio)
    : t('common.unknown')
  const usageCompletenessText = noProviderUsage
    ? t('admin.riskControl.auditStageNotApplicable')
    : hasCompleteTokenUsage
      ? t('admin.riskControl.usageComplete')
      : details.usage_unknown === true || (stages.length > 0 && !stageUsageComplete)
        ? t('admin.riskControl.usageUnknown')
        : t('common.unknown')
  const prefixStatusText = details.prefix_baseline === true
    ? t('admin.riskControl.auditPrefixBaseline')
    : details.prefix_continuity === true
      ? t('common.yes')
      : details.prefix_continuity === false || Boolean(details.prefix_break_reason)
        ? t('common.no')
        : t('common.unknown')
  const prefixBreakReasonText = details.prefix_baseline === true
    ? t('admin.riskControl.auditPrefixBaseline')
    : diagnosticText(details.prefix_break_reason)
  return [
    { key: 'latency-total', label: t('admin.riskControl.auditLatencyTotal'), value: auditLatencyText(details.total_latency_ms) },
    { key: 'latency-extraction', label: t('admin.riskControl.auditLatencyExtraction'), value: auditLatencyText(details.extraction_latency_ms) },
    { key: 'latency-provenance', label: t('admin.riskControl.auditLatencyProvenance'), value: auditLatencyText(details.provenance_latency_ms) },
    { key: 'latency-deterministic', label: t('admin.riskControl.auditLatencyDeterministic'), value: auditLatencyText(details.deterministic_latency_ms) },
    { key: 'latency-verdict-cache', label: t('admin.riskControl.auditLatencyVerdictCache'), value: auditLatencyText(details.verdict_cache_latency_ms) },
    { key: 'latency-context-load', label: t('admin.riskControl.auditLatencyContextLoad'), value: auditLatencyText(details.context_load_latency_ms) },
    { key: 'latency-fast-build', label: t('admin.riskControl.auditLatencyFastBuild'), value: auditLatencyText(details.fast_build_latency_ms) },
    { key: 'latency-review-build', label: t('admin.riskControl.auditLatencyReviewBuild'), value: auditLatencyText(details.review_build_latency_ms) },
    { key: 'latency-provider', label: t('admin.riskControl.auditLatencyProvider'), value: auditLatencyText(details.provider_latency_ms) },
    { key: 'latency-postprocess', label: t('admin.riskControl.auditLatencyPostprocess'), value: auditLatencyText(details.postprocess_latency_ms) },
    { key: 'stage', label: t('admin.riskControl.auditStage'), value: diagnosticText(details.audit_stage) },
    { key: 'target', label: t('admin.riskControl.auditTargetType'), value: diagnosticPair(details.audit_target_kind, details.audit_target_source) },
    { key: 'session', label: t('admin.riskControl.auditSession'), value: diagnosticPair(details.session_source, isKnownAuditCount(details.turn_count) ? formatNumber(details.turn_count) : undefined) },
    { key: 'input-chars', label: t('admin.riskControl.auditInputChars'), value: isKnownAuditCount(details.input_chars) ? formatNumber(details.input_chars) : t('common.unknown') },
    { key: 'tokens', label: t('admin.riskControl.auditTokens'), value: tokenText },
    { key: 'usage-completeness', label: t('admin.riskControl.auditUsageCompleteness'), value: usageCompletenessText },
    { key: 'cache', label: t('admin.riskControl.auditCache'), value: resultCacheApplicable === false || noProviderUsage ? t('admin.riskControl.auditStageNotApplicable') : resultCacheHit === undefined ? t('common.unknown') : resultCacheHit ? t('admin.riskControl.cacheHit') : t('admin.riskControl.cacheMiss') },
    { key: 'provider-cache', label: t('admin.riskControl.auditProviderCache'), value: noProviderUsage ? t('admin.riskControl.auditStageNotApplicable') : providerCacheText },
    { key: 'review-complete', label: t('admin.riskControl.auditReviewComplete'), value: details.review_applicable === false || noProviderUsage ? t('admin.riskControl.auditStageNotApplicable') : optionalBooleanText(details.review_complete) },
    { key: 'explicit-user', label: t('admin.riskControl.auditExplicitUserTurn'), value: optionalBooleanText(details.has_explicit_user_turn) },
    { key: 'trusted-client', label: t('admin.riskControl.auditTrustedClient'), value: optionalBooleanText(details.trusted_client) },
    { key: 'escalation', label: t('admin.riskControl.auditEscalation'), value: diagnosticList(details.escalation_reasons) },
    { key: 'input-hash', label: t('admin.riskControl.auditInputHash'), value: diagnosticText(details.input_hash) },
    { key: 'hash-scope', label: t('admin.riskControl.auditHashScope'), value: diagnosticText(details.hash_scope) },
    { key: 'hash', label: t('admin.riskControl.auditHash'), value: diagnosticPair(details.hash_state, details.hash_promotion_reason) },
    { key: 'policy-version', label: t('admin.riskControl.auditPolicyVersion'), value: diagnosticText(details.policy_version) },
    { key: 'audit-key-hash', label: t('admin.riskControl.auditKeyHash'), value: diagnosticText(details.audit_key_hash) },
    { key: 'prefix', label: t('admin.riskControl.auditPrefix'), value: diagnosticPair(isKnownAuditCount(details.prefix_epoch) ? formatNumber(details.prefix_epoch) : undefined, prefixStatusText) },
    { key: 'prefix-break-reason', label: t('admin.riskControl.auditPrefixBreakReason'), value: prefixBreakReasonText },
    { key: 'input-truncated', label: t('admin.riskControl.auditInputTruncated'), value: optionalBooleanText(details.input_truncated) },
    { key: 'trusted-signals', label: t('admin.riskControl.auditTrustedSignals'), value: diagnosticList(details.trusted_signals) },
    { key: 'ignored-metadata', label: t('admin.riskControl.auditIgnoredMetadata'), value: diagnosticList(details.ignored_metadata) },
    { key: 'signals', label: t('admin.riskControl.auditSignals'), value: diagnosticList(details.model_signals) },
    { key: 'local-rule-identity', label: t('admin.riskControl.auditLocalRuleIdentity'), value: diagnosticPair(localRule?.rule_id, localRule?.rule_version) },
    { key: 'local-rule-level', label: t('admin.riskControl.auditLocalRuleLevel'), value: diagnosticText(details.local_rule_level || localRule?.level) },
    { key: 'local-rule-target-type', label: t('admin.riskControl.auditLocalRuleTargetType'), value: diagnosticPair(localRule?.target_kind, localRule?.target_source) },
    { key: 'local-rule-intent', label: t('admin.riskControl.auditLocalRuleIntent'), value: diagnosticList(localRule?.matched_intent) },
    { key: 'local-rule-target', label: t('admin.riskControl.auditLocalRuleTarget'), value: diagnosticList(localRule?.matched_target) },
    { key: 'local-rule-action', label: t('admin.riskControl.auditLocalRuleAction'), value: diagnosticList(localRule?.matched_action) },
    { key: 'local-rule-excerpt', label: t('admin.riskControl.auditLocalRuleExcerpt'), value: diagnosticText(localRule?.matched_excerpt) },
    { key: 'lexical-types', label: t('admin.riskControl.auditLexicalTypes'), value: diagnosticList(localRule?.lexical_types) },
    { key: 'negation-detected', label: t('admin.riskControl.auditNegationDetected'), value: optionalBooleanText(localRule?.negation_detected) },
    { key: 'defensive-detected', label: t('admin.riskControl.auditDefensiveDetected'), value: optionalBooleanText(localRule?.defensive_detected) },
    { key: 'local-rule-metadata-excluded', label: t('admin.riskControl.auditLocalRuleMetadataExcluded'), value: diagnosticList(localRule?.metadata_excluded) },
  ]
})

const hasAuditDiagnostics = computed(() => auditDiagnosticItems.value.length > 0)

const legacyAuditDetailFalseDefaults = new Set<keyof ContentModerationAuditDetails>([
  'sub2api_result_cache_hit',
  'review_complete',
  'has_explicit_user_turn',
  'trusted_client',
])

function hasMeaningfulAuditDetails(details: ContentModerationAuditDetails | undefined): details is ContentModerationAuditDetails {
  if (!details) return false
  return (Object.entries(details) as [keyof ContentModerationAuditDetails, unknown][]).some(([key, value]) => {
    if (value === undefined || value === null) return false
    if (legacyAuditDetailFalseDefaults.has(key) && value === false) return false
    if (typeof value === 'string') return value.trim().length > 0
    if (Array.isArray(value)) return value.length > 0
    if (typeof value === 'object') return Object.keys(value).length > 0
    return true
  })
}

function isKnownAuditCount(value: number | undefined): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0
}

function optionalBooleanText(value: boolean | undefined): string {
  if (value === undefined) return t('common.unknown')
  return value ? t('common.yes') : t('common.no')
}

function diagnosticText(value: unknown): string {
  if (typeof value === 'number' && Number.isFinite(value)) return String(value)
  if (typeof value !== 'string') return t('common.unknown')
  return value.trim() || t('common.unknown')
}

function diagnosticPair(first: unknown, second: unknown): string {
  return `${diagnosticText(first)} / ${diagnosticText(second)}`
}

function diagnosticList(values: string[] | undefined): string {
  if (!Array.isArray(values)) return t('common.unknown')
  const normalized = values.map((value) => value.trim()).filter(Boolean)
  return normalized.length > 0 ? normalized.join(', ') : t('common.none')
}

function isValidFlaggedHash(value: string): boolean {
  return /^[a-fA-F0-9]{64}$/.test(value.trim())
}

const queueUsagePercent = computed(() => `${Math.min(100, Math.max(0, status.value?.queue_usage_percent ?? 0)).toFixed(1)}%`)

const queueUsageStyle = computed(() => ({
  width: queueUsagePercent.value,
}))

const runtimeMode = computed<ModerationMode>(() => status.value?.mode ?? configForm.mode)

const showPreBlockRuntimeCard = computed(() => runtimeMode.value === 'pre_block')

const showWorkerRuntimeCard = computed(() => runtimeMode.value === 'observe')

const preBlockMetricItems = computed(() => [
  {
    key: 'active',
    label: t('admin.riskControl.preBlockActive'),
    value: formatNumber(status.value?.pre_block_active ?? 0),
    meta: t('admin.riskControl.preBlockActiveHint'),
    class: 'bg-sky-50 dark:bg-sky-900/10',
    valueClass: 'text-sky-700 dark:text-sky-300',
  },
  {
    key: 'checked',
    label: t('admin.riskControl.preBlockChecked'),
    value: formatNumber(status.value?.pre_block_checked ?? 0),
    meta: t('admin.riskControl.preBlockCheckedHint'),
    class: 'bg-gray-50 dark:bg-dark-700/50',
    valueClass: 'text-gray-900 dark:text-white',
  },
  {
    key: 'allowed',
    label: t('admin.riskControl.preBlockAllowed'),
    value: formatNumber(status.value?.pre_block_allowed ?? 0),
    meta: t('admin.riskControl.preBlockAllowedHint'),
    class: 'bg-emerald-50 dark:bg-emerald-900/10',
    valueClass: 'text-emerald-700 dark:text-emerald-300',
  },
  {
    key: 'blocked',
    label: t('admin.riskControl.preBlockBlocked'),
    value: formatNumber(status.value?.pre_block_blocked ?? 0),
    meta: t('admin.riskControl.preBlockBlockedHint'),
    class: 'bg-rose-50 dark:bg-rose-900/10',
    valueClass: 'text-rose-700 dark:text-rose-300',
  },
  {
    key: 'errors',
    label: t('admin.riskControl.preBlockErrors'),
    value: formatNumber(status.value?.pre_block_errors ?? 0),
    meta: t('admin.riskControl.preBlockErrorsHint'),
    class: 'bg-amber-50 dark:bg-amber-900/10',
    valueClass: 'text-amber-700 dark:text-amber-300',
  },
  {
    key: 'latency',
    label: t('admin.riskControl.preBlockAvgLatency'),
    value: `${formatNumber(status.value?.pre_block_avg_latency_ms ?? 0)} ms`,
    meta: t('admin.riskControl.preBlockAvgLatencyHint'),
    class: 'bg-violet-50 dark:bg-violet-900/10',
    valueClass: 'text-violet-700 dark:text-violet-300',
  },
])

const preBlockAPIKeyLoads = computed<ContentModerationAPIKeyLoad[]>(() => (
  [...(status.value?.pre_block_api_key_loads ?? [])].sort((a, b) => a.index - b.index)
))

const preBlockAPIKeyMaxTotal = computed(() => Math.max(1, ...preBlockAPIKeyLoads.value.map((item) => item.total || 0)))

const preBlockAPIKeyLoadSummaryText = computed(() => t('admin.riskControl.preBlockAPIKeyLoadSummary', {
  active: formatNumber(status.value?.pre_block_api_key_active ?? 0),
  available: formatNumber(status.value?.pre_block_api_key_available_count ?? 0),
  total: formatNumber(status.value?.pre_block_api_key_total_calls ?? 0),
  workerActive: formatNumber(status.value?.active_workers ?? 0),
  workerTotal: formatNumber(status.value?.worker_count ?? configForm.worker_count),
}))

function preBlockAPIKeyLoadWidth(total: number): string {
  return `${Math.min(100, Math.max(0, (total / preBlockAPIKeyMaxTotal.value) * 100)).toFixed(1)}%`
}

const workerSlots = computed(() => {
  const total = Math.max(0, status.value?.worker_count ?? configForm.worker_count)
  const active = Math.max(0, status.value?.active_workers ?? 0)
  const enabled = Boolean(status.value?.risk_control_enabled && status.value?.enabled && status.value?.mode !== 'off')
  return Array.from({ length: total }, (_, index) => ({
    id: index + 1,
    state: (!enabled ? 'disabled' : index < active ? 'active' : 'idle') as WorkerSlotState,
    label: !enabled
      ? t('admin.riskControl.workerDisabled')
      : index < active
        ? t('admin.riskControl.workerActive')
        : t('admin.riskControl.workerIdle'),
  }))
})

const runtimeBadgeText = computed(() => {
  if (!status.value?.risk_control_enabled) return t('admin.riskControl.riskSwitchOff')
  if (!configForm.enabled || configForm.mode === 'off') return t('admin.riskControl.overview.disabled')
  return t('admin.riskControl.overview.enabled')
})

const runtimeBadgeClass = computed(() => {
  if (!status.value?.risk_control_enabled || !configForm.enabled || configForm.mode === 'off') {
    return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
  }
  return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300'
})

function applyConfig(config: ContentModerationConfig) {
  configForm.enabled = config.enabled
  configForm.mode = config.mode
  const auditProvider: ContentModerationAuditProvider = config.audit_provider === 'ai_chat' ? 'ai_chat' : 'openai_moderations'
  const legacyProfile: ContentModerationProviderProfile = {
    base_url: config.base_url || 'https://api.openai.com',
    model: config.model || 'omni-moderation-latest',
    proxy_id: config.proxy_id ?? null,
    api_key_configured: config.api_key_configured,
    api_key_count: config.api_key_count || 0,
    api_key_masks: Array.isArray(config.api_key_masks) ? [...config.api_key_masks] : [],
    timeout_ms: config.timeout_ms || 3000,
    retry_count: config.retry_count ?? 2,
  }
  setProviderProfile('openai_moderations', config.openai_moderations ?? (auditProvider === 'openai_moderations' ? legacyProfile : undefined))
  setProviderProfile('ai_chat', config.ai_chat ?? (auditProvider === 'ai_chat' ? legacyProfile : undefined))
  configForm.audit_provider = auditProvider
  configForm.ai_confidence_threshold = config.ai_chat?.confidence_threshold ?? 0.7
  configForm.ai_cache_enabled = config.ai_chat?.cache_enabled ?? true
  configForm.ai_cache_ttl_seconds = config.ai_chat?.cache_ttl_seconds ?? 300
  configForm.ai_system_prompt = config.ai_chat?.system_prompt || ''
  recommendedAIChatSystemPrompt.value = config.ai_chat?.recommended_system_prompt || ''
  recommendedAIChatPromptVersion.value = config.ai_chat?.recommended_prompt_version || ''
  activeAIChatPromptVersion.value = config.ai_chat?.system_prompt_version || ''
  usesRecommendedAIChatSystemPrompt.value = config.ai_chat?.uses_recommended_system_prompt ?? false
  configForm.ai_failure_policy = config.ai_chat?.failure_policy === 'block' ? 'block' : 'allow'
  configForm.ai_max_input_chars = config.ai_chat?.max_input_chars ?? 200000
  configForm.ai_synchronous_budget_ms = config.ai_chat?.synchronous_budget_ms ?? 4800
  configForm.ai_fast_stage_budget_ms = config.ai_chat?.fast_stage_budget_ms ?? 3000
  configForm.ai_fast_input_chars = config.ai_chat?.fast_input_chars ?? 3000
  configForm.ai_fallback_input_chars = config.ai_chat?.fallback_input_chars ?? 3000
  configForm.ai_thinking_mode = config.ai_chat?.thinking_mode === 'disabled' ? 'disabled' : 'enabled'
  const savedReasoningEffort = config.ai_chat?.reasoning_effort
  configForm.ai_reasoning_effort = savedReasoningEffort === 'adaptive' || savedReasoningEffort === 'low' || savedReasoningEffort === 'high' || savedReasoningEffort === 'max'
    ? savedReasoningEffort
    : 'adaptive'
  configForm.ai_risk_levels_enabled = config.ai_chat?.risk_levels_enabled ?? true
  configForm.ai_observe_threshold = config.ai_chat?.observe_threshold ?? 0.35
  configForm.ai_session_risk_enabled = config.ai_chat?.session_risk_enabled ?? true
  configForm.ai_session_risk_ttl_minutes = config.ai_chat?.session_risk_ttl_minutes ?? 120
  configForm.ai_session_risk_half_life_minutes = config.ai_chat?.session_risk_half_life_minutes ?? 30
  configForm.ai_session_risk_block_cooldown_minutes = config.ai_chat?.session_risk_block_cooldown_minutes ?? 30
  configForm.ai_actor_risk_enabled = config.ai_chat?.actor_risk_enabled ?? true
  configForm.ai_incremental_audit_enabled = config.ai_chat?.incremental_audit_enabled ?? false
  configForm.ai_input_provenance_v2_enabled = config.ai_chat?.input_provenance_v2_enabled ?? true
  configForm.ai_deterministic_risk_v2_enabled = config.ai_chat?.deterministic_risk_v2_enabled ?? true
  configForm.ai_recent_user_turns = config.ai_chat?.recent_user_turns ?? 2
  configForm.ai_summary_max_chars = config.ai_chat?.summary_max_chars ?? 500
  configForm.ai_full_review_threshold = config.ai_chat?.full_review_threshold ?? 0.4
  configForm.ai_full_review_risk_delta = config.ai_chat?.full_review_risk_delta ?? 0.15
  configForm.ai_periodic_full_review_turns = config.ai_chat?.periodic_full_review_turns ?? 10
  configForm.ai_full_review_max_input_chars = config.ai_chat?.full_review_max_input_chars ?? Math.min(60000, configForm.ai_max_input_chars)
  configForm.ai_fast_max_output_tokens = config.ai_chat?.fast_max_output_tokens ?? 128
  configForm.ai_full_max_output_tokens = config.ai_chat?.full_max_output_tokens ?? 1024
  configForm.ai_max_review_max_output_tokens = config.ai_chat?.max_review_max_output_tokens ?? 1536
  configForm.ai_audit_context_ttl_minutes = config.ai_chat?.audit_context_ttl_minutes ?? 120
  configForm.ai_pricing_configured = config.ai_chat?.pricing_configured ?? false
  configForm.ai_pricing_version = config.ai_chat?.pricing_version ?? ''
  configForm.ai_uncached_input_usd_per_million_tokens = config.ai_chat?.uncached_input_usd_per_million_tokens ?? null
  configForm.ai_cached_input_usd_per_million_tokens = config.ai_chat?.cached_input_usd_per_million_tokens ?? null
  configForm.ai_output_usd_per_million_tokens = config.ai_chat?.output_usd_per_million_tokens ?? null
  loadProviderDraft(auditProvider)
  configForm.api_key_statuses = Array.isArray(config.api_key_statuses) ? [...config.api_key_statuses] : []
  providerDrafts[auditProvider].api_key_statuses = [...configForm.api_key_statuses]
  pendingDeleteApiKeyHashes.value = []
  testedApiKeyStatuses.value = []
  apiKeyRowsExpanded.value = false
  configForm.sample_rate = config.sample_rate ?? 100
  configForm.all_groups = config.all_groups
  configForm.group_ids = Array.isArray(config.group_ids) ? [...config.group_ids] : []
  configForm.record_non_hits = config.record_non_hits
  configForm.worker_count = config.worker_count || 4
  configForm.queue_size = config.queue_size || 32768
  configForm.block_status = config.block_status || 403
  configForm.block_message = config.block_message || defaultBlockMessage()
  configForm.email_on_hit = config.email_on_hit ?? true
  configForm.auto_ban_enabled = config.auto_ban_enabled ?? true
  configForm.cyber_policy_exclude_from_ban_count = config.cyber_policy_exclude_from_ban_count ?? false
  configForm.ban_threshold = config.ban_threshold || 10
  configForm.violation_window_hours = config.violation_window_hours || 720
  configForm.hit_retention_days = config.hit_retention_days || 180
  configForm.non_hit_retention_days = Math.min(Math.max(config.non_hit_retention_days || 3, 1), 3)
  configForm.pre_hash_check_enabled = config.pre_hash_check_enabled ?? false
  configForm.thresholds = riskThresholdsFromConfig(config.thresholds)
  configForm.blocked_keywords_text = Array.isArray(config.blocked_keywords) ? config.blocked_keywords.join('\n') : ''
  configForm.keyword_blocking_mode = normalizeKeywordBlockingMode(config.keyword_blocking_mode)
  const modelFilter = normalizeModelFilter(config.model_filter)
  configForm.model_filter_type = modelFilter.type
  configForm.model_filter_models = modelFilter.models
  const userFilter = normalizeUserFilter(config.user_filter)
  configForm.user_filter_type = userFilter.type
  configForm.user_filter_ids = userFilter.user_ids
  const accountFilter = normalizeAccountFilter(config.account_filter)
  configForm.account_filter_type = accountFilter.type
  configForm.account_filter_ids = accountFilter.account_ids
  savedConfigSnapshot.value = JSON.parse(JSON.stringify(config)) as ContentModerationConfig
}

async function loadAll() {
  loading.value = true
  try {
    const [config, groupItems, runtimeStatus, proxyItems] = await Promise.all([
      adminAPI.riskControl.getConfig(),
      adminAPI.groups.getAll(),
      adminAPI.riskControl.getStatus(),
      // 代理列表加载失败不阻塞风控页面（仅影响下拉可选项）
      adminAPI.proxies.getAll().catch(() => [] as Proxy[]),
    ])
    applyConfig(config)
    groups.value = groupItems
    status.value = runtimeStatus
    proxies.value = proxyItems
    if (Array.isArray(runtimeStatus.api_key_statuses)) {
      configForm.api_key_statuses = [...runtimeStatus.api_key_statuses]
      prunePendingDeleteAPIKeyHashes()
    }
    await loadLogs()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.loadFailed')))
  } finally {
    loading.value = false
  }
}

async function loadStatus(silent = true) {
  statusLoading.value = true
  try {
    const runtimeStatus = await adminAPI.riskControl.getStatus()
    status.value = runtimeStatus
    if (Array.isArray(runtimeStatus.api_key_statuses)) {
      configForm.api_key_statuses = [...runtimeStatus.api_key_statuses]
      prunePendingDeleteAPIKeyHashes()
    }
  } catch (err: unknown) {
    if (!silent) {
      appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.statusFailed')))
    }
  } finally {
    statusLoading.value = false
  }
}

function validateAIChatPerformanceSettings(): boolean {
  if (configForm.audit_provider !== 'ai_chat') return true
  const budget = Number(configForm.ai_synchronous_budget_ms)
  const fastStageBudget = Number(configForm.ai_fast_stage_budget_ms)
  const maxInput = Number(configForm.ai_max_input_chars)
  const fastInput = Number(configForm.ai_fast_input_chars)
  const fallbackInput = Number(configForm.ai_fallback_input_chars)
  if (!Number.isInteger(budget) || budget < 500 || budget > 5000) {
    appStore.showError(t('admin.riskControl.aiPerformanceBudgetInvalid'))
    return false
  }
  if (!Number.isInteger(fastStageBudget) || fastStageBudget < 500 || fastStageBudget > 3000 || fastStageBudget > budget) {
    appStore.showError(t('admin.riskControl.aiFastStageBudgetInvalid'))
    return false
  }
  if (!Number.isInteger(fastInput) || fastInput <= 0 || fastInput > maxInput) {
    appStore.showError(t('admin.riskControl.aiPerformanceFastInputInvalid'))
    return false
  }
  if (!Number.isInteger(fallbackInput) || fallbackInput <= 0 || fallbackInput > fastInput) {
    appStore.showError(t('admin.riskControl.aiPerformanceFallbackInvalid'))
    return false
  }
  return true
}

function validateIncrementalAuditSettings(): boolean {
  if (configForm.audit_provider !== 'ai_chat') return true
  const errorKey = incrementalAuditSettingsRef.value?.validate()
  if (!errorKey) return true
  appStore.showError(t(errorKey))
  return false
}

function validateAIChatPricingSettings(): boolean {
  if (!configForm.ai_pricing_configured) return true
  const version = configForm.ai_pricing_version.trim()
  if (version.length < 1 || version.length > 100) {
    appStore.showError(t('admin.riskControl.aiPricingVersionInvalid'))
    return false
  }
  const rates = [
    configForm.ai_uncached_input_usd_per_million_tokens,
    configForm.ai_cached_input_usd_per_million_tokens,
    configForm.ai_output_usd_per_million_tokens,
  ]
  if (rates.some((value) => typeof value !== 'number' || !Number.isFinite(value) || value < 0 || value > 1000000)) {
    appStore.showError(t('admin.riskControl.aiPricingRateInvalid'))
    return false
  }
  return true
}

function auditPricingRatePayload(value: number | null): number | undefined {
  return typeof value === 'number' && Number.isFinite(value) ? value : undefined
}

async function saveConfig() {
  saving.value = true
  try {
    captureProviderDraft()
    const modelFilterPayload = buildModelFilterPayload()
    if (modelFilterPayload.type !== 'all' && modelFilterPayload.models.length === 0) {
      appStore.showError(t('admin.riskControl.modelFilterModelsRequired'))
      return
    }
    if (!validateAIChatPerformanceSettings()) return
    if (!validateAIChatPricingSettings()) return
    if (!validateIncrementalAuditSettings()) return
    const payload: UpdateContentModerationConfig = {
      enabled: configForm.enabled,
      mode: configForm.mode,
      audit_provider: configForm.audit_provider,
      base_url: configForm.base_url,
      model: configForm.model,
      // 后端语义：0 清除代理（直连），>0 指定代理
      proxy_id: configForm.proxy_id ?? 0,
      timeout_ms: Number(configForm.timeout_ms) || 3000,
      retry_count: Number(configForm.retry_count) || 0,
      ai_confidence_threshold: Number(configForm.ai_confidence_threshold) || 0.7,
      ai_cache_enabled: configForm.ai_cache_enabled,
      ai_cache_ttl_seconds: Number(configForm.ai_cache_ttl_seconds) || 300,
      ai_system_prompt: configForm.ai_system_prompt,
      ai_failure_policy: configForm.ai_failure_policy,
      ai_max_input_chars: Number(configForm.ai_max_input_chars) || 200000,
      ai_synchronous_budget_ms: Number(configForm.ai_synchronous_budget_ms) || 4800,
      ai_fast_stage_budget_ms: Number(configForm.ai_fast_stage_budget_ms) || 3000,
      ai_fast_input_chars: Number(configForm.ai_fast_input_chars) || 3000,
      ai_fallback_input_chars: Number(configForm.ai_fallback_input_chars) || 3000,
      ai_thinking_mode: configForm.ai_thinking_mode,
      ai_reasoning_effort: configForm.ai_reasoning_effort,
      ai_risk_levels_enabled: configForm.ai_risk_levels_enabled,
      ai_observe_threshold: Number(configForm.ai_observe_threshold) || 0.35,
      ai_session_risk_enabled: configForm.ai_session_risk_enabled,
      ai_session_risk_ttl_minutes: Number(configForm.ai_session_risk_ttl_minutes) || 120,
      ai_session_risk_half_life_minutes: Number(configForm.ai_session_risk_half_life_minutes) || 30,
      ai_session_risk_block_cooldown_minutes: Number(configForm.ai_session_risk_block_cooldown_minutes) || 0,
      ai_actor_risk_enabled: configForm.ai_actor_risk_enabled,
      ai_incremental_audit_enabled: configForm.ai_incremental_audit_enabled,
      ai_input_provenance_v2_enabled: configForm.ai_input_provenance_v2_enabled,
      ai_deterministic_risk_v2_enabled: configForm.ai_deterministic_risk_v2_enabled,
      ai_recent_user_turns: Number(configForm.ai_recent_user_turns) || 2,
      ai_summary_max_chars: Number(configForm.ai_summary_max_chars) || 500,
      ai_full_review_threshold: Number(configForm.ai_full_review_threshold) || 0.4,
      ai_full_review_risk_delta: Number(configForm.ai_full_review_risk_delta) || 0.15,
      ai_periodic_full_review_turns: Number(configForm.ai_periodic_full_review_turns) || 10,
      ai_full_review_max_input_chars: Number(configForm.ai_full_review_max_input_chars) || 60000,
      ai_fast_max_output_tokens: Number(configForm.ai_fast_max_output_tokens) || 128,
      ai_full_max_output_tokens: Number(configForm.ai_full_max_output_tokens) || 1024,
      ai_max_review_max_output_tokens: Number(configForm.ai_max_review_max_output_tokens) || 1536,
      ai_audit_context_ttl_minutes: Number(configForm.ai_audit_context_ttl_minutes) || 120,
      ai_pricing_configured: configForm.ai_pricing_configured,
      ai_pricing_version: configForm.ai_pricing_configured ? configForm.ai_pricing_version.trim() : '',
      ai_uncached_input_usd_per_million_tokens: configForm.ai_pricing_configured
        ? auditPricingRatePayload(configForm.ai_uncached_input_usd_per_million_tokens)
        : undefined,
      ai_cached_input_usd_per_million_tokens: configForm.ai_pricing_configured
        ? auditPricingRatePayload(configForm.ai_cached_input_usd_per_million_tokens)
        : undefined,
      ai_output_usd_per_million_tokens: configForm.ai_pricing_configured
        ? auditPricingRatePayload(configForm.ai_output_usd_per_million_tokens)
        : undefined,
      sample_rate: Number(configForm.sample_rate) || 0,
      all_groups: configForm.all_groups,
      group_ids: configForm.all_groups ? [] : [...configForm.group_ids],
      record_non_hits: configForm.record_non_hits,
      clear_api_key: configForm.clear_api_key,
      worker_count: Number(configForm.worker_count) || 4,
      queue_size: Number(configForm.queue_size) || 32768,
      block_status: Number(configForm.block_status) || 403,
      block_message: configForm.block_message || defaultBlockMessage(),
      email_on_hit: configForm.email_on_hit,
      auto_ban_enabled: configForm.auto_ban_enabled,
      cyber_policy_exclude_from_ban_count: configForm.cyber_policy_exclude_from_ban_count,
      ban_threshold: Number(configForm.ban_threshold) || 10,
      violation_window_hours: Number(configForm.violation_window_hours) || 720,
      hit_retention_days: Number(configForm.hit_retention_days) || 180,
      non_hit_retention_days: Math.min(Math.max(Number(configForm.non_hit_retention_days) || 3, 1), 3),
      pre_hash_check_enabled: configForm.pre_hash_check_enabled,
      thresholds: buildRiskThresholdPayload(),
      blocked_keywords: blockedKeywordList.value,
      keyword_blocking_mode: configForm.keyword_blocking_mode,
      model_filter: modelFilterPayload,
      user_filter: buildUserFilterPayload(),
      account_filter: buildAccountFilterPayload(),
    }
    const keys = parseApiKeys(configForm.api_keys_text)
    if (!payload.clear_api_key && configForm.api_keys_mode === 'replace' && keys.length === 0) {
      appStore.showError(t('admin.riskControl.apiKeysReplaceNoInput'))
      return
    }
    if (keys.length > 0) {
      payload.api_keys = keys
      payload.api_keys_mode = configForm.api_keys_mode
      payload.clear_api_key = false
    }
    if (!payload.clear_api_key && configForm.api_keys_mode !== 'replace' && pendingDeleteApiKeyHashes.value.length > 0) {
      payload.delete_api_key_hashes = [...pendingDeleteApiKeyHashes.value]
    }

    const updated = await adminAPI.riskControl.updateConfig(payload)
    applyConfig(updated)
    settingsOpen.value = false
    appStore.showSuccess(t('admin.riskControl.saved'))
    await Promise.all([loadStatus(true), loadLogs()])
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.saveFailed')))
  } finally {
    saving.value = false
  }
}

async function loadLogs() {
  logsLoading.value = true
  try {
    const params = {
      page: pagination.page,
      page_size: pagination.page_size,
      result: filters.result || undefined,
      group_id: filters.group_id || undefined,
      endpoint: filters.endpoint || undefined,
      search: filters.search || undefined,
      from: normalizeDateTimeLocal(filters.from),
      to: normalizeDateTimeLocal(filters.to),
    }
    const result = await adminAPI.riskControl.listLogs(params)
    logs.value = result.items
    pagination.total = result.total
    pagination.page = result.page
    pagination.page_size = result.page_size
    pagination.pages = result.pages
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.logsFailed')))
  } finally {
    logsLoading.value = false
  }
}

function canUnbanRow(row: ContentModerationLog): boolean {
  return Boolean(row.moderation_ban_active && row.user_id && row.user_status === 'disabled')
}

function unbanUnavailableReason(row: ContentModerationLog): string {
  if (row.user_status !== 'disabled' || row.moderation_ban_active) return ''
  return row.unban_block_reason || t('admin.riskControl.unbanNotModerationOwned')
}

function inputSummaryText(row: ContentModerationLog): string {
  return row.input_excerpt || row.error || '-'
}

function openInputDetail(row: ContentModerationLog) {
  inputDetailRow.value = row
}

function closeInputDetail() {
  inputDetailRow.value = null
}

function openUnbanDialog(row: ContentModerationLog) {
  if (!canUnbanRow(row) || unbanningUserID.value !== null) return
  unbanWarning.value = ''
  unbanDialogRow.value = row
}

function closeUnbanDialog() {
  if (unbanningUserID.value !== null) return
  unbanDialogRow.value = null
  unbanWarning.value = ''
}

async function confirmUnbanUser(mode: ContentModerationUnbanMode) {
  const row = unbanDialogRow.value
  if (!row?.user_id || unbanningUserID.value !== null) return
  const retryingRiskCleanup = mode === 'clear_risk_only'
  if (retryingRiskCleanup ? !unbanWarning.value : !canUnbanRow(row)) return
  unbanningUserID.value = row.user_id
  try {
    const result = await adminAPI.riskControl.unbanUser(row.user_id, { mode })
    const restoredRow: ContentModerationLog = {
      ...row,
      user_status: result.status,
      moderation_ban_active: false,
      unban_block_reason: '',
    }
    logs.value = logs.value.map((item) => {
      if (item.user_id !== row.user_id) return item
      return {
        ...item,
        user_status: result.status,
        moderation_ban_active: false,
        unban_block_reason: '',
      }
    })
    if (result.warning) {
      unbanDialogRow.value = restoredRow
      unbanWarning.value = result.warning
      appStore.showWarning(t('admin.riskControl.unbanPartialSuccess', { warning: result.warning }))
    } else {
      unbanDialogRow.value = null
      unbanWarning.value = ''
      appStore.showSuccess(t(
        retryingRiskCleanup
          ? 'admin.riskControl.riskStateCleanupRetrySuccess'
          : result.risk_state_cleared
          ? 'admin.riskControl.unbanSuccessCleared'
          : 'admin.riskControl.unbanSuccessRestored'
      ))
    }
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(
      err,
      t(retryingRiskCleanup
        ? 'admin.riskControl.riskStateCleanupRetryFailed'
        : 'admin.riskControl.unbanFailed')
    ))
  } finally {
    unbanningUserID.value = null
  }
}

async function deleteFlaggedHash() {
  if (!isFlaggedHashInputValid.value) return
  await deleteFlaggedHashValue(flaggedHashInput.value, true)
}

async function deleteInputDetailFlaggedHash() {
  if (!isInputDetailHashValid.value || hashActionLoading.value) return
  const hash = inputDetailHash.value
  const confirmed = window.confirm(t('admin.riskControl.deleteRecordFlaggedHashConfirm', { hash }))
  if (!confirmed) return
  await deleteFlaggedHashValue(hash, false)
}

async function deleteFlaggedHashValue(value: string, clearManualInput: boolean) {
  const hash = value.trim()
  if (!isValidFlaggedHash(hash) || hashActionLoading.value) return
  hashActionLoading.value = true
  try {
    const result = await adminAPI.riskControl.deleteFlaggedHash(hash)
    if (clearManualInput) flaggedHashInput.value = ''
    await loadStatus(true)
    appStore.showSuccess(result.deleted ? t('admin.riskControl.flaggedHashDeleted') : t('admin.riskControl.flaggedHashNotFound'))
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.flaggedHashDeleteFailed')))
  } finally {
    hashActionLoading.value = false
  }
}

async function clearFlaggedHashes() {
  if (hashActionLoading.value) return
  const confirmed = window.confirm(t('admin.riskControl.clearFlaggedHashesConfirm'))
  if (!confirmed) return
  hashActionLoading.value = true
  try {
    const result = await adminAPI.riskControl.clearFlaggedHashes()
    await loadStatus(true)
    appStore.showSuccess(t('admin.riskControl.flaggedHashesCleared', { count: result.deleted }))
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.flaggedHashesClearFailed')))
  } finally {
    hashActionLoading.value = false
  }
}

function openSettings() {
  activeSettingsTab.value = 'basic'
  settingsOpen.value = true
}

function closeSettings() {
  if (savedConfigSnapshot.value) {
    applyConfig(savedConfigSnapshot.value)
  }
  clearModerationTestInput()
  settingsOpen.value = false
}

function reloadLogsFromFirstPage() {
  pagination.page = 1
  void loadLogs()
}

function onPageChange(page: number) {
  pagination.page = page
  void loadLogs()
}

function onPageSizeChange(pageSize: number) {
  pagination.page = 1
  pagination.page_size = pageSize
  void loadLogs()
}

function toggleClearApiKey() {
  configForm.clear_api_key = !configForm.clear_api_key
  if (configForm.clear_api_key) {
    configForm.api_keys_text = ''
    configForm.api_keys_mode = 'append'
    testedApiKeyStatuses.value = []
    pendingDeleteApiKeyHashes.value = []
  }
}

function setAPIKeysMode(mode: APIKeysWriteMode) {
  configForm.api_keys_mode = mode
  if (mode === 'replace') {
    pendingDeleteApiKeyHashes.value = []
  }
}

function setModelFilterType(type: ContentModerationModelFilterType) {
  configForm.model_filter_type = type
  if (type === 'all') {
    configForm.model_filter_models = []
  }
}

function scopeFilterType(entity: ScopeEntity): ContentModerationScopeFilterType {
  return entity === 'user' ? configForm.user_filter_type : configForm.account_filter_type
}

function scopeFilterIds(entity: ScopeEntity): number[] {
  return entity === 'user' ? configForm.user_filter_ids : configForm.account_filter_ids
}

function setScopeFilterType(entity: ScopeEntity, type: ContentModerationScopeFilterType): void {
  if (entity === 'user') {
    configForm.user_filter_type = type
    if (type === 'all') configForm.user_filter_ids = []
    return
  }
  configForm.account_filter_type = type
  if (type === 'all') configForm.account_filter_ids = []
}

function updateScopeFilterIds(entity: ScopeEntity, ids: number[]): void {
  const normalized = normalizePositiveIDs(ids)
  if (entity === 'user') {
    configForm.user_filter_ids = normalized
  } else {
    configForm.account_filter_ids = normalized
  }
}

function scopeFilterTypeLabel(type: ContentModerationScopeFilterType): string {
  return t(`admin.riskControl.scopeFilter${type === 'all' ? 'All' : type === 'include' ? 'Include' : 'Exclude'}`)
}

function scopeFilterTypeDescription(entity: ScopeEntity, type: ContentModerationScopeFilterType): string {
  const suffix = type === 'all' ? 'AllDesc' : type === 'include' ? 'IncludeDesc' : 'ExcludeDesc'
  return t(`admin.riskControl.${entity}Filter${suffix}`)
}

function scopeFilterSummary(entity: ScopeEntity): string {
  const type = scopeFilterType(entity)
  const count = scopeFilterIds(entity).length
  if (type === 'include') return t('admin.riskControl.scopeFilterIncludeSummary', { count })
  if (type === 'exclude') return t('admin.riskControl.scopeFilterExcludeSummary', { count })
  return t('admin.riskControl.scopeFilterAllSummary')
}

async function testApiKeys(useInputKeys: boolean) {
  const keys = useInputKeys ? parseApiKeys(configForm.api_keys_text) : []
  if (useInputKeys && keys.length === 0) {
    appStore.showError(t('admin.riskControl.apiKeyTestNoInput'))
    return
  }
  if (!validateAIChatPerformanceSettings()) return
  apiKeyTesting.value = true
  try {
    const result = await adminAPI.riskControl.testAPIKeys({
      api_keys: keys,
      audit_provider: configForm.audit_provider,
      base_url: configForm.base_url,
      model: configForm.model,
      timeout_ms: Number(configForm.timeout_ms) || 3000,
      // 与保存语义一致：0 强制直连，>0 指定代理，确保测试与实际审计走同一条链路
      proxy_id: configForm.proxy_id ?? 0,
      prompt: moderationTestPrompt.value,
      images: configForm.audit_provider === 'ai_chat' ? [] : moderationTestImages.value,
      ai_confidence_threshold: Number(configForm.ai_confidence_threshold) || 0.7,
      ai_system_prompt: configForm.ai_system_prompt,
      ai_max_input_chars: Number(configForm.ai_max_input_chars) || 200000,
      ai_synchronous_budget_ms: Number(configForm.ai_synchronous_budget_ms) || 4800,
      ai_fast_stage_budget_ms: Number(configForm.ai_fast_stage_budget_ms) || 3000,
      ai_fast_input_chars: Number(configForm.ai_fast_input_chars) || 3000,
      ai_fallback_input_chars: Number(configForm.ai_fallback_input_chars) || 3000,
      ai_thinking_mode: configForm.ai_thinking_mode,
      ai_reasoning_effort: configForm.ai_reasoning_effort,
      ai_risk_levels_enabled: configForm.ai_risk_levels_enabled,
      ai_observe_threshold: Number(configForm.ai_observe_threshold) || 0.35,
    })
    moderationTestResult.value = result.audit_result ?? null
    if (useInputKeys) {
      testedApiKeyStatuses.value = result.items.map((item) => ({ ...item, configured: false }))
    } else {
      mergeConfiguredAPIKeyStatuses(result.items)
      testedApiKeyStatuses.value = []
      await loadStatus(true)
    }
    if (hasModerationAuditInput.value && !result.audit_result) {
      appStore.showError(result.audit_error?.message || t('admin.riskControl.auditTestNoResult'))
      return
    }
    appStore.showSuccess(t('admin.riskControl.apiKeyTestDone', { count: result.items.length }))
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.apiKeyTestFailed')))
  } finally {
    apiKeyTesting.value = false
  }
}

function mergeConfiguredAPIKeyStatuses(items: ContentModerationAPIKeyStatus[]) {
  if (!hasModerationAuditInput.value || configForm.api_key_statuses.length === 0) {
    configForm.api_key_statuses = items
    return
  }
  const updates = new Map(items.map((item) => [item.key_hash, item]))
  configForm.api_key_statuses = configForm.api_key_statuses.map((item) => updates.get(item.key_hash) ?? item)
}

function toggleDeleteStoredApiKey(row: ContentModerationAPIKeyStatus) {
  if (!row.configured || !row.key_hash) return
  const index = pendingDeleteApiKeyHashes.value.indexOf(row.key_hash)
  if (index >= 0) {
    pendingDeleteApiKeyHashes.value.splice(index, 1)
    return
  }
  pendingDeleteApiKeyHashes.value.push(row.key_hash)
}

function isStoredApiKeyPendingDelete(row: ContentModerationAPIKeyStatus): boolean {
  return row.configured && row.key_hash !== '' && pendingDeleteApiKeyHashes.value.includes(row.key_hash)
}

function prunePendingDeleteAPIKeyHashes() {
  const currentHashes = new Set(savedApiKeyRows.value.map((row) => row.key_hash).filter(Boolean))
  pendingDeleteApiKeyHashes.value = pendingDeleteApiKeyHashes.value.filter((hash) => currentHashes.has(hash))
}

function clearModerationTestInput() {
  moderationTestPrompt.value = ''
  moderationTestImages.value = []
  moderationTestResult.value = null
}

function removeModerationTestImage(index: number) {
  moderationTestImages.value.splice(index, 1)
}

async function handleModerationImageUpload(event: Event) {
  const input = event.target as HTMLInputElement
  await addModerationTestFiles(input.files)
  input.value = ''
}

async function handleModerationImageDrop(event: DragEvent) {
  await addModerationTestFiles(event.dataTransfer?.files ?? null)
}

async function handleModerationImagePaste(event: ClipboardEvent) {
  const files = Array.from(event.clipboardData?.files ?? []).filter((file) => file.type.startsWith('image/'))
  if (files.length === 0) return
  event.preventDefault()
  await addModerationTestFiles(files)
}

async function addModerationTestFiles(files: FileList | File[] | null) {
  if (!files) return
  const items = Array.from(files).filter((file) => file.type.startsWith('image/'))
  for (const file of items) {
    if (moderationTestImages.value.length >= maxModerationTestImages) {
      appStore.showError(t('admin.riskControl.auditTestImageLimit', { count: maxModerationTestImages }))
      return
    }
    if (file.size > maxModerationTestImageSize) {
      appStore.showError(t('admin.riskControl.auditTestImageTooLarge'))
      continue
    }
    try {
      moderationTestImages.value.push(await fileToDataURL(file))
    } catch {
      appStore.showError(t('admin.riskControl.auditTestImageReadFailed'))
    }
  }
}

function fileToDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result || ''))
    reader.onerror = () => reject(reader.error)
    reader.readAsDataURL(file)
  })
}

function toggleGroup(groupID: number) {
  const index = configForm.group_ids.indexOf(groupID)
  if (index >= 0) {
    configForm.group_ids.splice(index, 1)
  } else {
    configForm.group_ids.push(groupID)
  }
}

function isGroupSelected(groupID: number): boolean {
  return configForm.group_ids.includes(groupID)
}

function modeLabel(mode: ModerationMode): string {
  const found = modeOptions.value.find((option) => option.value === mode)
  return found?.label ?? mode
}

function modeDescription(mode: ModerationMode): string {
  const descriptions: Record<ModerationMode, string> = {
    pre_block: t('admin.riskControl.modePreBlockDesc'),
    observe: t('admin.riskControl.modeObserveDesc'),
    off: t('admin.riskControl.modeOffDesc'),
  }
  return descriptions[mode] ?? ''
}

function workerSlotClass(state: WorkerSlotState): string {
  if (state === 'active') {
    return 'border-sky-200 bg-sky-50 text-sky-700 dark:border-sky-900/60 dark:bg-sky-900/20 dark:text-sky-300'
  }
  if (state === 'idle') {
    return 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900/60 dark:bg-emerald-900/20 dark:text-emerald-300'
  }
  return 'border-gray-100 bg-white text-gray-400 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-500'
}

function workerDotClass(state: WorkerSlotState): string {
  if (state === 'active') return 'bg-sky-500'
  if (state === 'idle') return 'bg-emerald-500'
  return 'bg-gray-300 dark:bg-dark-500'
}

function percent(value: number): string {
  if (!Number.isFinite(value)) return '-'
  return `${(value * 100).toFixed(1)}%`
}

function percentWidth(value: number): string {
  if (!Number.isFinite(value)) return '0%'
  return `${Math.min(100, Math.max(0, value * 100)).toFixed(1)}%`
}

function latencyText(value: number | null): string {
  if (value === null || value === undefined) return '-'
  return `${formatNumber(value)} ms`
}

function auditTotalLatency(row: ContentModerationLog): number | null {
  const total = row.audit_details?.total_latency_ms
  return typeof total === 'number' && Number.isFinite(total) && total >= 0
    ? total
    : row.upstream_latency_ms
}

function auditLatencyText(value?: number): string {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0
    ? `${formatNumber(value)} ms`
    : t('common.unknown')
}

function apiKeyRowKey(row: ContentModerationAPIKeyStatus, index: number): string {
  return `${row.configured ? 'saved' : 'test'}-${row.key_hash || index}`
}

function apiKeyStatusLabel(statusValue: ContentModerationAPIKeyStatus['status']): string {
  const labels: Record<ContentModerationAPIKeyStatus['status'], string> = {
    ok: t('admin.riskControl.apiKeyStatusOk'),
    error: t('admin.riskControl.apiKeyStatusError'),
    frozen: t('admin.riskControl.apiKeyStatusFrozen'),
    unknown: t('admin.riskControl.apiKeyStatusUnknown'),
  }
  return labels[statusValue] ?? labels.unknown
}

function apiKeyStatusBadgeClass(statusValue: ContentModerationAPIKeyStatus['status']): string {
  const classes: Record<ContentModerationAPIKeyStatus['status'], string> = {
    ok: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300',
    error: 'bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300',
    frozen: 'bg-red-50 text-red-700 dark:bg-red-900/20 dark:text-red-300',
    unknown: 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300',
  }
  return classes[statusValue] ?? classes.unknown
}

function apiKeyStatusDotClass(statusValue: ContentModerationAPIKeyStatus['status']): string {
  const classes: Record<ContentModerationAPIKeyStatus['status'], string> = {
    ok: 'bg-emerald-500',
    error: 'bg-amber-500',
    frozen: 'bg-red-500',
    unknown: 'bg-gray-400',
  }
  return classes[statusValue] ?? classes.unknown
}

function apiKeyStatusMeta(row: ContentModerationAPIKeyStatus): string {
  const parts: string[] = []
  parts.push(t('admin.riskControl.apiKeyFailureCount', { count: row.failure_count || 0 }))
  if (row.last_latency_ms > 0) {
    parts.push(t('admin.riskControl.apiKeyLatency', { ms: row.last_latency_ms }))
  }
  if (row.last_http_status > 0) {
    parts.push(t('admin.riskControl.apiKeyHTTPStatus', { status: row.last_http_status }))
  }
  if (row.frozen_until) {
    parts.push(t('admin.riskControl.apiKeyFrozenUntil', { time: formatDateTime(row.frozen_until) }))
  } else if (row.last_checked_at) {
    parts.push(t('admin.riskControl.apiKeyLastChecked', { time: formatDateTime(row.last_checked_at) }))
  } else {
    parts.push(t('admin.riskControl.apiKeyNotTested'))
  }
  return parts.join(' / ')
}

function parseApiKeys(value: string): string[] {
  return value
    .split(/\r?\n/)
    .map((item) => item.trim())
    .filter((item, index, arr) => item && arr.indexOf(item) === index)
}

function normalizeKeywordBlockingMode(value: unknown): KeywordBlockingMode {
  if (value === 'keyword_only' || value === 'api_only' || value === 'keyword_and_api') {
    return value
  }
  return 'keyword_and_api'
}

function normalizeModelFilter(value: unknown): ContentModerationModelFilter {
  if (!value || typeof value !== 'object') {
    return { type: 'all', models: [] }
  }
  const raw = value as Partial<ContentModerationModelFilter>
  const type = normalizeModelFilterType(raw.type)
  const models = type === 'all' ? [] : normalizeModelNames(raw.models)
  return { type, models }
}

function normalizeModelFilterType(value: unknown): ContentModerationModelFilterType {
  if (value === 'include' || value === 'exclude' || value === 'all') {
    return value
  }
  return 'all'
}

function normalizeModelNames(models: unknown): string[] {
  if (!Array.isArray(models)) return []
  const seen = new Set<string>()
  const out: string[] = []
  for (const item of models) {
    const model = String(item ?? '').trim()
    if (!model) continue
    const key = model.toLowerCase()
    if (seen.has(key)) continue
    seen.add(key)
    out.push(model)
  }
  return out
}

function buildModelFilterPayload(): ContentModerationModelFilter {
  const type = normalizeModelFilterType(configForm.model_filter_type)
  if (type === 'all') {
    return { type: 'all', models: [] }
  }
  return {
    type,
    models: normalizeModelNames(configForm.model_filter_models),
  }
}

function normalizeScopeFilterType(value: unknown): ContentModerationScopeFilterType {
  if (value === 'include' || value === 'exclude' || value === 'all') return value
  return 'all'
}

function normalizePositiveIDs(value: unknown): number[] {
  if (!Array.isArray(value)) return []
  return Array.from(new Set(value.map(Number).filter((id) => Number.isInteger(id) && id > 0)))
}

function normalizeUserFilter(value: unknown): ContentModerationUserFilter {
  if (!value || typeof value !== 'object') return { type: 'all', user_ids: [] }
  const raw = value as Partial<ContentModerationUserFilter>
  const type = normalizeScopeFilterType(raw.type)
  return { type, user_ids: type === 'all' ? [] : normalizePositiveIDs(raw.user_ids) }
}

function normalizeAccountFilter(value: unknown): ContentModerationAccountFilter {
  if (!value || typeof value !== 'object') return { type: 'all', account_ids: [] }
  const raw = value as Partial<ContentModerationAccountFilter>
  const type = normalizeScopeFilterType(raw.type)
  return { type, account_ids: type === 'all' ? [] : normalizePositiveIDs(raw.account_ids) }
}

function buildUserFilterPayload(): ContentModerationUserFilter {
  const type = normalizeScopeFilterType(configForm.user_filter_type)
  return { type, user_ids: type === 'all' ? [] : normalizePositiveIDs(configForm.user_filter_ids) }
}

function buildAccountFilterPayload(): ContentModerationAccountFilter {
  const type = normalizeScopeFilterType(configForm.account_filter_type)
  return { type, account_ids: type === 'all' ? [] : normalizePositiveIDs(configForm.account_filter_ids) }
}

function riskThresholdsFromConfig(thresholds: Record<string, number> | null | undefined): Record<string, number> {
  const out: Record<string, number> = { ...riskThresholdDefaults }
  for (const category of riskThresholdCategories) {
    const value = thresholds?.[category]
    if (Number.isFinite(value)) {
      out[category] = clampPercent(Number(value) * 100)
    }
  }
  return out
}

function buildRiskThresholdPayload(): Record<string, number> {
  const payload: Record<string, number> = {}
  for (const category of riskThresholdCategories) {
    payload[category] = Number((clampPercent(configForm.thresholds[category]) / 100).toFixed(4))
  }
  return payload
}

function resetRiskThresholds() {
  configForm.thresholds = { ...riskThresholdDefaults }
}

function clampPercent(value: unknown): number {
  const numeric = Number(value)
  if (!Number.isFinite(numeric)) {
    return 0
  }
  return Math.min(100, Math.max(0, numeric))
}

function formatThresholdPercent(value: number): string {
  return `${clampPercent(value).toFixed(1)}%`
}

function parseBlockedKeywords(value: string): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const line of value.split(/\r?\n/)) {
    const kw = line.trim()
    if (!kw) continue
    const key = kw.toLowerCase()
    if (seen.has(key)) continue
    seen.add(key)
    out.push(kw)
  }
  return out
}

function violationCountText(row: ContentModerationLog): string {
  if (!row.flagged) return '-'
  if (row.violation_count === 0) return t('admin.riskControl.violationNotCounted')
  return t('admin.riskControl.violationCount', { count: row.violation_count || 1 })
}

function normalizeDateTimeLocal(value: string): string | undefined {
  if (!value) return undefined
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return undefined
  return date.toISOString()
}

function formatDateTime(value: string): string {
  return formatDateTimeValue(value) || '-'
}

function formatNumber(value: number): string {
  return new Intl.NumberFormat().format(value)
}

onMounted(() => {
  void loadAll()
  statusTimer = window.setInterval(() => {
    void loadStatus(true)
  }, 15000)
})

onUnmounted(() => {
  if (statusTimer !== null) {
    window.clearInterval(statusTimer)
    statusTimer = null
  }
})
</script>
