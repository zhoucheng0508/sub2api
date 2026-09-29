/** Keep a requested in-app destination when registration interrupts navigation. */
export function safeRegistrationRedirect(value: unknown): string | undefined {
  if (typeof value !== 'string' || !value.startsWith('/') || value.startsWith('//')) return undefined
  if (value.includes('\\') || Array.from(value).some(char => char.charCodeAt(0) < 32 || char.charCodeAt(0) === 127)) return undefined
  return value
}

export function getRegistrationDestination(
  redirect: unknown,
  options: { isAdmin?: boolean; isPendingOAuth?: boolean } = {},
): string {
  const requestedDestination = safeRegistrationRedirect(redirect)
  if (requestedDestination) return requestedDestination
  if (options.isAdmin) return '/admin/dashboard'
  // Pending OAuth may resolve to an existing account or a binding flow.
  return options.isPendingOAuth ? '/dashboard' : '/get-started'
}
