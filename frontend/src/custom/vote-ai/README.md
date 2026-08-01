# Vote AI frontend customization boundary

This directory owns the Vote AI-specific public frontend. Official Sub2API files should only contain small integration points marked with one of these searchable tokens:

- `CUSTOM(VOTE-AI-HOME)`
- `CUSTOM(VOTE-AI-PRICING)`
- `CUSTOM(VOTE-AI-DOCS)`
- `CUSTOM(VOTE-AI-THEME)`
- `CUSTOM(VOTE-AI-BUILD)`

Owned code:

- `views/VoteAiHome.vue`: branded default homepage;
- `views/PricingView.vue`: static public pricing page;
- `views/DocsView.vue`: public and administrator documentation UI;
- `components/InteractiveGlobe.vue`: branded globe visualization;
- `components/MarkdownContent.vue`: sanitized documentation renderer;
- `api/docs.ts`: managed-document API client;
- `pricing-data.ts`: static model prices and group multipliers;
- `__tests__/`: focused customization tests.

Official integration points:

- `src/views/HomeView.vue`: selects custom content, compact mode, or `VoteAiHome`;
- `src/router/index.ts`: registers `/pricing` and `/docs/:slug?`;
- `tailwind.config.js`, `postcss.config.js`, `vite.config.ts`, and `src/style.css`: branded theme/build hooks.

Run focused frontend checks with:

```powershell
corepack pnpm test:custom
```

After every upstream merge, search all integration points with:

```powershell
rg "CUSTOM\(VOTE-AI" frontend backend
```
