import { apiClient } from '../client'

export type ModerationMode = 'off' | 'observe' | 'pre_block'
export type ContentModerationAuditProvider = 'openai_moderations' | 'ai_chat'
export type AIAuditFailurePolicy = 'allow' | 'block'
export type AIAuditThinkingMode = 'disabled' | 'enabled'
export type AIAuditReasoningEffort = 'adaptive' | 'low' | 'high' | 'max'
export type KeywordBlockingMode = 'keyword_only' | 'keyword_and_api' | 'api_only'
export type ContentModerationModelFilterType = 'all' | 'include' | 'exclude'
export type ContentModerationScopeFilterType = 'all' | 'include' | 'exclude'
export type ContentModerationAuditStatus = 'success' | 'skipped' | 'incomplete' | 'error'
export type ContentModerationSideEffectStatus = 'pending' | 'completed' | 'partial' | 'failed' | 'not_applicable'
export type ContentModerationNotificationStatus = 'pending' | 'sent' | 'deduplicated' | 'not_required' | 'failed'
export type ContentModerationUnbanMode = 'restore_only' | 'restore_and_clear_risk' | 'clear_risk_only'

export interface ContentModerationModelFilter {
  type: ContentModerationModelFilterType
  models: string[]
}

export interface ContentModerationUserFilter {
  type: ContentModerationScopeFilterType
  user_ids: number[]
}

export interface ContentModerationAccountFilter {
  type: ContentModerationScopeFilterType
  account_ids: number[]
}

export interface ContentModerationProviderProfile {
  base_url: string
  model: string
  proxy_id: number | null
  api_key_configured: boolean
  api_key_count: number
  api_key_masks: string[]
  timeout_ms: number
  retry_count: number
}

export interface ContentModerationAIChatProfile extends ContentModerationProviderProfile {
  confidence_threshold: number
  cache_enabled: boolean
  cache_ttl_seconds: number
  system_prompt: string
  recommended_system_prompt: string
  recommended_prompt_version: string
  system_prompt_version: string
  uses_recommended_system_prompt: boolean
  failure_policy: AIAuditFailurePolicy
  max_input_chars: number
  synchronous_budget_ms: number
  fast_input_chars: number
  fallback_input_chars: number
  thinking_mode: AIAuditThinkingMode
  reasoning_effort: AIAuditReasoningEffort
  risk_levels_enabled: boolean
  observe_threshold: number
  session_risk_enabled: boolean
  session_risk_ttl_minutes: number
  session_risk_half_life_minutes: number
  session_risk_block_cooldown_minutes: number
  actor_risk_enabled: boolean
  incremental_audit_enabled?: boolean
  input_provenance_v2_enabled?: boolean
  deterministic_risk_v2_enabled?: boolean
  recent_user_turns?: number
  summary_max_chars?: number
  full_review_threshold?: number
  full_review_risk_delta?: number
  periodic_full_review_turns?: number
  full_review_max_input_chars?: number
  fast_max_output_tokens?: number
  full_max_output_tokens?: number
  max_review_max_output_tokens?: number
  audit_context_ttl_minutes?: number
  pricing_configured?: boolean
  pricing_version?: string
  uncached_input_usd_per_million_tokens?: number | null
  cached_input_usd_per_million_tokens?: number | null
  output_usd_per_million_tokens?: number | null
}

export interface ContentModerationConfig {
  enabled: boolean
  mode: ModerationMode
  audit_provider?: ContentModerationAuditProvider
  openai_moderations?: ContentModerationProviderProfile
  ai_chat?: ContentModerationAIChatProfile
  base_url: string
  model: string
  proxy_id: number | null
  api_key_configured: boolean
  api_key_masked: string
  api_key_count: number
  api_key_masks: string[]
  api_key_statuses: ContentModerationAPIKeyStatus[]
  timeout_ms: number
  sample_rate: number
  all_groups: boolean
  group_ids: number[]
  record_non_hits: boolean
  thresholds: Record<string, number>
  worker_count: number
  queue_size: number
  block_status: number
  block_message: string
  email_on_hit: boolean
  auto_ban_enabled: boolean
  ban_threshold: number
  violation_window_hours: number
  retry_count: number
  hit_retention_days: number
  non_hit_retention_days: number
  pre_hash_check_enabled: boolean
  blocked_keywords: string[]
  keyword_blocking_mode: KeywordBlockingMode
  model_filter: ContentModerationModelFilter
  user_filter: ContentModerationUserFilter
  account_filter: ContentModerationAccountFilter
  cyber_policy_exclude_from_ban_count: boolean
}

export type ContentModerationAPIKeyStatusValue = 'unknown' | 'ok' | 'error' | 'frozen'

export interface ContentModerationAPIKeyStatus {
  index: number
  key_hash: string
  masked: string
  status: ContentModerationAPIKeyStatusValue
  failure_count: number
  success_count: number
  last_error: string
  last_checked_at?: string
  frozen_until?: string
  last_latency_ms: number
  last_http_status: number
  last_tested: boolean
  configured: boolean
}

export interface TestContentModerationAPIKeysPayload {
  api_keys?: string[]
  audit_provider?: ContentModerationAuditProvider
  base_url?: string
  model?: string
  timeout_ms?: number
  // null/undefined 沿用已保存配置的代理；0 强制直连；>0 指定代理
  proxy_id?: number
  prompt?: string
  images?: string[]
  ai_confidence_threshold?: number
  ai_system_prompt?: string
  ai_max_input_chars?: number
  ai_synchronous_budget_ms?: number
  ai_fast_input_chars?: number
  ai_fallback_input_chars?: number
  ai_thinking_mode?: AIAuditThinkingMode
  ai_reasoning_effort?: AIAuditReasoningEffort
  ai_risk_levels_enabled?: boolean
  ai_observe_threshold?: number
  ai_session_risk_enabled?: boolean
  ai_session_risk_ttl_minutes?: number
  ai_session_risk_half_life_minutes?: number
  ai_session_risk_block_cooldown_minutes?: number
  ai_actor_risk_enabled?: boolean
}

export interface TestContentModerationAPIKeysResponse {
  items: ContentModerationAPIKeyStatus[]
  audit_result?: ContentModerationTestAuditResult
  audit_error?: ContentModerationTestAuditError
  image_count: number
}

export interface ContentModerationTestAuditError {
  code: string
  message: string
  http_status?: number
}

export interface ContentModerationTestAuditResult {
  flagged: boolean
  risk_score: number
  risk_tier: string
  categories: string[]
  signals: string[]
  highest_category: string
  highest_score: number
  composite_score: number
  category_scores: Record<string, number>
  thresholds: Record<string, number>
  reason?: string
  review_incomplete: boolean
  review_error?: string
  scope?: 'request'
}

export interface UpdateContentModerationConfig {
  enabled?: boolean
  mode?: ModerationMode
  audit_provider?: ContentModerationAuditProvider
  base_url?: string
  model?: string
  // undefined 不修改；0 清除（直连）；>0 指定代理
  proxy_id?: number
  api_key?: string
  api_keys?: string[]
  api_keys_mode?: 'append' | 'replace'
  delete_api_key_hashes?: string[]
  clear_api_key?: boolean
  timeout_ms?: number
  sample_rate?: number
  all_groups?: boolean
  group_ids?: number[]
  record_non_hits?: boolean
  thresholds?: Record<string, number>
  worker_count?: number
  queue_size?: number
  block_status?: number
  block_message?: string
  email_on_hit?: boolean
  auto_ban_enabled?: boolean
  ban_threshold?: number
  violation_window_hours?: number
  retry_count?: number
  hit_retention_days?: number
  non_hit_retention_days?: number
  pre_hash_check_enabled?: boolean
  blocked_keywords?: string[]
  keyword_blocking_mode?: KeywordBlockingMode
  model_filter?: ContentModerationModelFilter
  user_filter?: ContentModerationUserFilter
  account_filter?: ContentModerationAccountFilter
  cyber_policy_exclude_from_ban_count?: boolean
  ai_confidence_threshold?: number
  ai_cache_enabled?: boolean
  ai_cache_ttl_seconds?: number
  ai_system_prompt?: string
  ai_failure_policy?: AIAuditFailurePolicy
  ai_max_input_chars?: number
  ai_synchronous_budget_ms?: number
  ai_fast_input_chars?: number
  ai_fallback_input_chars?: number
  ai_thinking_mode?: AIAuditThinkingMode
  ai_reasoning_effort?: AIAuditReasoningEffort
  ai_risk_levels_enabled?: boolean
  ai_observe_threshold?: number
  ai_session_risk_enabled?: boolean
  ai_session_risk_ttl_minutes?: number
  ai_session_risk_half_life_minutes?: number
  ai_session_risk_block_cooldown_minutes?: number
  ai_actor_risk_enabled?: boolean
  ai_incremental_audit_enabled?: boolean
  ai_input_provenance_v2_enabled?: boolean
  ai_deterministic_risk_v2_enabled?: boolean
  ai_recent_user_turns?: number
  ai_summary_max_chars?: number
  ai_full_review_threshold?: number
  ai_full_review_risk_delta?: number
  ai_periodic_full_review_turns?: number
  ai_full_review_max_input_chars?: number
  ai_fast_max_output_tokens?: number
  ai_full_max_output_tokens?: number
  ai_max_review_max_output_tokens?: number
  ai_audit_context_ttl_minutes?: number
  ai_pricing_configured?: boolean
  ai_pricing_version?: string
  ai_uncached_input_usd_per_million_tokens?: number
  ai_cached_input_usd_per_million_tokens?: number
  ai_output_usd_per_million_tokens?: number
}

export interface ContentModerationRuntimeStatus {
  enabled: boolean
  risk_control_enabled: boolean
  mode: ModerationMode
  worker_count: number
  max_workers: number
  active_workers: number
  idle_workers: number
  queue_size: number
  queue_length: number
  queue_usage_percent: number
  enqueued: number
  dropped: number
  processed: number
  errors: number
  pre_block_active: number
  pre_block_checked: number
  pre_block_allowed: number
  pre_block_blocked: number
  pre_block_errors: number
  pre_block_avg_latency_ms: number
  pre_block_api_key_active: number
  pre_block_api_key_available_count: number
  pre_block_api_key_total_calls: number
  pre_block_api_key_loads: ContentModerationAPIKeyLoad[]
  api_key_statuses: ContentModerationAPIKeyStatus[]
  audit_fast_calls: number
  audit_full_calls: number
  audit_max_calls: number
  audit_result_cache_hits: number
  audit_prompt_tokens: number
  audit_cached_input_tokens: number
  audit_uncached_input_tokens: number
  audit_output_tokens: number
  audit_usage_complete?: number
  audit_usage_unknown: number
  audit_input_chars: number
  metrics_started_at?: string
  audit_estimated_cost_usd?: number | null
  audit_cost_coverage?: 'no_samples' | 'unknown' | 'partial' | 'complete'
  audit_cost_complete?: boolean
  audit_cost_partial?: boolean
  audit_cost_priced_samples?: number
  audit_cost_unpriced_samples?: number
  audit_cost_by_pricing_version_usd?: Record<string, number>
  business_actual_cost_usd?: number | null
  audit_cost_per_business_usd?: number | null
  audit_stage_latency?: Record<string, ContentModerationLatencySummary>
  audit_session_sources?: Record<string, number>
  audit_prefix_continuity?: Record<string, number>
  flagged_hash_count: number
  last_cleanup_at?: string
  last_cleanup_deleted_hit: number
  last_cleanup_deleted_non_hit: number
}

export interface ContentModerationLatencySummary {
  count: number
  average_ms: number
  p95_upper_ms: number
}

export interface ContentModerationAPIKeyLoad {
  index: number
  key_hash: string
  masked: string
  status: ContentModerationAPIKeyStatusValue
  active: number
  total: number
  success: number
  errors: number
  avg_latency_ms: number
  last_latency_ms: number
  last_http_status: number
}

export interface ContentModerationLog {
  id: number
  request_id: string
  user_id: number | null
  user_email: string
  api_key_id: number | null
  api_key_name: string
  group_id: number | null
  group_name: string
  endpoint: string
  provider: string
  model: string
  mode: string
  action: string
  flagged: boolean
  highest_category: string
  highest_score: number
  matched_keyword: string
  category_scores: Record<string, number>
  threshold_snapshot: Record<string, number>
  input_excerpt: string
  upstream_latency_ms: number | null
  error: string
  audit_status: ContentModerationAuditStatus
  audit_code: string
  audit_retryable: boolean
  violation_count: number
  auto_banned: boolean
  email_sent: boolean
  side_effect_status: ContentModerationSideEffectStatus
  notification_status: ContentModerationNotificationStatus
  side_effect_error: string
  moderation_ban_active: boolean
  unban_block_reason?: string
  user_status: string
  queue_delay_ms: number | null
  audit_details?: ContentModerationAuditDetails
  created_at: string
}

export interface ContentModerationLocalRuleMatch {
  rule_id?: string
  rule_version?: string
  level?: string
  target_kind?: string
  target_source?: string
  matched_intent?: string[]
  matched_target?: string[]
  matched_action?: string[]
  matched_excerpt?: string
  lexical_types?: string[]
  negation_detected?: boolean
  defensive_detected?: boolean
  metadata_excluded?: string[]
}

export interface ContentModerationAuditStageDetails {
  stage: string
  provider_called: boolean
  result_cache_hit: boolean
  usage_known: boolean
  failed: boolean
  input_chars?: number
  latency_ms?: number
  prompt_tokens?: number
  cached_input_tokens?: number
  uncached_input_tokens?: number
  output_tokens?: number
}

export interface ContentModerationAuditDetails {
  audit_stage?: string
  escalation_reasons?: string[]
  session_source?: string
  turn_count?: number
  input_chars?: number
  prompt_tokens?: number
  cached_input_tokens?: number
  uncached_input_tokens?: number
  output_tokens?: number
  usage_unknown?: boolean
  result_cache_hit?: boolean
  provider_applicable?: boolean
  result_cache_applicable?: boolean
  review_applicable?: boolean
  sub2api_result_cache_hit?: boolean
  provider_prefix_cache_ratio?: number
  prefix_epoch?: number
  prefix_continuity?: boolean
  prefix_baseline?: boolean
  prefix_break_reason?: string
  input_truncated?: boolean
  review_complete?: boolean
  audit_target_kind?: string
  audit_target_source?: string
  has_explicit_user_turn?: boolean
  trusted_client?: boolean
  audit_target_excerpt?: string
  supporting_context_excerpt?: string
  trusted_signals?: string[]
  ignored_metadata?: string[]
  audit_key_hash?: string
  input_hash?: string
  hash_scope?: string
  hash_state?: string
  hash_promotion_reason?: string
  policy_version?: string
  review_incomplete?: boolean
  model_reason?: string
  model_signals?: string[]
  local_rule_level?: string
  local_rule_match?: ContentModerationLocalRuleMatch
  stages?: ContentModerationAuditStageDetails[]
}

export interface ListContentModerationLogsParams {
  page?: number
  page_size?: number
  result?: string
  group_id?: number
  endpoint?: string
  search?: string
  from?: string
  to?: string
}

export interface ContentModerationLogsResponse {
  items: ContentModerationLog[]
  total: number
  page: number
  page_size: number
  pages: number
}

export interface ContentModerationUnbanUserResponse {
  user_id: number
  status: string
  mode: ContentModerationUnbanMode
  restored: boolean
  risk_state_cleared: boolean
  warning?: string
}

export interface ContentModerationUnbanUserRequest {
  mode: ContentModerationUnbanMode
}

export interface DeleteFlaggedHashResponse {
  input_hash: string
  deleted: boolean
}

export interface ClearFlaggedHashesResponse {
  deleted: number
}

export async function getConfig(): Promise<ContentModerationConfig> {
  const { data } = await apiClient.get<ContentModerationConfig>('/admin/risk-control/config')
  return data
}

export async function updateConfig(
  payload: UpdateContentModerationConfig
): Promise<ContentModerationConfig> {
  const { data } = await apiClient.put<ContentModerationConfig>('/admin/risk-control/config', payload)
  return data
}

export async function getStatus(): Promise<ContentModerationRuntimeStatus> {
  const { data } = await apiClient.get<ContentModerationRuntimeStatus>('/admin/risk-control/status')
  return data
}

export async function testAPIKeys(
  payload: TestContentModerationAPIKeysPayload = {}
): Promise<TestContentModerationAPIKeysResponse> {
  const { data } = await apiClient.post<TestContentModerationAPIKeysResponse>('/admin/risk-control/api-keys/test', payload)
  return data
}

export async function listLogs(
  params: ListContentModerationLogsParams = {}
): Promise<ContentModerationLogsResponse> {
  const { data } = await apiClient.get<ContentModerationLogsResponse>('/admin/risk-control/logs', {
    params,
  })
  return data
}

export async function unbanUser(
  userID: number,
  payload: ContentModerationUnbanUserRequest = { mode: 'restore_and_clear_risk' }
): Promise<ContentModerationUnbanUserResponse> {
  const { data } = await apiClient.post<ContentModerationUnbanUserResponse>(
    `/admin/risk-control/users/${userID}/unban`,
    payload
  )
  return data
}

export async function deleteFlaggedHash(inputHash: string): Promise<DeleteFlaggedHashResponse> {
  const { data } = await apiClient.delete<DeleteFlaggedHashResponse>('/admin/risk-control/hashes', {
    data: { input_hash: inputHash },
  })
  return data
}

export async function clearFlaggedHashes(): Promise<ClearFlaggedHashesResponse> {
  const { data } = await apiClient.delete<ClearFlaggedHashesResponse>('/admin/risk-control/hashes/all')
  return data
}

export const riskControlAPI = {
  getConfig,
  updateConfig,
  getStatus,
  testAPIKeys,
  listLogs,
  unbanUser,
  deleteFlaggedHash,
  clearFlaggedHashes,
}

export default riskControlAPI
