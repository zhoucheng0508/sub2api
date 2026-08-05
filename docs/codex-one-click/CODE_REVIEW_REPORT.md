# Codex One-Click Code Review Report

## 1. Overall verdict

**PASS - suitable for release candidate validation.**

The initial review found three P2 issues. All three have now been remediated and independently re-reviewed. No open P1, P2, or confirmed P3 defect remains within the agreed delivery scope.

Current-row binding, eligibility guards, key redaction, Codex and CC Switch endpoint normalization, protocol lifecycle handling, Blob cleanup, script generation, and ARIA keyboard behavior are coherent. Linux BusyBox/Alpine compatibility remains explicitly deferred by the product owner and is not a blocker.

## 2. Findings by severity

### P1

None found in either review pass.

### P2

No open P2 finding remains.

#### Initial P2-1: Generated Codex configuration did not implement the documented `/v1` endpoint contract - RESOLVED

- Initial finding: the first implementation trimmed the root URL but wrote it directly to `config.toml`, so `https://ai.vote520.com` did not become the documented `https://ai.vote520.com/v1`.
- Fix verified: `frontend/src/utils/codexOneClick.ts:1` imports the shared normalizer and `frontend/src/utils/codexOneClick.ts:35` applies `normalizeV1Endpoint` before TOML escaping. `frontend/src/utils/ccswitchImport.ts:23` owns the shared normalization behavior.
- Contract result: root, trailing slash, existing `/v1`, and `/v1/` variants all produce exactly `https://ai.vote520.com/v1`; CC Switch usage remains exactly `https://ai.vote520.com/v1/usage`.
- Test evidence: `frontend/src/utils/__tests__/codexOneClick.spec.ts:33` covers all four production-domain variants and rejects `/v1/v1`. Existing CC Switch normalization cases also pass.
- Wider-surface check: `UseKeyModal.vue` uses the same helper, so its Codex and Codex WebSocket configurations now follow the same documented endpoint contract.

#### Initial P2-2: CC Switch protocol detection could report a premature false failure - RESOLVED

- Initial finding: a 100 ms `document.hasFocus()` heuristic could report CC Switch as missing before a successfully launched external application blurred the browser.
- Fix verified: `frontend/src/components/keys/CodexOneClickModal.vue:270` sets a 1.8 second detection window. `frontend/src/components/keys/CodexOneClickModal.vue:366` through `399` installs blur and visibility listeners, cancels on either successful lifecycle signal, removes listeners with the timer, and checks that the modal remains open before emitting failure.
- Lifecycle result: repeated starts clear stale timers/listeners; close, reopen, blur, hidden visibility, synchronous launch failure, and unmount all clean up coherently.
- Test evidence: `frontend/src/components/keys/__tests__/CodexOneClickModal.spec.ts:128` verifies no early failure before 1.8 seconds; lines 144, 156, 168, and 184 cover delayed blur, visibility change, close/reopen cleanup, and unmount cleanup.

#### Initial P2-3: Delivery documents were ignored by Git - RESOLVED

- Initial finding: the repository's `docs/*` rule caused the requested delivery, checklist, test, coordination, and review documents to be omitted from a normal commit.
- Fix verified: `.gitignore:138` through `140` narrowly re-include `docs/codex-one-click/` and its Markdown files while leaving the broader documentation ignore policy intact.
- Git result: `git ls-files --others --exclude-standard docs/codex-one-click` lists all six delivery files, confirming they are visible to a normal add/commit workflow.

### P3

No confirmed P3 code defect was found.

An explicit parent-level close/reopen key-binding unit test would still improve defense in depth. The implementation clears `codexOneClickKey` on close and assigns the clicked row on every guarded open, while existing unit and browser evidence verifies row selection and sequential-key behavior. This is a non-blocking coverage suggestion.

## 3. Checklist result

| Review check | Final result | Notes |
| --- | --- | --- |
| Current row identity/key cannot become stale | PASS | Dedicated selected-key state, guarded open, and clear on close. |
| Inactive/empty keys cannot invoke one-click | PASS | Disabled controls plus handler-level eligibility guard. |
| Raw key/Base64 absent before explicit action | PASS | Masked display and redacted preview; full payload only on explicit import/copy/download. |
| Endpoints and usage contain exactly one `/v1` | PASS | Shared normalizer covers Codex, CC Switch endpoint, and usage URL. |
| Production domain root and `/v1` variants | PASS | Automated cases cover root, slash, `/v1`, and `/v1/`. |
| CC Switch provider/client/model/parameters | PASS | Codex app, model `gpt-5.6`, normalized endpoint, current key, and usage script. |
| OS scripts produce valid TOML/JSON | PASS | Escaped TOML and `JSON.stringify`; UTF-8-safe Base64 payload creation. |
| Backup/replacement/permissions/rollback | PASS | Backups, temporary writes, restore scripts, and Unix permissions are coherent. |
| Protocol detection lifecycle | PASS | 1.8 second window; blur/hidden cancel; close/reopen/unmount cleanup. |
| Blob URL cleanup | PASS | URL is revoked after successful and exceptional click paths. |
| Tabs/radios ARIA and keyboard contract | PASS | Linked IDs, roving tabindex, arrows, Home/End, and focus updates. |
| Desktop/mobile overflow | PASS by supplied evidence | Prior 1280 x 720 and 375 x 812 browser checks remain applicable. |
| Behavioral test quality | PASS | Tests cover action-time secrets, endpoint contracts, timers/listeners, scripts, and accessibility behavior. |
| Delivery files visible to Git | PASS | Narrow `.gitignore` exception verified with Git. |
| No unrelated accidental change | PASS | Reviewed feature changes are scoped; known pre-existing `.gitignore` entries were preserved. |
| BusyBox/Alpine compatibility | DEFERRED | Explicit product-owner decision; not a blocker. |

## 4. Test and tooling evidence

Second review pass independently reran:

- Focused Vitest suite: **PASS**, 4 files / 55 tests.
  - `ccswitchImport.spec.ts`: 18 passed
  - `codexOneClick.spec.ts`: 8 passed
  - `CodexOneClickModal.spec.ts`: 17 passed
  - `KeysView.spec.ts`: 12 passed
- ESLint on all changed one-click source and test files: **PASS**.
- `vue-tsc --noEmit`: **PASS**.
- `git diff --check`: **PASS**; only the existing Git line-ending notice for `.gitignore` was emitted.
- `git check-ignore` / `git ls-files --others --exclude-standard`: the scoped documentation exception is effective.

First review evidence also passed at 4 files / 49 focused tests, ESLint, TypeScript, and whitespace validation. The increase to 55 tests is attributable to the remediation coverage.

## 5. Residual risks

- macOS and Linux scripts were generated and statically reviewed but not executed on physical macOS/Linux hosts.
- BusyBox/Alpine `base64 --decode` support is deliberately deferred.
- Custom-protocol launch detection remains an OS/browser lifecycle heuristic. The 1.8 second blur/visibility design materially reduces the original premature-failure risk and is covered by deterministic tests, but physical CC Switch launch behavior should remain part of release smoke testing.
- Existing build warnings about Browserslist data, mixed dynamic/static imports, and large chunks are outside this change.
- At review time the working tree was uncommitted. The delivery was subsequently committed locally; this reviewer did not commit or push.

## 6. Release recommendation

**Approve the implementation for release candidate validation.**

No code-review blocker remains in the agreed scope. Before production release, retain the normal final gate already defined by the delivery checklist: production build, desktop/mobile smoke flow, one real CC Switch launch on the target desktop environment, and confirmation that the six `docs/codex-one-click/*.md` artifacts are staged with the feature.

Linux BusyBox/Alpine compatibility may proceed as a separately tracked follow-up and does not prevent this release.

## 7. CC Switch automatic download review

### Final verdict

**PASS - no open P1, P2, or confirmed P3 finding.**

The automatic-download implementation is suitable for release candidate validation. It resolves metadata only and sends the browser to a validated, versioned GitHub asset; the application server does not proxy installer bytes.

### Security and behavior review

- **Open redirect / arbitrary navigation: PASS.** `backend/internal/service/ccswitch_download_service.go:247` accepts only HTTPS `github.com` URLs with no user info, explicit port, query, or fragment, and requires the exact escaped path prefix `/farion1231/cc-switch/releases/download/`. The public request cannot supply a URL or repository. `frontend/src/api/downloads.ts:12` navigates only to the server-returned value after that validation.
- **SSRF / arbitrary proxy: PASS.** `backend/internal/service/ccswitch_download_service.go:17` fixes the repository to `farion1231/cc-switch`. The handler accepts only the `os` and `arch` enums and never calls the generic download/proxy methods. The server returns JSON metadata rather than fetching the installer asset.
- **Public handler surface: PASS.** `backend/internal/handler/ccswitch_download_handler.go:20` maps invalid enums to 400, a missing compatible asset to 404, and upstream failure to a generic 502 without exposing internal errors. The route is intentionally public and exposes only fixed-repository release metadata.
- **Failure amplification and cache invalidation: PASS.** `backend/internal/service/ccswitch_download_service.go:80` uses `singleflight.DoChan`, a 15-minute success cache, one-minute negative cache, and stale-on-error behavior. No network call is made while the cache mutex is held. A service-owned 30-second context at line 90 prevents the first public caller from cancelling or poisoning the shared fetch, while lines 112-119 still let each caller stop waiting independently.
- **Nil and upstream anomalies: PASS.** A nil release is converted to a cached upstream error at `backend/internal/service/ccswitch_download_service.go:96`, preventing a dereference panic. An invalid release page URL falls back to the fixed official Releases URL.
- **GitHub asset URL validation: PASS.** Tests reject HTTP, wrong host, lookalike subdomain, user info, port, wrong repository, lookalike repository path, encoded path separator, and query-based redirect attempts. Checksum, signature, and source artifacts are excluded before selection.
- **Current and future asset naming: PASS with safe fallback.** The reviewer queried the live fixed GitHub API during review: latest was `v3.19.1`, with neutral Windows x64 MSI, explicit Windows arm64 MSI, neutral macOS DMG, and explicit Linux x86_64/arm64 AppImages. `backend/internal/service/ccswitch_download_service_test.go:197` captures those real naming patterns. Unknown future names fail closed to 404, and the frontend always displays the fixed official Releases link.
- **Architecture selection: PASS.** `backend/internal/service/ccswitch_download_service.go:207` requires explicit arm64 tokens for Windows and Linux, preventing an x64-neutral installer from being mislabeled as arm64. The neutral macOS DMG is deliberately allowed for both architectures because the current release publishes one universal DMG.
- **Browser download and fallback: PASS.** `frontend/src/components/keys/CodexOneClickModal.vue:372` resolves the selected platform, then performs a same-page navigation to GitHub so the browser can follow GitHub's asset redirect and `Content-Disposition` behavior without loading a large installer Blob into application memory. The official Releases fallback is permanently visible at line 130, including when metadata resolution fails.
- **Async cancellation and stale-result handling: PASS.** `frontend/src/components/keys/CodexOneClickModal.vue:362` aborts and invalidates pending requests on OS change, architecture change, modal close, and unmount. Lines 372-389 capture the requested selection and request ID, preventing an older response from downloading the wrong installer or navigating after the modal closes.

### Remediations verified during review

The first review pass identified four issues before implementation freeze; all were corrected before this final verdict:

1. Public requests could have triggered sequential upstream calls during GitHub failure. Resolved with singleflight, negative caching, and stale-on-error.
2. The first caller's cancellation could have poisoned the shared cache. Resolved with `context.WithoutCancel` plus a bounded service timeout and a regression test.
3. Architecture-neutral Windows/Linux packages could have been selected for arm64. Resolved by requiring explicit arm64 markers outside macOS.
4. Changing selection or closing the modal during resolution could have navigated to a stale installer. Resolved with AbortController and request/selection guards.

### Independent verification

- Backend service and handler download tests: **PASS**.
- Frontend CC Switch import and modal suites: **PASS**, 2 files / 39 tests.
- Frontend ESLint on changed download files: **PASS**.
- `vue-tsc --noEmit`: **PASS**.
- Backend service, handler, route, and server compilation checks: **PASS**.
- `git diff --check`: **PASS**.

### Release recommendation

**Approve CC Switch automatic download for release candidate validation.** No review blocker remains. The final smoke gate should click each supported OS/architecture combination against the live resolver, confirm GitHub starts the expected asset download, and confirm the permanent official Releases fallback remains usable when the resolver is unavailable.
