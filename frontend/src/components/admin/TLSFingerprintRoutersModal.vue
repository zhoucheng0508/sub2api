<template>
  <BaseDialog :show="show" :title="t('admin.tlsFingerprintRouters.title')" width="wide" @close="$emit('close')">
    <div class="space-y-4">
      <div class="flex items-center justify-between gap-4">
        <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.tlsFingerprintRouters.description') }}</p>
        <button class="btn btn-primary btn-sm shrink-0" type="button" @click="openCreate">
          <Icon name="plus" size="sm" class="mr-1" />{{ t('admin.tlsFingerprintRouters.create') }}
        </button>
      </div>

      <div v-if="loading" class="flex justify-center py-10"><Icon name="refresh" class="animate-spin text-gray-400" /></div>
      <div v-else-if="routers.length === 0" class="py-10 text-center text-sm text-gray-500">
        {{ t('admin.tlsFingerprintRouters.empty') }}
      </div>
      <div v-else class="max-h-96 overflow-auto rounded-lg border border-gray-200 dark:border-dark-600">
        <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
          <thead class="sticky top-0 bg-gray-50 dark:bg-dark-700">
            <tr>
              <th class="px-3 py-2 text-left text-xs text-gray-500">{{ t('admin.tlsFingerprintRouters.name') }}</th>
              <th class="px-3 py-2 text-left text-xs text-gray-500">{{ t('admin.tlsFingerprintRouters.rules') }}</th>
              <th class="px-3 py-2 text-left text-xs text-gray-500">{{ t('admin.tlsFingerprintRouters.status') }}</th>
              <th class="px-3 py-2 text-right text-xs text-gray-500">{{ t('common.actions') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-200 bg-white dark:divide-dark-700 dark:bg-dark-800">
            <tr v-for="router in routers" :key="router.id">
              <td class="px-3 py-2">
                <div class="text-sm font-medium text-gray-900 dark:text-white">{{ router.name }}</div>
                <div class="max-w-xs truncate text-xs text-gray-500">{{ router.description || '-' }}</div>
              </td>
              <td class="px-3 py-2 text-sm text-gray-600 dark:text-gray-300">{{ router.rules.length }}</td>
              <td class="px-3 py-2"><span :class="router.enabled ? 'badge badge-success' : 'badge'">{{ router.enabled ? t('common.enabled') : t('common.disabled') }}</span></td>
              <td class="px-3 py-2">
                <div class="flex justify-end gap-1">
                  <button class="p-1 text-gray-500 hover:text-primary-600" type="button" :title="t('common.edit')" @click="openEdit(router)"><Icon name="edit" size="sm" /></button>
                  <button class="p-1 text-gray-500 hover:text-red-600" type="button" :title="t('common.delete')" @click="requestDelete(router)"><Icon name="trash" size="sm" /></button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
    <template #footer><button class="btn btn-secondary" type="button" @click="$emit('close')">{{ t('common.close') }}</button></template>

    <BaseDialog :show="showForm" :title="editingId ? t('admin.tlsFingerprintRouters.edit') : t('admin.tlsFingerprintRouters.create')" width="wide" :z-index="60" @close="showForm = false">
      <form class="space-y-4" @submit.prevent="save">
        <div class="grid gap-4 md:grid-cols-2">
          <div><label class="input-label">{{ t('admin.tlsFingerprintRouters.name') }}</label><input v-model.trim="form.name" class="input" required /></div>
          <div><label class="input-label">{{ t('admin.tlsFingerprintRouters.descriptionLabel') }}</label><input v-model.trim="form.description" class="input" /></div>
        </div>
        <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
          <input v-model="form.enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600" />{{ t('admin.tlsFingerprintRouters.enabled') }}
        </label>

        <div class="flex items-center justify-between border-t border-gray-200 pt-4 dark:border-dark-600">
          <div><div class="input-label mb-0">{{ t('admin.tlsFingerprintRouters.rules') }}</div><p class="input-hint">{{ t('admin.tlsFingerprintRouters.firstMatch') }}</p></div>
          <button class="btn btn-secondary btn-sm" type="button" @click="addRule"><Icon name="plus" size="sm" class="mr-1" />{{ t('admin.tlsFingerprintRouters.addRule') }}</button>
        </div>

        <div v-for="(rule, index) in form.rules" :key="index" class="space-y-3 rounded-lg border border-gray-200 p-3 dark:border-dark-600">
          <div class="flex items-center justify-between">
            <strong class="text-sm text-gray-900 dark:text-white">{{ t('admin.tlsFingerprintRouters.ruleNumber', { index: index + 1 }) }}</strong>
            <button class="p-1 text-gray-500 hover:text-red-600" type="button" :title="t('common.delete')" @click="form.rules.splice(index, 1)"><Icon name="trash" size="sm" /></button>
          </div>
          <div class="grid gap-3 md:grid-cols-2">
            <div><label class="input-label">{{ t('admin.tlsFingerprintRouters.ruleName') }}</label><input v-model.trim="rule.name" class="input" /></div>
            <div><label class="input-label">{{ t('admin.tlsFingerprintRouters.profile') }}</label><select v-model.number="rule.tls_fingerprint_profile_id" class="input"><option :value="0">{{ t('admin.tlsFingerprintRouters.builtin') }}</option><option :value="-1">{{ t('admin.tlsFingerprintRouters.random') }}</option><option v-for="profile in profiles" :key="profile.id" :value="profile.id">{{ profile.name }}</option></select></div>
            <div><label class="input-label">{{ t('admin.tlsFingerprintRouters.matchType') }}</label><select v-model="rule.match_type" class="input"><option value="contains">contains</option><option value="prefix">prefix</option><option value="exact">exact</option><option value="regex">regex</option></select></div>
            <div><label class="input-label">{{ t('admin.tlsFingerprintRouters.pattern') }}</label><input v-model.trim="rule.pattern" class="input" required placeholder="codex_cli_rs/" /></div>
            <div><label class="input-label">User-Agent</label><input v-model.trim="rule.upstream_user_agent" class="input" :placeholder="fixedUserAgent" /></div>
            <div><label class="input-label">Originator</label><input v-model.trim="rule.upstream_originator" class="input" placeholder="codex_cli_rs" /></div>
          </div>
          <div class="flex flex-wrap gap-5 text-sm text-gray-700 dark:text-gray-300">
            <label class="flex items-center gap-2"><input v-model="rule.enabled" type="checkbox" class="h-4 w-4 rounded" />{{ t('common.enabled') }}</label>
            <label class="flex items-center gap-2"><input v-model="rule.case_sensitive" type="checkbox" class="h-4 w-4 rounded" />{{ t('admin.tlsFingerprintRouters.caseSensitive') }}</label>
          </div>
        </div>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3"><button class="btn btn-secondary" type="button" @click="showForm = false">{{ t('common.cancel') }}</button><button class="btn btn-primary" type="button" :disabled="saving" @click="save">{{ t('common.save') }}</button></div>
      </template>
    </BaseDialog>

    <ConfirmDialog :show="showDelete" :title="t('admin.tlsFingerprintRouters.deleteTitle')" :message="t('admin.tlsFingerprintRouters.deleteMessage', { name: deleting?.name })" danger @confirm="confirmDelete" @cancel="showDelete = false" />
  </BaseDialog>
</template>

<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { TLSFingerprintRouter, TLSFingerprintRouterRule } from '@/api/admin/tlsFingerprintRouter'
import { useAppStore } from '@/stores/app'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{ show: boolean }>()
defineEmits<{ close: [] }>()
const { t } = useI18n()
const appStore = useAppStore()
const fixedUserAgent = 'codex_cli_rs/0.144.1 (Ubuntu 22.4.0; x86_64) xterm-256color'
const routers = ref<TLSFingerprintRouter[]>([])
const profiles = ref<{ id: number; name: string }[]>([])
const loading = ref(false)
const saving = ref(false)
const showForm = ref(false)
const showDelete = ref(false)
const editingId = ref<number | null>(null)
const deleting = ref<TLSFingerprintRouter | null>(null)
const form = reactive({ name: '', description: '', enabled: true, rules: [] as TLSFingerprintRouterRule[] })

const newRule = (): TLSFingerprintRouterRule => ({ name: '', enabled: true, match_type: 'contains', pattern: '', case_sensitive: false, tls_fingerprint_profile_id: 0, upstream_user_agent: fixedUserAgent, upstream_originator: 'codex_cli_rs' })
const load = async () => {
  loading.value = true
  try {
    const [routerRows, profileRows] = await Promise.all([adminAPI.tlsFingerprintRouters.list(), adminAPI.tlsFingerprintProfiles.list()])
    routers.value = routerRows
    profiles.value = profileRows.map(p => ({ id: p.id, name: p.name }))
  } catch { appStore.showError(t('admin.tlsFingerprintRouters.loadFailed')) } finally { loading.value = false }
}
watch(() => props.show, visible => { if (visible) void load() })

const reset = () => { editingId.value = null; form.name = ''; form.description = ''; form.enabled = true; form.rules = [newRule()] }
const openCreate = () => { reset(); showForm.value = true }
const openEdit = (router: TLSFingerprintRouter) => { editingId.value = router.id; form.name = router.name; form.description = router.description || ''; form.enabled = router.enabled; form.rules = router.rules.map(rule => ({ ...rule })); showForm.value = true }
const addRule = () => form.rules.push(newRule())
const requestDelete = (router: TLSFingerprintRouter) => { deleting.value = router; showDelete.value = true }

const save = async () => {
  if (!form.name || form.rules.some(rule => !rule.pattern)) { appStore.showError(t('admin.tlsFingerprintRouters.required')); return }
  if (form.rules.some(rule => Boolean(rule.upstream_user_agent) !== Boolean(rule.upstream_originator))) { appStore.showError(t('admin.tlsFingerprintRouters.identityPair')); return }
  saving.value = true
  const payload = { name: form.name, description: form.description || null, enabled: form.enabled, rules: form.rules }
  try {
    if (editingId.value) await adminAPI.tlsFingerprintRouters.update(editingId.value, payload)
    else await adminAPI.tlsFingerprintRouters.create(payload)
    appStore.showSuccess(t('admin.tlsFingerprintRouters.saved'))
    showForm.value = false
    await load()
  } catch { appStore.showError(t('admin.tlsFingerprintRouters.saveFailed')) } finally { saving.value = false }
}
const confirmDelete = async () => {
  if (!deleting.value) return
  try { await adminAPI.tlsFingerprintRouters.delete(deleting.value.id); showDelete.value = false; await load() }
  catch { appStore.showError(t('admin.tlsFingerprintRouters.deleteFailed')) }
}

</script>
