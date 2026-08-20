# Absence Form States Image Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use inline execution in this session; do not dispatch a sub-agent.

**Goal:** Capture all reachable public absence-form states from the real `/absence` route and compose them into one readable labeled PNG.

**Architecture:** A standalone Playwright script will create fresh browser contexts for deterministic state captures, using the existing absence fixture data and route overrides for loading, errors, and submission delay. The same script will create a temporary composition page from the captured PNG buffers and screenshot it into `artifacts/absence-form-states.png`, avoiding any new image-processing dependency.

**Tech Stack:** Node.js, Playwright Chromium, existing Vite preview server, HTML/CSS composition, PNG screenshots.

---

### Task 1: Add the deterministic absence-form state capture runner

**Files:**
- Create: `scripts/capture-absence-form-states.mjs`
- Reference: `e2e/fixtures/absence.ts`, `e2e/helpers/absenceFlow.ts`, `src/pages/AbsenceForm.tsx`

- [ ] **Step 1: Implement route fixtures and state helpers**

  Import `chromium` from `@playwright/test`, use the real `/absence` route, and add helpers for baseline routes, student lookup, verification, classes, review, offline, and submit states. Keep each state in a fresh context and wait for its semantic heading/status/alert before taking a viewport screenshot.

- [ ] **Step 2: Implement the 19 capture scenarios**

  Capture config loading; student idle, invalid, pending, CRM-email, and manual-email states; verification ready, sending, verify error, and expired states; classes loading, ready, missing make-up, and offline states; review ready and offline states; submitting; quota error; and success receipt.

- [ ] **Step 3: Implement labeled PNG composition**

  Build an in-memory HTML contact sheet with a navy title band, two-column card grid, state labels, and captured PNG data URLs. Screenshot the composition page to `artifacts/absence-form-states.png` and write `artifacts/absence-form-state-manifest.json` with the state labels and capture status.

### Task 2: Run the capture and validate the artifact

**Files:**
- Output: `artifacts/absence-form-states.png`
- Output: `artifacts/absence-form-state-manifest.json`

- [ ] **Step 1: Start the Vite preview server**

  Run `npm run build && npm run preview -- --host 127.0.0.1 --port 4173` and keep the server alive for the capture.

- [ ] **Step 2: Execute the capture runner**

  Run `node scripts/capture-absence-form-states.mjs` with `BASE_URL=http://127.0.0.1:4173`.

- [ ] **Step 3: Verify output metadata**

  Confirm the script exits 0, the PNG is non-empty, the manifest contains all 19 labels, and no scenario is marked unavailable unless the runtime makes that state impossible.

- [ ] **Step 4: Perform manual visual QA**

  Open the PNG, inspect the title band, labels, grid alignment, readable headings, alerts, action bars, and the success receipt. If any panel is clipped in a way that hides the state-defining content, adjust the composition CSS and recapture.

