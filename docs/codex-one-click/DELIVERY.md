# Codex One-Click Integration Delivery

## Delivery Metadata

- Delivery date: 2026-08-05
- Repository: `D:\sub2api`
- Branch: `custom`
- Baseline HEAD: `e83f49b99`
- Delivery state: local Git commit created; push remains pending
- Frontend acceptance URL: `http://127.0.0.1:5173/keys`
- Backend health URL: `http://127.0.0.1:18080/health`

## Delivered Scope

This delivery adds a Codex one-click integration workflow to the API key page.

1. API key page banner and row-level one-click entry points.
2. Current-row key isolation so each action uses the selected API key.
3. New-user guide with official Codex and CC Switch download links.
4. CC Switch provider import using model `gpt-5.6`.
5. Windows, macOS, and Linux setup script preview, copy, download, and run commands.
6. Configuration backup and generated restore scripts.
7. API key and Base64 payload redaction in the visible preview.
8. Responsive desktop and mobile dialog layouts.
9. Keyboard and ARIA support for access-method tabs and OS radio groups.

## Endpoint Contract

- Public service roots such as `https://ai.vote520.com` normalize to `https://ai.vote520.com/v1` for Codex and CC Switch provider endpoints.
- Roots already ending in `/v1` remain unchanged.
- CC Switch usage queries normalize to exactly `<root>/v1/usage`.
- The implementation must never produce `/v1/v1`.

## Security Contract

- The full API key is not rendered before an explicit copy, download, or import action.
- Script previews contain neither the raw key nor recoverable Base64 payloads.
- The selected row key is resolved at action time.
- No API key is written to console, local storage, or session storage.
- CC Switch receives the selected key only after an explicit import action.

## Primary Files

- `frontend/src/components/keys/CodexOneClickModal.vue`
- `frontend/src/components/keys/UseKeyModal.vue`
- `frontend/src/views/user/KeysView.vue`
- `frontend/src/utils/codexOneClick.ts`
- `frontend/src/utils/ccswitchImport.ts`
- `frontend/src/i18n/locales/en/dashboard.ts`
- `frontend/src/i18n/locales/zh/dashboard.ts`

## Test Evidence

See [TEST_REPORT.md](./TEST_REPORT.md) for automated, browser, security, and service results.

Final independent code review: **PASS**. No open P1, P2, or confirmed P3 finding remains within scope. See [CODE_REVIEW_REPORT.md](./CODE_REVIEW_REPORT.md).

## Known Limitations

- BusyBox/Alpine compatibility for Linux `base64 --decode` is intentionally excluded from this delivery at the product owner's direction.
- macOS and Linux scripts were validated through generation tests and static review, not executed on physical macOS/Linux hosts.
- Existing build warnings remain for outdated Browserslist data, mixed dynamic/static imports, and chunks larger than 500 kB.

## Handoff

- The product owner approved preparation of the local delivery commit.
- No push was authorized or performed.
- Preserve unrelated working-tree changes, including `.gitignore`.
- Before release, review [TASK_CHECKLIST.md](./TASK_CHECKLIST.md) and [CODE_REVIEW_CHECKLIST.md](./CODE_REVIEW_CHECKLIST.md).
