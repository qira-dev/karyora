# karyora-ui — the Unified AI Work Platform shared UI shell

Placement: this is the `ui/` folder baseline of the UI Master Blueprint
§31.6, implemented by UI Phase 2 milestone **P2.6** (shared shell and
approved navigation) against the frozen P2.1 surface contracts.

## What is here (P2.6)

- The shared application shell: navigation rail (expanded/collapsed with
  persistence as a local per-user/device preference), top bar, page-title
  focus routing, search entry, `+ New` menu, notification entry,
  connection indicator/popover, session menu, shortcut help.
- The frozen Phase 2 route set only: `/` and the ten accepted settings
  routes. Feature routes (Overview, Chat, Jobs, Knowledge, Memory,
  Context, Workflows, Providers, Plugins, Activity) are later packages
  (UIR-4–UIR-8) and are deliberately ABSENT — no stub routes.
- Semantic design tokens (P2.2): the Blueprint's proposed Light and Dark
  values verbatim in `src/styles/tokens.css`; CSS Modules consumption
  only (UI-07); CSS logical properties for direction-aware layout (G-10).
- Localization (G-10, UI-18 LOCKED): i18next + react-i18next with
  per-locale JSON resources and explicit `fallbackLng: "en"`. English is
  the only resource set that exists — no translation content is
  fabricated. `npm run i18n:parity` enforces key parity for any locale
  file added later.
- CoreClient contract layer (`src/coreclient/`): the typed boundary and
  the D-35 error envelope per P1.5. No transport is wired; the truthful
  `unavailable` state is what the shell renders. Exact wire names are
  Open (P2.1 O-1) and none is invented.

## Icon family (O-5 build-time selection, recorded)

One family: **lucide-react** (ISC license; Apache-2.0-compatible;
MIT/ISC-class notice obligations tracked in the PP-2 proof). The §4.1
concept map is family-independent; the concrete glyphs used and their
substitutions:

| Route / surface | §4.1 concept | lucide glyph |
| --- | --- | --- |
| Overview | Dashboard grid | `layout-dashboard` |
| Chat | Message bubble | `message-circle` |
| Jobs | Checklist with time/state cue | `list-todo` |
| Knowledge | Open book / document library | `book-open` |
| Memory | Bookmark | `bookmark` |
| Context | Filter / selected-source set | `filter` |
| Workflows | Connected steps / branching flow | `workflow` |
| Providers & Models | Connected nodes / network | `network` |
| Plugins (EXTENSIONS) | Puzzle piece | `puzzle` |
| Activity & Audit | Activity trace / history | `history` |
| Settings | Sliders or gear | `sliders-horizontal` |
| Help & Documentation | Question mark / help | `circle-help` |
| Connection status (Unknown) | State + safe label | `circle-dashed` |
| Session menu | Session-menu treatment | `circle-user` |

Withheld items (Work / Projects; Team & Access) have NO glyph here: their
routes are withheld (UI-10/UI-23) and their icons do not render.

## Scripts

- `npm run build` — typecheck + production build (the PP-4 build).
- `npm run preview` — serve the production build locally.
- `npm run i18n:parity` — locale key-parity check (G-10).
- `npm run types:generate` / `npm run types:check` — regenerate and
  drift-check the generated schema types against `contracts/openapi.yaml`
  (the non-canonical `0.1.0-eval` evaluation surface; consumed by
  nothing; see `src/coreclient/generated-eval-surface/README.md`).

## Honest status

No accessibility conformance, performance result, desktop support, or
usability claim is made by this codebase. Rendered verification of
contrast/focus/keyboard/reflow (UI-25, G-05/G-06 evidence) is future,
not-yet-run work, recorded in the P2.6 vault record.
