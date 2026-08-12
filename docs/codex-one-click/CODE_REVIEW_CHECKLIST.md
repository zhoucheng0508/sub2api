# Codex One-Click Code Review Checklist

## Reviewer Instructions

Review the current working tree against `DELIVERY.md`, `TASK_CHECKLIST.md`, and `TEST_REPORT.md`. This is a review-only assignment: do not modify files, commit, or push.

Report findings first, ordered P1 to P3, with exact file and line references. Distinguish confirmed defects from residual risks and test gaps. If no issue is found, state that explicitly.

## Scope

- `frontend/src/components/keys/CodexOneClickModal.vue`
- `frontend/src/components/keys/UseKeyModal.vue`
- `frontend/src/views/user/KeysView.vue`
- `frontend/src/utils/codexOneClick.ts`
- `frontend/src/utils/ccswitchImport.ts`
- Related locale and test files shown by `git status --short`

## Required Review Checks

- [ ] Current row identity and API key cannot become stale across open/close/reopen.
- [ ] Disabled/inactive/empty keys cannot invoke one-click actions.
- [ ] Full API keys and recoverable Base64 payloads are absent before explicit action.
- [ ] Endpoints and usage URLs contain exactly one `/v1` segment.
- [ ] The production-domain contract works for `https://ai.vote520.com` and `https://ai.vote520.com/v1`.
- [ ] CC Switch provider, client, endpoint, model, usage script, and key parameters are correct.
- [ ] Windows/macOS/Linux scripts produce valid TOML and JSON content.
- [ ] Backup, replacement, permissions, and rollback behavior are coherent.
- [ ] Protocol detection cannot emit after modal close/unmount or duplicate timers.
- [ ] Blob URLs are revoked after successful and exceptional download paths.
- [ ] Tabs and radios satisfy their declared ARIA and keyboard contracts.
- [ ] Desktop and mobile layouts do not overflow or overlap.
- [ ] Tests assert behavior rather than implementation-only details.
- [ ] No unrelated regression or accidental repository change was introduced.
- [ ] Linux BusyBox/Alpine compatibility is noted but not raised as a release blocker for this review.

## Reviewer Output

Write the result to `docs/codex-one-click/CODE_REVIEW_REPORT.md` using these sections:

1. Overall verdict
2. Findings by severity
3. Checklist result
4. Test and tooling evidence
5. Residual risks
6. Release recommendation
