import { apiClient } from '../client'

export interface UpstreamBalance {
  status: 'ok' | 'unavailable'
  provider?: 'sub2api' | 'newapi'
  scope?: 'wallet' | 'key' | 'subscription' | 'upstream'
  amount?: number
  unit?: string
  unlimited?: boolean
  checked_at: string
}

export async function getUpstreamBalance(id: number, signal?: AbortSignal): Promise<UpstreamBalance> {
  const { data } = await apiClient.get<UpstreamBalance>(`/admin/accounts/${id}/upstream-balance`, { signal })
  return data
}
