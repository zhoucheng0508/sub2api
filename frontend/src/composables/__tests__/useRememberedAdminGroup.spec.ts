import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, ref } from 'vue'
import { useRememberedAdminGroup } from '../useRememberedAdminGroup'

const key = (page: string, user: number) => `sub2api:admin-group-filter:${page}:user:${user}`
const scopes: ReturnType<typeof effectScope>[] = []
function remembered(page: 'usage' | 'accounts', user = ref<number | null>(1)) {
  const scope = effectScope()
  scopes.push(scope)
  return scope.run(() => useRememberedAdminGroup(page, () => user.value))!
}

beforeEach(() => localStorage.clear())
afterEach(() => {
  scopes.splice(0).forEach(scope => scope.stop())
  vi.restoreAllMocks()
})

describe('remembered admin group', () => {
  it('restores selections on a new visit and keeps the two pages independent', () => {
    remembered('usage').group.value = '7'
    remembered('accounts').group.value = '8'
    expect(remembered('usage').group.value).toBe('7')
    expect(remembered('accounts').group.value).toBe('8')
  })

  it('switches preferences with the administrator without overwriting either user', () => {
    localStorage.setItem(key('usage', 2), '9')
    const user = ref<number | null>(1)
    const state = remembered('usage', user)
    state.group.value = '7'
    user.value = 2
    expect(state.group.value).toBe('9')
    user.value = null
    expect(state.group.value).toBe('')
    user.value = 1
    expect(state.group.value).toBe('7')
    expect(localStorage.getItem(key('usage', 2))).toBe('9')
  })

  it('clears the saved choice when reset and falls back when the group is deleted', () => {
    const state = remembered('usage')
    state.group.value = '7'
    expect(state.validate([{ id: 7 }])).toBe(false)
    expect(state.validate([{ id: 8 }])).toBe(true)
    expect(remembered('usage').group.value).toBe('')
    state.group.value = '8'
    state.group.value = ''
    expect(localStorage.getItem(key('usage', 1))).toBeNull()
  })

  it('remembers the accounts-only ungrouped choice', () => {
    remembered('accounts').group.value = 'ungrouped'
    expect(remembered('accounts').group.value).toBe('ungrouped')
    expect(remembered('accounts').validate([])).toBe(false)
    localStorage.setItem(key('usage', 1), 'ungrouped')
    expect(remembered('usage').group.value).toBe('')
  })

  it.each(['-1', '0', 'NaN', '1.5', 'Infinity', '9007199254740992', '{broken'])('ignores invalid saved ID %s', value => {
    localStorage.setItem(key('usage', 1), value)
    expect(remembered('usage').group.value).toBe('')
  })

  it('keeps filters usable when browser storage is disabled', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('disabled') })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('disabled') })
    const state = remembered('accounts')
    state.group.value = '7'
    expect(state.group.value).toBe('7')
  })
})
