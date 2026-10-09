import { ref, watch } from 'vue'

type Page = 'usage' | 'accounts'

export function normalizeAdminGroup(value: unknown, page: Page): string {
  if (page === 'accounts' && value === 'ungrouped') return 'ungrouped'
  if (typeof value !== 'string' && typeof value !== 'number') return ''
  const id = Number(value)
  return Number.isSafeInteger(id) && id > 0 ? String(id) : ''
}

/** Remember only the group, independently for each administrator and page. */
export function useRememberedAdminGroup(page: Page, userId: () => number | null | undefined) {
  const group = ref('')
  const storageKey = () => {
    const id = userId()
    return id && Number.isSafeInteger(id) && id > 0
      ? `sub2api:admin-group-filter:${page}:user:${id}`
      : null
  }

  watch(userId, () => {
    const key = storageKey()
    try {
      group.value = key ? normalizeAdminGroup(localStorage.getItem(key), page) : ''
    } catch {
      group.value = ''
    }
  }, { immediate: true, flush: 'sync' })

  watch(group, value => {
    const key = storageKey()
    if (!key) return
    try {
      const normalized = normalizeAdminGroup(value, page)
      if (normalized) localStorage.setItem(key, normalized)
      else localStorage.removeItem(key)
    } catch {
      // Browsers that disable storage can still use both pages normally.
    }
  }, { flush: 'sync' })

  const validate = (groups: ReadonlyArray<{ id: number }>): boolean => {
    if (!group.value || group.value === 'ungrouped') return false
    if (groups.some(item => String(item.id) === group.value)) return false
    group.value = ''
    return true
  }

  return { group, validate }
}
