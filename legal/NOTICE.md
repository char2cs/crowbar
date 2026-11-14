# Third-Party Notices

Crowbar itself is dual-licensed under [AGPL-3.0](LICENSE) or a commercial license
(see [LICENSING.md](LICENSING.md)). It redistributes the third-party components below,
each under its own license.

Runtime dependencies installed from package registries (npm, Go modules, crates.io)
are governed by the licenses shipped with those packages and are not restated here.
This file covers components whose files are checked into this repository or bundled
into the application by the build.

---

## Fonts

### Cal Sans — `app/panes/public/fonts/CalSans-Regular.woff2`, `app/panes/public/fonts/CalSansUI.woff2`

Copyright 2021 The Cal Sans Project Authors (https://github.com/calcom/font)

Licensed under the SIL Open Font License, Version 1.1.
Full license text: [`licenses/CalSans-OFL-1.1.txt`](licenses/CalSans-OFL-1.1.txt)

### Symbols Nerd Font Mono — `app/panes/public/fonts/SymbolsNerdFontMono-Regular.woff2`

Copyright (c) 2014 Ryan L McIntyre (https://github.com/ryanoasis/nerd-fonts)

Licensed under the MIT License. The font aggregates glyphs from upstream icon sets
that carry their own licenses; see the note in the license file.
Full license text: [`licenses/SymbolsNerdFont-MIT.txt`](licenses/SymbolsNerdFont-MIT.txt)

### Geist Mono — `app/desktop/crates/ui/assets/fonts/GeistMono-Variable.ttf`

Copyright (c) 2023 Vercel, in collaboration with basement.studio

Licensed under the SIL Open Font License, Version 1.1.
Full license text: [`licenses/Geist-OFL-1.1.txt`](licenses/Geist-OFL-1.1.txt)

### Instrument Serif — `app/desktop/crates/ui/assets/fonts/InstrumentSerif.ttf`

Copyright 2022 The Instrument Serif Project Authors (https://github.com/Instrument/instrument-serif)

Licensed under the SIL Open Font License, Version 1.1.
Full license text: [`licenses/InstrumentSerif-OFL-1.1.txt`](licenses/InstrumentSerif-OFL-1.1.txt)

### Cal Sans UI — `app/desktop/crates/ui/assets/fonts/CalSansUI.ttf`

TrueType build of the Cal Sans UI file listed above; same copyright and license
([`licenses/CalSans-OFL-1.1.txt`](licenses/CalSans-OFL-1.1.txt)).

## Icons

### Phosphor Icons — `app/desktop/crates/ui/assets/icons`

Copyright (c) 2023 Phosphor Icons (https://github.com/phosphor-icons/core)

Licensed under the MIT License. SVG files of the icons the shell uses, copied unchanged from
`@phosphor-icons/core` 2.1.1.
Full license text: [`licenses/Phosphor-MIT.txt`](licenses/Phosphor-MIT.txt)

## Go source

### charmbracelet/x/vt — `api/third_party/x-vt`

Copyright (c) 2023 Charmbracelet, Inc.

Licensed under the MIT License. Vendored from `github.com/charmbracelet/x/vt` at
`v0.0.0-20260629091435-9c70f75e26a4` (untagged commit `9c70f75e26a4`); the `replace`
in `api/go.mod` points the module at this directory.
The copy is locally patched; details in `api/internal/core/terminal/internal/model/UPSTREAM.md`.
Full license text: [`api/third_party/x-vt/LICENSE`](api/third_party/x-vt/LICENSE)

### xterm.js — `api/third_party/x-vt/reflow.go`, `app/panes/src/features/terminal/lib/terminal-links.ts`

Copyright (c) 2017-2019, The xterm.js authors (https://github.com/xtermjs/xterm.js)

Licensed under the MIT License. `reflow.go` ports the resize reflow of `@xterm/xterm` 6.0.0
(`src/common/buffer/Buffer.ts`, `BufferReflow.ts`) to Go. `terminal-links.ts` ports the URL
regex and URL check of `@xterm/addon-web-links` 0.12.0.
Full license text: [`licenses/xtermjs-MIT.txt`](licenses/xtermjs-MIT.txt)
