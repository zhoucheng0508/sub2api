<template>
  <div ref="containerRef" class="relative" :data-test="`${entity}-scope-selector`">
    <div v-if="selectedIds.length > 0" class="mb-2 flex flex-wrap gap-2">
      <span
        v-for="id in selectedIds"
        :key="id"
        class="inline-flex max-w-full items-center gap-1.5 rounded-md bg-gray-100 px-2.5 py-1.5 text-xs text-gray-700 dark:bg-dark-600 dark:text-gray-200"
      >
        <span class="max-w-64 truncate font-medium" :title="selectedLabel(id)">
          {{ selectedLabel(id) }}
        </span>
        <span class="shrink-0 text-gray-400">#{{ id }}</span>
        <button
          type="button"
          class="shrink-0 rounded text-gray-400 hover:text-red-600 dark:hover:text-red-400"
          :aria-label="removeLabel"
          :title="removeLabel"
          @click="removeEntity(id)"
        >
          <Icon name="x" size="xs" :stroke-width="2" />
        </button>
      </span>
    </div>

    <div class="relative">
      <Icon
        name="search"
        size="sm"
        class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400"
      />
      <input
        v-model="searchQuery"
        type="search"
        autocomplete="off"
        class="input input-sm w-full pl-9"
        :data-test="`${entity}-scope-search`"
        :placeholder="searchPlaceholder"
        @input="debounceSearch"
        @focus="showDropdown = true"
      />
    </div>

    <div
      v-if="showDropdown && searchQuery.trim()"
      class="absolute z-50 mt-1 max-h-64 w-full overflow-auto rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-600 dark:bg-dark-700"
    >
      <div v-if="searchLoading" class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400">
        {{ t('common.loading') }}
      </div>
      <div v-else-if="availableResults.length === 0" class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400">
        {{ emptyLabel }}
      </div>
      <template v-else>
        <button
          v-for="option in availableResults"
          :key="option.id"
          type="button"
          class="flex w-full items-center justify-between gap-3 px-4 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-dark-600"
          :data-test="`${entity}-scope-option-${option.id}`"
          @click="selectEntity(option)"
        >
          <span class="min-w-0">
            <span class="block truncate font-medium text-gray-900 dark:text-white">{{ option.label }}</span>
            <span v-if="option.meta" class="mt-0.5 block truncate text-xs text-gray-500 dark:text-gray-400">
              {{ option.meta }}
            </span>
          </span>
          <span class="shrink-0 text-xs text-gray-400">#{{ option.id }}</span>
        </button>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import Icon from '@/components/icons/Icon.vue'
import type { Account, AdminUser } from '@/types'

type ScopeEntity = 'user' | 'account'
type EntityOption = {
  id: number
  label: string
  meta: string
  direct: boolean
}

const props = defineProps<{
  modelValue: number[]
  entity: ScopeEntity
}>()

const emit = defineEmits<{
  'update:modelValue': [value: number[]]
}>()

const { t } = useI18n()
const containerRef = ref<HTMLElement | null>(null)
const searchQuery = ref('')
const searchResults = ref<EntityOption[]>([])
const selectedEntities = ref<Record<number, EntityOption>>({})
const searchLoading = ref(false)
const showDropdown = ref(false)
let searchTimer: ReturnType<typeof setTimeout> | null = null
let searchSequence = 0

const selectedIds = computed(() => normalizeIds(props.modelValue))
const availableResults = computed(() => {
  const selected = new Set(selectedIds.value)
  return searchResults.value.filter((option) => !selected.has(option.id))
})
const searchPlaceholder = computed(() => t(`admin.riskControl.${props.entity}FilterSearchPlaceholder`))
const emptyLabel = computed(() => t(`admin.riskControl.${props.entity}FilterSearchEmpty`))
const removeLabel = computed(() => t(`admin.riskControl.${props.entity}FilterRemove`))

function normalizeIds(value: unknown): number[] {
  if (!Array.isArray(value)) return []
  return Array.from(new Set(value.map(Number).filter((id) => Number.isInteger(id) && id > 0)))
}

function normalizePositiveId(value: unknown): number | null {
  const id = Number(value)
  return Number.isInteger(id) && id > 0 ? id : null
}

function toUserOption(user: AdminUser): EntityOption {
  return {
    id: user.id,
    label: user.email,
    meta: user.username || user.role || '',
    direct: true,
  }
}

function toAccountOption(account: Account): EntityOption {
  const parentId = normalizePositiveId(account.parent_account_id)
  const id = parentId ?? account.id
  return {
    id,
    label: parentId
      ? t('admin.riskControl.accountFilterShadowLabel', { name: account.name, id })
      : account.name,
    meta: [account.platform, account.type].filter(Boolean).join(' / '),
    direct: parentId === null,
  }
}

function deduplicateOptions(options: EntityOption[]): EntityOption[] {
  const byId = new Map<number, EntityOption>()
  for (const option of options) {
    const existing = byId.get(option.id)
    if (!existing || (!existing.direct && option.direct)) {
      byId.set(option.id, option)
    }
  }
  return Array.from(byId.values())
}

function selectedLabel(id: number): string {
  return selectedEntities.value[id]?.label || t(`admin.riskControl.${props.entity}FilterIdFallback`, { id })
}

function clearPendingSearch(): void {
  if (searchTimer) {
    clearTimeout(searchTimer)
    searchTimer = null
  }
  searchSequence += 1
}

function debounceSearch(): void {
  clearPendingSearch()
  const query = searchQuery.value.trim()
  showDropdown.value = true
  if (!query) {
    searchResults.value = []
    searchLoading.value = false
    return
  }

  const sequence = searchSequence
  searchTimer = setTimeout(async () => {
    searchLoading.value = true
    try {
      const result = props.entity === 'user'
        ? await adminAPI.users.list(1, 20, { search: query })
        : await adminAPI.accounts.list(1, 20, { search: query })
      if (sequence === searchSequence) {
        const options = props.entity === 'user'
          ? result.items.map((item) => toUserOption(item as AdminUser))
          : result.items.map((item) => toAccountOption(item as Account))
        searchResults.value = deduplicateOptions(options)
      }
    } catch {
      if (sequence === searchSequence) {
        searchResults.value = []
      }
    } finally {
      if (sequence === searchSequence) {
        searchLoading.value = false
      }
    }
  }, 300)
}

function selectEntity(option: EntityOption): void {
  selectedEntities.value = { ...selectedEntities.value, [option.id]: option }
  emit('update:modelValue', normalizeIds([...selectedIds.value, option.id]))
  clearPendingSearch()
  searchQuery.value = ''
  searchResults.value = []
  searchLoading.value = false
  showDropdown.value = false
}

function removeEntity(id: number): void {
  emit('update:modelValue', selectedIds.value.filter((selectedId) => selectedId !== id))
}

async function hydrateSelectedEntities(ids: number[]): Promise<void> {
  const missing = ids.filter((id) => !selectedEntities.value[id])
  if (missing.length === 0) return

  const resolved = await Promise.all(missing.map(async (requestedId): Promise<{ requestedId: number; option: EntityOption } | null> => {
    try {
      if (props.entity === 'user') {
        return { requestedId, option: toUserOption(await adminAPI.users.getById(requestedId, true)) }
      }
      return { requestedId, option: toAccountOption(await adminAPI.accounts.getById(requestedId)) }
    } catch {
      return null
    }
  }))

  const next = { ...selectedEntities.value }
  const activeIds = new Set(selectedIds.value)
  const canonicalByRequestedId = new Map<number, number>()
  for (const item of resolved) {
    if (!item || !activeIds.has(item.requestedId)) continue
    canonicalByRequestedId.set(item.requestedId, item.option.id)
    const existing = next[item.option.id]
    if (!existing || (!existing.direct && item.option.direct)) {
      next[item.option.id] = item.option
    }
  }
  selectedEntities.value = next

  const canonicalIds = normalizeIds(selectedIds.value.map((id) => canonicalByRequestedId.get(id) ?? id))
  if (canonicalIds.length !== selectedIds.value.length || canonicalIds.some((id, index) => id !== selectedIds.value[index])) {
    emit('update:modelValue', canonicalIds)
  }
}

function handleDocumentClick(event: MouseEvent): void {
  const target = event.target as Node | null
  if (target && !containerRef.value?.contains(target)) {
    showDropdown.value = false
  }
}

watch(
  () => [props.entity, ...selectedIds.value] as const,
  () => {
    void hydrateSelectedEntities(selectedIds.value)
  },
  { immediate: true },
)

onMounted(() => document.addEventListener('click', handleDocumentClick))
onUnmounted(() => {
  clearPendingSearch()
  document.removeEventListener('click', handleDocumentClick)
})
</script>
