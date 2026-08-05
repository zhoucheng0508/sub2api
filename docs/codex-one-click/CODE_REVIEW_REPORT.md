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
