# Vendored icon artwork

Extracted **once**, committed, and never regenerated at build time.

`native/` must not reach into `web/node_modules` — check-invariants rule 7 —
and a generator that did would break on the day `web/` is deleted, which is the
whole reason that rule exists. These are assets, like the typefaces S0.7
vendored into `native/assets/fonts/` and for the same reason.

## Why these files exist at all

Until S1a **nothing in `crowbar-ui` drew an icon.** Every icon surface rendered
a correctly-sized, correctly-coloured, entirely empty box — 23 of them in the
sidebar alone. The retired parity oracle could not see it: its contract
compares `bounds`, `bg`, `radius`, `border` and text, and an empty box of the
right size matches a reference containing an SVG on every one of those fields.

## Where each file came from

Each is the exact artwork the React app renders at the same call site, so the
two apps draw the same shape and not merely the same box.

| file | upstream | licence |
|---|---|---|
| `arrow-left.svg` | `lucide-react` `arrow-left` | ISC |
| `arrow-right.svg` | `lucide-react` `arrow-right` | ISC |
| `settings.svg` | `lucide-react` `settings` | ISC |
| `library.svg` | `lucide-react` `library` | ISC |
| `folder-symlink.svg` | `lucide-react` `folder-symlink` | ISC |
| `layout-grid.svg` | `lucide-react` `layout-grid` | ISC |
| `download-cloud.svg` | `lucide-react` `cloud-download` (the package re-exports it as `DownloadCloud`, which is the name the app imports) | ISC |
| `git-branch.svg` | `@phosphor-icons/react` `GitBranch`, **regular** weight | MIT |
| `git-fork.svg` | `@phosphor-icons/react` `GitFork`, regular | MIT |
| `git-merge.svg` | `@phosphor-icons/react` `GitMerge`, regular | MIT |
| `git-pull-request.svg` | `@phosphor-icons/react` `GitPullRequest`, regular | MIT |
| `lock.svg` | `@phosphor-icons/react` `Lock`, regular | MIT |
| `warning.svg` | `@phosphor-icons/react` `Warning`, regular | MIT |
| `squares-four.svg` | `@phosphor-icons/react` `SquaresFour`, regular | MIT |
| `chats-circle.svg` | `@phosphor-icons/react` `ChatsCircle`, regular | MIT |
| `folder-open.svg` | `@phosphor-icons/react` `FolderOpen`, regular | MIT |
| `row-add.svg` | **not an import** — `web/src/components/layout/workspace-row-base.ts`'s `ADD_GLYPH_PATH`, an inline path | this repo |
| `row-chevron.svg` | **not an import** — the inline disclosure path in `repo-section.tsx` / `workspace-tree-item.tsx` | this repo |

`regular` is Phosphor's default weight, which is what `<Lock />` with no
`weight` prop renders — the shape the app actually shows.

## Licences

* **Lucide** — ISC, © Lucide Icons and Contributors. Lucide is itself a fork of
  Feather Icons (MIT, © Cole Bemis).
* **Phosphor Icons** — MIT, © 2020 Phosphor Icons.

Both are permissive and require the copyright notice to travel with the
artwork, which is what this file is.

## Shape of the files

Normalised on the way in so the renderer needs no per-icon special cases:

* every file paints in `currentColor`, so `Icon::render`'s one `text_color`
  call themes all of them — the same mechanism the React app's `text-*`
  classes use;
* lucide keeps its `viewBox="0 0 24 24"` and stroke attributes, Phosphor its
  `viewBox="0 0 256 256"` fill, because rescaling either would change the
  optical weight the reference draws;
* the two inline glyphs keep the app's own `viewBox="0 0 16 16"` and
  `stroke-width="1.5"`, which `workspace-row-base.ts` documents as deliberate.
