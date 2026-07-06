# Company Brain
## Module Spec 10, UI/UX Design

**Status** Ready for build
**Consumed by** Module 07, all web app and Slack surfaces

---

## 10.1 Design Thesis

The product sells provable trust, so the interface must feel like an instrument, not an app. The reference points are mission control and a private bank, precision surfaces, calm density, information that earns its place. Dark mode is the only mode, not a preference toggle, because the product is ambient infrastructure that people glance at between real work, and a dark instrument panel reads as always-on in a way a white document page never will.

The single idea every screen expresses, truth has a receipt. Verified knowledge glows warm, unverified signal stays cool and recedes, and the path from any statement back to its human approver is always one gesture away.

One deliberate rejection. The default futuristic look, near-black with one acid-green accent, is what every AI dashboard ships. This product uses warmth as its signal instead, because the emotional job of the interface is trust, and trust is warm.

## 10.2 Color System

Base surfaces, a deep blue-black ramp, never pure black, pure black makes elevated panels impossible.

- void, #0A0D14, the app background
- surface, #10141D, cards and panels
- surface-raised, #161B27, modals, popovers, hovered rows
- line, #232A38, hairline borders and dividers
- line-strong, #303948, focused and active borders

Text.

- text-primary, #E8EAF0
- text-secondary, #9AA3B5
- text-muted, #5E687C, timestamps, metadata, never for content

The trust spectrum, the heart of the system. Every piece of knowledge on screen carries exactly one of these, and the colors are reserved, nothing else on the interface may use them.

- verified, #E8B04B, warm amber gold. Canon-verified content, approval seals, the receipt glyph. The only warm color in the interface, which is precisely why it means what it means.
- stale, #A08850, desaturated ochre. Decayed canon, served but flagged.
- stream, #6B87A8, cool steel blue. Stream-only content, unverified signal.
- conflict, #E5484D, signal red. Open contradictions, and nothing else, not errors, not destructive buttons, those get their own muted treatment, red is spent entirely on drift.
- none, #5E687C, no-coverage states.

Functional accents, used sparingly.

- action, #7B9EF8, a restrained periwinkle for primary interactive elements, links, and focus rings
- positive, #4CAF8E, success confirmations only

## 10.3 Typography

Three faces, three jobs.

- **Display, Archivo Expanded.** Page titles, domain headers, the big numbers on the drift dashboard. Wide, industrial, set tight, weights 600 and 800 only. This face carries the futuristic register so nothing else has to.
- **Body, Instrument Sans.** Everything readable, 400 and 500, 15px base on desktop, 1.6 line height. Quiet on purpose.
- **Data, JetBrains Mono.** Timestamps, ids, confidence values, version numbers, excerpts from source events, and the entire receipt component. The mono face is the visual marker for evidence, when the reader sees mono, they are looking at ground-level fact, not interface chrome.

Type scale, 13, 15, 17, 22, 28, 40. Eyebrow labels in 11px mono uppercase with 8 percent letter spacing, used for domain tags and trust labels.

## 10.4 The Signature, the Receipt

Every canon-backed answer, entry, and citation carries the receipt, a compact horizontal chain rendered in mono, seal glyph, approver, verified date, source count.

Collapsed it reads as a single quiet line under the statement. Expanded, one click or the R key, it unfolds into a vertical provenance chain, entry, version, approval, source events, each node a link, connected by a thin amber thread on the void background. The thread is drawn with a 300ms trace animation on expand, the one place the interface performs, because watching the line of trust draw itself from statement to source is the product's entire pitch in half a second.

The receipt is one component. The approval queue, canon browser, version history, serving previews, and cold-start review all render it. There is exactly one of it.

## 10.5 Layout System

- Fixed left rail, 240px, navigation by surface, canon, queue, drift, connectors, audit, admin. Collapsed rail at 64px shows glyphs only.
- Content area on an 8px spacing grid, max content width 1200px for reading surfaces, full-bleed for the dashboard.
- Density is a virtue. Rows at 44px, generous only where reading happens, entry statements and diffs get air, metadata stays compact.
- Elevation by surface color step plus a 1px line border, never drop shadows heavier than 0 4px 24px rgba(0,0,0,0.4). Glow is reserved for the trust spectrum, a verified seal may glow faintly, a button may not.
- Corner radius 6px on controls, 10px on cards, nothing fully rounded except status dots and avatars.

## 10.6 Motion

Motion explains state change, never decorates.

- Standard transitions 150ms ease-out, surface enter and exit 200ms with 4px translate.
- The receipt trace, 300ms, described above.
- The drift pulse, the one ambient element. On the dashboard, each domain header carries a thin baseline that pulses once, subtly, when new stream events arrive in that domain, a heartbeat of maybe 2 seconds, opacity 0.3 to 0.6. The brain is alive and the interface breathes exactly this much and no more.
- Approval actions resolve optimistically with a 150ms seal-stamp scale on the verified glyph.
- prefers-reduced-motion disables the trace, the pulse, and all translates, opacity-only fallbacks.

## 10.7 Surface-by-Surface UX

**Canon browser.** Left, domain list with per-domain health rings, a thin circular gauge in the trust spectrum colors. Center, entry list, each row shows statement, owner avatar, trust dot, verified date in mono. Right, the detail pane, statement large in body face, attributes as a mono key-value block, the receipt, the version timeline as a vertical thread of amber nodes, relations as chips. Search is a command palette, cmd-K, spanning canon first, stream results below a hard visual divider in cool blue, the two-layer architecture visible in the search results themselves.

**Approval queue.** The sacred surface, designed for velocity. One proposal at a time in a focused center column, kind and confidence in the eyebrow, the diff as the centerpiece, removed text struck in muted red-gray, added text in amber, attributes diff as a mono block. Source excerpts dock on the right, cool blue border coding them as stream. Keyboard-first, A approve, E edit, X reject with a reason picker, J and K to move. A session progress rail on the left edge fills as the owner clears the queue, gamified exactly this much and no more. Batch mode collapses to a table with multi-select.

**Drift dashboard.** The mission-control surface and the demo screen. Top band, three display-face numbers, open contradictions, pending proposals, canon health, each with a seven-day sparkline in mono-thin strokes. Center, the domain grid, each domain a card with its health ring, pulse baseline, and open-item count. Conflict cards surface below in signal red left-borders, statement versus stream claim side by side. This screen must look extraordinary in a sales demo at 1080p, it is the poster.

**Cold start review.** A guided full-screen flow, progress bar against the entry budget across the top, one drafted entry at a time using the same diff and receipt components, domain-batched with a domain intro screen showing what was inferred and from how many events. The final screen renders the founder's new canon as a constellation, entries as amber points grouped by domain on the void, the moment the company sees itself as a machine-readable thing for the first time. Screenshot-bait, deliberately.

**Connector management.** Status board, each connector a card with source glyph, state (live in positive, degraded in ochre with remediation steps inline, backfilling with a determinate progress bar), lag figure in mono. Exclusion rule editor as plain readable rules, no regex exposed unless the user opens advanced.

**Audit log.** Dense mono table, filter bar pinned, every row expandable to the full record. This surface is allowed to look like a terminal, that is its honesty.

**Empty, loading, error.** Empty states name the next action, an empty queue says the canon is current and shows the last cleared item. Loading is skeleton rows in surface-raised, never spinners on content surfaces. Errors state what happened and the remediation, in the interface's voice, no apologies, no mascots.

## 10.8 Slack Surface

Slack styling is not controllable, so the design system reduces to language and structure, proposal messages lead with the kind and the entry statement, the diff as a quote block, three buttons in the fixed order approve, edit, reject, and the receipt collapsed to one mono-style context line, approver, date, source count. Identical vocabulary to the web app, an action called Approve produces a confirmation that says Approved.

## 10.9 Copy Voice

Plain verbs, sentence case, no filler. Buttons say what happens, Approve entry, Reject with reason, Reconnect Slack. Trust labels are exact and consistent everywhere, Verified, Stale, Stream signal, No coverage, never synonyms. The interface never says AI thinks or AI found, it says detected in 12 events or contradicts pricing entry, evidence has counts and sources, not feelings. Numbers in mono, always.

## 10.10 Accessibility Floor

Contrast 4.5 to 1 minimum for text on all surfaces, the trust spectrum values are chosen to pass on void and surface. Trust is never color-alone, every trust state pairs its color with its glyph and label. Full keyboard traversal on every surface with visible focus rings in action blue, the approval queue is operable without a pointer entirely. Reduced motion honored per 10.6. Semantic landmarks and live-region announcements for queue advancement and toast confirmations.

## 10.11 Implementation Notes

Tokens ship as the Tailwind theme in apps/web, colors, radii, type scale, spacing, motion durations, named exactly as in this document. Components consume tokens only, a raw hex in a class string fails review. The receipt, trust badge, diff view, health ring, and pulse baseline are shared primitives in the component library before any surface is assembled from them. Fonts self-hosted, variable where available, display face subset to the weights used.
