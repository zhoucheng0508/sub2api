# Codex One-Click Task Checklist

Status legend: `[x]` complete, `[ ]` pending, `[~]` explicitly deferred.

## Repository And Coordination

- [x] Work against the local `custom` branch and baseline `e83f49b99`.
- [x] Preserve pre-existing uncommitted changes.
- [x] Use separate development and test agents with independent verification.
- [x] Add a separate review agent for final code review.
- [x] Product owner approved creation of the local delivery commit.
- [ ] Push only after separate product-owner approval.

## Product Work

- [x] Add the API key page one-click entry points.
- [x] Enable eligible active keys independently of group platform.
- [x] Bind modal actions to the current row key.
- [x] Add the new-user Codex and CC Switch guide.
- [x] Add the installed CC Switch flow.
- [x] Set the default and review model to `gpt-5.6`.
- [x] Normalize Codex and CC Switch endpoints to one `/v1` suffix.
- [x] Normalize usage queries to one `/v1/usage` suffix.
- [x] Add Windows, macOS, and Linux script preview/copy/download controls.
- [x] Add backup and rollback generation.
- [x] Redact raw keys and generated Base64 payloads in previews.
- [x] Add responsive desktop/mobile layouts.

## Remediation Work

- [x] Prevent `/v1/v1/usage` in both CC Switch entry points.
- [x] Clear protocol detection timers on close, reopen, and unmount.
- [x] Add tab/tabpanel ARIA relationships and roving keyboard focus.
- [x] Add radio-group keyboard focus for both OS selectors.
- [x] Revoke Blob URLs after normal and exceptional download paths.
- [~] BusyBox/Alpine `base64` compatibility deferred by product-owner decision.

## Verification

- [x] Focused one-click automated suite.
- [x] Complete frontend Vitest suite.
- [x] TypeScript project checking.
- [x] Full frontend ESLint check.
- [x] Production frontend build.
- [x] Desktop browser validation at 1280 x 720.
- [x] Mobile browser validation at 375 x 812.
- [x] Browser console inspection.
- [x] Backend health check.
- [x] Independent tester review.
- [x] Independent reviewer P2 findings resolved and re-reviewed.
- [x] Final reviewer verdict: PASS for release candidate validation.
- [x] Run a real CC Switch external-protocol launch and validate the imported provider payload.
