# Codex One-Click Test Report

## Result

Final result: **PASS**. Independent testing and two-pass code review found no open P1, P2, or confirmed P3 blocker within the agreed scope. Linux BusyBox/Alpine compatibility is explicitly deferred.

## Environment

- Date: 2026-08-05
- Repository: `D:\sub2api`
- Branch: `custom`
- Baseline HEAD: `e83f49b99`
- Frontend: `http://127.0.0.1:5173/keys`
- Backend: `http://127.0.0.1:18080`

## Automated Results

| Test area | Result | Evidence |
| --- | --- | --- |
| Focused one-click regression | PASS | 4 files, 55 tests |
| Complete frontend Vitest suite | PASS | Exit code 0 |
| TypeScript | PASS | `vue-tsc --noEmit` |
| Full ESLint | PASS | No findings |
| Production build | PASS | 1011 modules transformed |
| Diff whitespace validation | PASS | `git diff --check` |

Focused suite distribution:

- `ccswitchImport.spec.ts`: 18 tests
- `codexOneClick.spec.ts`: 8 tests
- `CodexOneClickModal.spec.ts`: 17 tests
- `KeysView.spec.ts`: 12 tests

## Functional Test Matrix

| ID | Scenario | Expected result | Result |
| --- | --- | --- | --- |
| F-01 | Open one-click modal from key row `11` | Modal shows key name `11` and masked current key | PASS |
| F-02 | Open different key rows sequentially | No stale key/name binding | PASS |
| F-03 | Open active key in a non-Codex group | Action remains enabled | PASS |
| F-04 | Open new-user guide | Codex and CC Switch official links are available | PASS |
| F-05 | Inspect CC Switch method | Client is Codex and model is `gpt-5.6` | PASS |
| F-06 | Normalize root, trailing slash, `/v1`, and `/v1/` | Endpoint contains exactly one `/v1` | PASS |
| F-07 | Generate CC Switch usage query | URL contains exactly one `/v1/usage` | PASS |
| F-08 | Switch Windows/macOS/Linux scripts | Correct preview and run command are shown | PASS |
| F-09 | Copy generated script | Visible success state is shown | PASS |
| F-10 | Download script generation | Correct content and OS filename are generated | PASS |
| F-11 | Download click throws | Created Blob URL is still revoked | PASS |
| F-12 | Close/unmount after CC Switch detection begins | Delayed protocol failure is cancelled | PASS |
| F-13 | Arrow/Home/End on access-method tabs | Selection, tabindex, focus, and panel update together | PASS |
| F-14 | Arrow/Home/End on OS radios | Checked state, tabindex, and focus update together | PASS |
| F-15 | Delay protocol failure for 1.8 seconds | No premature failure before timeout | PASS |
| F-16 | Blur or hide page during protocol launch | Pending failure is cancelled | PASS |
| F-17 | Generate Codex config from production root variants | Exactly one `/v1` is written | PASS |

## Security Test Matrix

| ID | Scenario | Result |
| --- | --- | --- |
| S-01 | Raw key absent from initial modal and script preview | PASS |
| S-02 | Real Base64 config/auth payload absent from preview | PASS |
| S-03 | Explicit copy/download uses the selected row key | PASS |
| S-04 | CC Switch payload is built only after explicit action | PASS |
| S-05 | No key output to console/localStorage/sessionStorage | PASS |
| S-06 | Backup and rollback scripts are generated | PASS |

## Browser Results

- Desktop 1280 x 720: controls visible, modal bounded, no overlap.
- Mobile 375 x 812: document and dialog stabilize at 375 px with no horizontal overflow.
- Keyboard browser check: tab and OS radio selection move with focus and ARIA state.
- Console: no errors or warnings after the final interaction flow.
- Current-row verification: row `11` displayed `sk-451...4f9e`; no complete key was exposed.
- Final post-review smoke used the current row `123`, verified the selected tab/tabpanel keyboard state, and repeated the stable 375 px layout check.
- Real CC Switch protocol launch: PASS. The confirmation dialog received Codex, provider `Sub2API`, endpoint `https://ai.vote520.com/v1`, a masked selected key, model `gpt-5.6`, and enabled usage querying.
- The imported local test key returned HTTP 401 when queried against the production domain, which is expected because local and production credentials are isolated. The production `/v1/usage` route was independently confirmed to exist and require authentication.

## Service Results

- Frontend API key page returned HTTP 200.
- Backend health returned HTTP 200 with `{"status":"ok"}`.

## Expected Warnings And Limitations

- Existing Browserslist database age warning.
- Existing Vite dynamic/static import and large-chunk warnings.
- No physical macOS/Linux execution was performed.
- BusyBox/Alpine Linux compatibility is outside this delivery scope.
- A successful authenticated usage query should still use a temporary key created by the production environment when performing production release validation.

## Independent Review

- Initial review: three P2 findings covering Codex `/v1`, protocol detection, and ignored documents.
- Remediation review: all three findings closed.
- Final verdict: PASS for release candidate validation.
- Full evidence: [CODE_REVIEW_REPORT.md](./CODE_REVIEW_REPORT.md).
