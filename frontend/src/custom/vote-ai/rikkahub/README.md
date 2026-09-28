# RikkaHub Responses import

Independent frontend customization; no backend, database, payment or plugin protocol changes.
Upstream integration points are `UseKeyModal.vue` and two locale index imports,
marked `CUSTOM(VOTE-AI-RIKKAHUB)`. All feature logic and messages stay in this directory.
Open an OpenAI key's **Use key → RikkaHub** tab. The module reads the authenticated
Codex model catalog via the existing API helper, accepts one selected model, and
generates the private QR locally after an explicit click. No new key is issued.

The import contract is RikkaHub 2.5.5 `ai-provider:v1:` + Base64(UTF-8 JSON).
`useResponseApi: true` and `responsesPath: /responses` are mandatory. Model image
capability and reasoning effort come from catalog metadata. This feature only
presets text/vision chat; it does not enable additional app tools or server access.
The catalog is a discovery aid, not a replacement for server authorization.

Key changes, model changes, closing the dialog and switching tabs clear QR state.
Stale asynchronous model/QR results cannot restore an old key's QR.
Do not log payloads or add them to URLs, analytics, localStorage or remote QR services.
The official download link is separate and uses `noreferrer`.

Test the module, its integration, and existing UseKeyModal tests; run the standard
frontend build/type checks. Recheck the RikkaHub schema when changing app versions.
An actual Android import remains a device acceptance step.

`frontend/rikka-preview.html` and `vite.rikka-preview.config.ts` are a separate
visual preview build using a deliberately invalid demo key and mock catalog.
They are not entry points in the normal production build.

Initial integration testing used `custom-v0.2.7-1` (commit
`4275195aa1143c47ba9bde8c5418fd6906ddba9f`). The feature is integrated into the
normal `custom` branch history and does not change the backend or its schema.
