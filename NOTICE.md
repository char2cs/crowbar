# Third-Party Notices

Crowbar itself is licensed under [AGPL-3.0-only](LICENSE) (see [LICENSING.md](LICENSING.md)).
It redistributes the third-party components below, each under its own license.

Runtime dependencies installed from package registries (npm, Go modules, crates.io)
are governed by the licenses shipped with those packages and are not restated here.
This file covers components whose files are checked into this repository or bundled
into the application by the build.

---

## Fonts

### Cal Sans — `web/public/fonts/CalSans-Regular.woff2`, `web/public/fonts/CalSansUI.woff2`

Copyright 2021 The Cal Sans Project Authors (https://github.com/calcom/font)

Licensed under the SIL Open Font License, Version 1.1.
Full license text: [`licenses/CalSans-OFL-1.1.txt`](licenses/CalSans-OFL-1.1.txt)

### Symbols Nerd Font Mono — `web/public/fonts/SymbolsNerdFontMono-Regular.woff2`

Copyright (c) 2014 Ryan L McIntyre (https://github.com/ryanoasis/nerd-fonts)

Licensed under the MIT License. The font aggregates glyphs from upstream icon sets
that carry their own licenses; see the note in the license file.
Full license text: [`licenses/SymbolsNerdFont-MIT.txt`](licenses/SymbolsNerdFont-MIT.txt)

---

## Tree-sitter grammars

The syntax-highlighting grammars and highlight queries under
`web/public/tree-sitter/parsers/` are compiled from the upstream grammar
repositories below and committed to this repository so builds are reproducible
without network access. The exact upstream commit each grammar was built from is
recorded in [`web/scripts/tree-sitter-grammars.json`](web/scripts/tree-sitter-grammars.json)
and can be regenerated with `bun run refresh:tree-sitter`.

Full license texts: [`licenses/tree-sitter-grammars-MIT.txt`](licenses/tree-sitter-grammars-MIT.txt),
[`licenses/tree-sitter-grammars-Apache-2.0.txt`](licenses/tree-sitter-grammars-Apache-2.0.txt)

| Parser folder(s) | Upstream repository | License | Copyright |
| --- | --- | --- | --- |
| `sql` | [DerekStride/tree-sitter-sql](https://github.com/DerekStride/tree-sitter-sql) | MIT | © 2021 Derek Stride |
| `solidity` | [JoranHonig/tree-sitter-solidity](https://github.com/JoranHonig/tree-sitter-solidity) | MIT | © 2020 Joran Honig |
| `dart` | [UserNobody14/tree-sitter-dart](https://github.com/UserNobody14/tree-sitter-dart) | MIT | © 2020-2023 UserNobody14 and others |
| `elisp` | [Wilfred/tree-sitter-elisp](https://github.com/Wilfred/tree-sitter-elisp) | MIT | © 2021 Wilfred Hughes |
| `swift` | [alex-pinkus/tree-sitter-swift](https://github.com/alex-pinkus/tree-sitter-swift) | MIT | © 2021 alex-pinkus |
| `graphql` | [bkegley/tree-sitter-graphql](https://github.com/bkegley/tree-sitter-graphql) | MIT | © 2021 bkegley |
| `dockerfile` | [camdencheek/tree-sitter-dockerfile](https://github.com/camdencheek/tree-sitter-dockerfile) | MIT | © 2021 Camden Cheek |
| `protobuf` | [coder3101/tree-sitter-proto](https://github.com/coder3101/tree-sitter-proto) | MIT | © 2024-2025 Mohammad Ashar Khan |
| `elixir` | [elixir-lang/tree-sitter-elixir](https://github.com/elixir-lang/tree-sitter-elixir) | Apache-2.0 | © the project authors |
| `elm` | [elm-tooling/tree-sitter-elm](https://github.com/elm-tooling/tree-sitter-elm) | MIT | © 2018 Kolja Lampe |
| `kotlin` | [fwcd/tree-sitter-kotlin](https://github.com/fwcd/tree-sitter-kotlin) | MIT | © 2019 fwcd |
| `nix` | [nix-community/tree-sitter-nix](https://github.com/nix-community/tree-sitter-nix) | MIT | © 2019 Charles Strahan |
| `diff` | [the-mikedavis/tree-sitter-diff](https://github.com/the-mikedavis/tree-sitter-diff) | MIT | © 2021 Michael Davis |
| `terraform` | [tree-sitter-grammars/tree-sitter-hcl](https://github.com/tree-sitter-grammars/tree-sitter-hcl) | Apache-2.0 | © the project authors |
| `lua` | [tree-sitter-grammars/tree-sitter-lua](https://github.com/tree-sitter-grammars/tree-sitter-lua) | MIT | © 2021 Munif Tanjim |
| `markdown` | [tree-sitter-grammars/tree-sitter-markdown](https://github.com/tree-sitter-grammars/tree-sitter-markdown) | MIT | © 2021 Matthias Deiml |
| `svelte` | [tree-sitter-grammars/tree-sitter-svelte](https://github.com/tree-sitter-grammars/tree-sitter-svelte) | MIT | © 2024 Amaan Qureshi <amaanq12@gmail.com> |
| `toml` | [tree-sitter-grammars/tree-sitter-toml](https://github.com/tree-sitter-grammars/tree-sitter-toml) | MIT | © Ika <ikatyang@gmail.com> (https://github.com/ikatyang) |
| `vue` | [tree-sitter-grammars/tree-sitter-vue](https://github.com/tree-sitter-grammars/tree-sitter-vue) | MIT | © 2024 Amaan Qureshi <amaanq12@gmail.com> |
| `xml` | [tree-sitter-grammars/tree-sitter-xml](https://github.com/tree-sitter-grammars/tree-sitter-xml) | MIT | © 2023 ObserverOfTime |
| `yaml` | [tree-sitter-grammars/tree-sitter-yaml](https://github.com/tree-sitter-grammars/tree-sitter-yaml) | MIT | © 2024 tree-sitter-grammars contributors |
| `zig` | [tree-sitter-grammars/tree-sitter-zig](https://github.com/tree-sitter-grammars/tree-sitter-zig) | MIT | © 2024 Amaan Qureshi <amaanq12@gmail.com> |
| `bash` | [tree-sitter/tree-sitter-bash](https://github.com/tree-sitter/tree-sitter-bash) | MIT | © 2017 Max Brunsfeld |
| `c` | [tree-sitter/tree-sitter-c](https://github.com/tree-sitter/tree-sitter-c) | MIT | © 2014 Max Brunsfeld |
| `c_sharp` | [tree-sitter/tree-sitter-c-sharp](https://github.com/tree-sitter/tree-sitter-c-sharp) | MIT | © 2014-2023 Max Brunsfeld, Damien Guard, Amaan Qureshi, and contributors |
| `cpp` | [tree-sitter/tree-sitter-cpp](https://github.com/tree-sitter/tree-sitter-cpp) | MIT | © 2014 Max Brunsfeld |
| `css` | [tree-sitter/tree-sitter-css](https://github.com/tree-sitter/tree-sitter-css) | MIT | © 2018 Max Brunsfeld |
| `go` | [tree-sitter/tree-sitter-go](https://github.com/tree-sitter/tree-sitter-go) | MIT | © 2014 Max Brunsfeld |
| `html` | [tree-sitter/tree-sitter-html](https://github.com/tree-sitter/tree-sitter-html) | MIT | © 2014 Max Brunsfeld |
| `java` | [tree-sitter/tree-sitter-java](https://github.com/tree-sitter/tree-sitter-java) | MIT | © 2017 Ayman Nadeem |
| `json` | [tree-sitter/tree-sitter-json](https://github.com/tree-sitter/tree-sitter-json) | MIT | © 2014 Max Brunsfeld |
| `ocaml` | [tree-sitter/tree-sitter-ocaml](https://github.com/tree-sitter/tree-sitter-ocaml) | MIT | © 2020 Max Brunsfeld and Pieter Goetschalckx |
| `php` | [tree-sitter/tree-sitter-php](https://github.com/tree-sitter/tree-sitter-php) | MIT | © 2017 Josh Vera, GitHub |
| `python` | [tree-sitter/tree-sitter-python](https://github.com/tree-sitter/tree-sitter-python) | MIT | © 2016 Max Brunsfeld |
| `ql` | [tree-sitter/tree-sitter-ql](https://github.com/tree-sitter/tree-sitter-ql) | MIT | © 2019-2024 GitHub, Inc |
| `ruby` | [tree-sitter/tree-sitter-ruby](https://github.com/tree-sitter/tree-sitter-ruby) | MIT | © 2016 Rob Rix |
| `rust` | [tree-sitter/tree-sitter-rust](https://github.com/tree-sitter/tree-sitter-rust) | MIT | © 2017 Maxim Sokolov |
| `scala` | [tree-sitter/tree-sitter-scala](https://github.com/tree-sitter/tree-sitter-scala) | MIT | © 2018 Max Brunsfeld and GitHub |
| `tsx`, `typescript` | [tree-sitter/tree-sitter-typescript](https://github.com/tree-sitter/tree-sitter-typescript) | MIT | © 2017 Max Brunsfeld |

### Highlight queries from other sources

Most highlight queries come from the same commit as their grammar. These do not:

| Parser folder | Query source | License |
| --- | --- | --- |
| `cpp` | [tree-sitter/tree-sitter-c](https://github.com/tree-sitter/tree-sitter-c) (`queries/highlights.scm`, composed as a base layer) | MIT |
| `json` | [nvim-treesitter/nvim-treesitter](https://github.com/nvim-treesitter/nvim-treesitter) | Apache-2.0 |
| `markdown` | [nvim-treesitter/nvim-treesitter](https://github.com/nvim-treesitter/nvim-treesitter) | Apache-2.0 |
| `terraform` | [nvim-treesitter/nvim-treesitter](https://github.com/nvim-treesitter/nvim-treesitter) | Apache-2.0 |
| `tsx` | [nvim-treesitter/nvim-treesitter](https://github.com/nvim-treesitter/nvim-treesitter) | Apache-2.0 |
| `typescript` | [tree-sitter/tree-sitter-javascript](https://github.com/tree-sitter/tree-sitter-javascript) (`queries/highlights.scm`, composed as a base layer) | MIT |
