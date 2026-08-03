import { apiClient } from '../client'

export type TLSFingerprintRouterMatchType = 'contains' | 'prefix' | 'exact' | 'regex'

export interface TLSFingerprintRouterRule {
  name: string
  enabled: boolean
  match_type: TLSFingerprintRouterMatchType
  pattern: string
  case_sensitive: boolean
  tls_fingerprint_profile_id: number
  upstream_user_agent?: string
  upstream_originator?: string
}

export interface TLSFingerprintRouter {
  id: number
  name: string
  description: string | null
  enabled: boolean
  rules: TLSFingerprintRouterRule[]
  created_at: string
  updated_at: string
}

export type TLSFingerprintRouterPayload = Omit<TLSFingerprintRouter, 'id' | 'created_at' | 'updated_at'>

const tlsFingerprintRouterAPI = {
  async list(): Promise<TLSFingerprintRouter[]> {
    const { data } = await apiClient.get<TLSFingerprintRouter[]>('/admin/tls-fingerprint-routers')
    return data
  },
  async create(payload: TLSFingerprintRouterPayload): Promise<TLSFingerprintRouter> {
    const { data } = await apiClient.post<TLSFingerprintRouter>('/admin/tls-fingerprint-routers', payload)
    return data
  },
  async update(id: number, payload: TLSFingerprintRouterPayload): Promise<TLSFingerprintRouter> {
    const { data } = await apiClient.put<TLSFingerprintRouter>(`/admin/tls-fingerprint-routers/${id}`, payload)
    return data
  },
  async delete(id: number): Promise<void> {
    await apiClient.delete(`/admin/tls-fingerprint-routers/${id}`)
  }
}

export default tlsFingerprintRouterAPI
