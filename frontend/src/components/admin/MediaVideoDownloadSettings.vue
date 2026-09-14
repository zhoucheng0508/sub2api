<template>
  <section class="card" aria-labelledby="video-download-settings-title">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 id="video-download-settings-title" class="text-lg font-semibold text-gray-900 dark:text-white">{{ copy.title }}</h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ copy.description }}</p>
    </div>
    <div class="space-y-5 p-6">
      <p v-if="loading" role="status" class="text-sm text-gray-500">{{ copy.loading }}</p>
      <div v-else-if="loaded" class="grid grid-cols-1 gap-4 md:grid-cols-2">
        <div v-for="field in fields" :key="field.key">
          <label :for="`video-download-${field.key}`" class="input-label">{{ field.label }}</label>
          <input :id="`video-download-${field.key}`" v-model.number="form[field.key]" class="input" type="number"
            :min="field.min" :max="field.max" step="1" :disabled="saving" />
        </div>
      </div>
      <p v-if="loaded" class="text-sm text-gray-500 dark:text-gray-400">
        {{ copy.bandwidth }} {{ globalMbps }} Mbps · {{ copy.userBandwidth }} {{ userMbps }} Mbps
      </p>
      <p class="text-sm text-gray-500 dark:text-gray-400">{{ copy.hint }}</p>
      <p class="text-sm text-gray-500 dark:text-gray-400">{{ copy.deletion }}</p>
      <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
      <div class="flex flex-wrap gap-3">
        <button v-if="loaded" type="button" class="btn btn-primary" :disabled="saving || loading" @click="save">
          {{ saving ? copy.saving : copy.save }}
        </button>
        <button type="button" class="btn btn-secondary" :disabled="saving || loading" @click="load">{{ copy.reload }}</button>
        <button v-if="loaded" type="button" class="btn btn-secondary" :disabled="saving || loading" @click="resetDefaults">{{ copy.defaults }}</button>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { isAxiosError } from 'axios'
import { apiClient } from '@/api/client'
import { useAppStore } from '@/stores/app'

interface DownloadSettings {
  global_concurrency: number
  user_concurrency: number
  global_kib_per_second: number
  user_kib_per_second: number
  user_requests_per_minute: number
  write_timeout_seconds: number
  max_duration_seconds: number
}

const defaults: DownloadSettings = {
  global_concurrency: 4, user_concurrency: 2,
  global_kib_per_second: 1536, user_kib_per_second: 512,
  user_requests_per_minute: 60, write_timeout_seconds: 30, max_duration_seconds: 1800
}
const form = reactive<DownloadSettings>({ ...defaults })
const { locale } = useI18n()
const app = useAppStore()
const loading = ref(false)
const loaded = ref(false)
const saving = ref(false)
const error = ref('')
const chinese = computed(() => locale.value.startsWith('zh'))
const copy = computed(() => chinese.value ? {
  title: '视频下载保护', description: '手动调整视频下载资源上限，与聊天和图片生成并发独立。',
  loading: '正在读取设置…', save: '保存视频下载设置', saving: '正在保存…', reload: '重新读取',
  defaults: '填入 30 Mbps 推荐值（需保存）', bandwidth: '全站视频最高约', userBandwidth: '每用户最高约',
  hint: '限速单位为 KiB/s，1024 KiB/s = 1 MiB/s。每用户的所有 Key、完整下载和分段下载共享上限。并发、频率和限速约 2 秒内更新，超时设置对新下载生效。降低并发不会中断已有下载。',
  deletion: '有未完成视频或未结清款项时不能删除用户；账号还需等待视频下载有效期结束。可先停用访问或调度。',
  invalid: '请填写范围内的整数，每用户的并发和限速不能超过全站上限。', saved: '视频下载设置已保存', failed: '视频下载设置操作失败'
} : {
  title: 'Video download protection', description: 'Manually limit video downloads independently from chat and image generation concurrency.',
  loading: 'Loading settings…', save: 'Save video download settings', saving: 'Saving…', reload: 'Reload',
  defaults: 'Fill recommended values for 30 Mbps (save required)', bandwidth: 'Total video bandwidth up to', userBandwidth: 'Per user up to',
  hint: 'Rates use KiB/s; 1024 KiB/s = 1 MiB/s. All keys, full downloads and ranges share each user’s limits. Concurrency, request rate and bandwidth update within about 2 seconds; timeouts apply to new downloads. Lower concurrency does not interrupt active downloads.',
  deletion: 'Users cannot be deleted with unfinished videos or unsettled funds. Accounts must also retain videos until download access expires. Disable access or scheduling first if needed.',
  invalid: 'Enter integers within the allowed ranges. User concurrency and rate cannot exceed the total limits.', saved: 'Video download settings saved', failed: 'Unable to update video download settings'
})
const fields = computed<Array<{ key: keyof DownloadSettings; label: string; min: number; max: number }>>(() => [
  { key: 'global_concurrency', label: chinese.value ? '全站同时下载数（1–128）' : 'Total concurrent downloads (1–128)', min: 1, max: 128 },
  { key: 'user_concurrency', label: chinese.value ? '每用户同时下载数（1–16）' : 'Concurrent downloads per user (1–16)', min: 1, max: 16 },
  { key: 'global_kib_per_second', label: chinese.value ? '全站总限速（KiB/s，至少 64）' : 'Total rate (KiB/s, minimum 64)', min: 64, max: 1048576 },
  { key: 'user_kib_per_second', label: chinese.value ? '每用户总限速（KiB/s，至少 64）' : 'Rate per user (KiB/s, minimum 64)', min: 64, max: 1048576 },
  { key: 'user_requests_per_minute', label: chinese.value ? '每用户请求数/分钟（1–600）' : 'Requests per user per minute (1–600)', min: 1, max: 600 },
  { key: 'write_timeout_seconds', label: chinese.value ? '单次写入超时（秒，5–120）' : 'Write timeout (seconds, 5–120)', min: 5, max: 120 },
  { key: 'max_duration_seconds', label: chinese.value ? '下载总时限（秒，60–7200）' : 'Download duration limit (seconds, 60–7200)', min: 60, max: 7200 }
])
const globalMbps = computed(() => (Number(form.global_kib_per_second || 0) * 1024 * 8 / 1e6).toFixed(2))
const userMbps = computed(() => (Number(form.user_kib_per_second || 0) * 1024 * 8 / 1e6).toFixed(2))
const endpoint = '/admin/settings/media-video-download'
function message(err: unknown): string {
  if (isAxiosError(err) && typeof err.response?.data?.message === 'string') return err.response.data.message
  if (err && typeof err === 'object' && 'message' in err && typeof err.message === 'string') return err.message
  return copy.value.failed
}
async function load() {
  loading.value = true
  error.value = ''
  try { const { data } = await apiClient.get<DownloadSettings>(endpoint); Object.assign(form, data); loaded.value = true }
  catch (err) { error.value = message(err) }
  finally { loading.value = false }
}
function resetDefaults() { Object.assign(form, defaults); error.value = '' }
async function save() {
  if (fields.value.some(f => !Number.isInteger(form[f.key]) || form[f.key] < f.min || form[f.key] > f.max)
      || form.user_concurrency > form.global_concurrency || form.user_kib_per_second > form.global_kib_per_second) {
    error.value = copy.value.invalid
    return
  }
  saving.value = true
  error.value = ''
  try { const { data } = await apiClient.put<DownloadSettings>(endpoint, { ...form }); Object.assign(form, data); app.showSuccess(copy.value.saved) }
  catch (err) { error.value = message(err) }
  finally { saving.value = false }
}
onMounted(load)
</script>
