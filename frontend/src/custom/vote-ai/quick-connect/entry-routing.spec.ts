import { describe, expect, it } from 'vitest'
import { getRegistrationDestination, safeRegistrationRedirect } from './entry-routing'

describe('registration destinations', () => {
  it('starts plain registration with guided setup while preserving admin and OAuth defaults', () => {
    expect(getRegistrationDestination(undefined)).toBe('/get-started')
    expect(getRegistrationDestination(undefined, { isAdmin: true })).toBe('/admin/dashboard')
    expect(getRegistrationDestination(undefined, { isPendingOAuth: true })).toBe('/dashboard')
  })

  it.each(['/purchase?plan=2', '/profile/security', '/get-started?scene=image', '/usage#recent', '/admin/users'])(
    'preserves an explicit in-app destination %s', (destination) => {
      expect(getRegistrationDestination(destination)).toBe(destination)
      expect(getRegistrationDestination(destination, { isAdmin: true, isPendingOAuth: true })).toBe(destination)
    },
  )

  it.each([undefined, null, '', ['/', '/profile'], 'https://example.com', '//example.com', '/\\example.com', '/\n/profile'])(
    'ignores a missing or unsafe destination %j', (destination) => {
      expect(safeRegistrationRedirect(destination)).toBeUndefined()
      expect(getRegistrationDestination(destination)).toBe('/get-started')
    },
  )
})
