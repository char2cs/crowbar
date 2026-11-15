# Standards coverage

Every rule of `docs/rabbyte-manifest.md` against the mechanism that enforces it in each language. `tools/scripts/check-standards.sh` runs the script mechanisms (Rust `check-rust-rules.sh`, Swift `check-swift-rules.sh`, TypeScript `check-ts-rules.sh`), all of which fail the build on any finding; `--full` adds the formatter and linter column. Script rule ids are the `[tag]` in the output. "sample" means the rule cannot be decided by a machine; the sampled-review procedure is at the bottom. GAP means no mechanism yet.

Rust: rustfmt (`app/rustfmt.toml`), clippy (`app/Cargo.toml` lint table, `app/clippy.toml`), `check-rust-rules.sh`, cargo-deny, cargo-machete. Swift: swift-format, SwiftLint (`app/mobile/ios/.swiftlint.yml`), `check-swift-rules.sh`. TypeScript: eslint, tsc, prettier, knip, `check-ts-rules.sh`.

| # | Rust | Swift | TypeScript |
|---|------|-------|------------|
| 1 params one per line | rustfmt `fn_params_layout = "Vertical"`; script `param-layout`, clippy `too_many_arguments` 4 | SwiftLint `multiline_parameters`, `function_parameter_count` 4; script `param-layout`, `param-count` | script `param-count`, `param-layout` (a wrapped list is one per line); see "TS rule 1 decision" below |
| 2 depth max 2 | script `depth`; clippy `excessive_nesting` 2, `cognitive_complexity` | script `depth`; SwiftLint `nesting` function_level 2 | script `depth` |
| 3 guards, no else | script `else`; clippy `redundant_else`, `question_mark` | script `else` (`guard ... else` allowed) | script `else`; eslint `no-else-return` |
| 4 named returns | script `named-return` (accumulator analog; Rust has no naked returns); clippy `let_and_return` | script `named-return` | script `named-return` |
| 5 named conditions | script `long-condition`; clippy `nonminimal_bool` | script `long-condition` | script `long-condition` |
| 6 stdlib over loops | clippy `needless_range_loop`, `manual_*` | script `hand-loop`; SwiftLint `reduce_into`, `contains_over_filter_count` | script `hand-loop` |
| 7 comments | script `comment`, `doc-private`; clippy `missing_docs`, `missing_errors_doc` | script `comment`, `doc-private`, `doc-missing`; SwiftLint `no_inline_comment`, `missing_docs` | script `comment`, `doc-missing`, `doc-private`, `todo` |
| 8 one type per file | script `multi-type`, `type-name` | script `multi-type`, `type-name`; SwiftLint `one_declaration_per_file`, `file_name` | script `multi-concept`, `type-name`, `file-name` |
| 9 error with its struct | script `type-name` (`Error` in `error.rs`) | script `type-name` | script `type-name` |
| 10 entry in file named after module | script `lib-decl`, `entry-file` (only crates whose lib.rs re-exports a free function; an entry type lives in its own file by rule 8) | one type per file named after it (8) | sample |
| 11 stateless type is a function | script `stateless-type` | sample | script `new-stateless` (empty constructor); sample |
| 12 `new` only with state | script `new-stateless`; clippy `new_without_default` | script `new-stateless` | script `new-stateless` |
| 13 public API exposes interfaces | clippy `unreachable_pub`; sample | SwiftLint `missing_docs`; sample | knip (unused exports); sample |
| 14 root exposes only API | script `lib-decl`; clippy `unreachable_pub` | sample | knip |
| 15 rest is internal | `pub(crate)`/private `mod`; clippy `unreachable_pub` | `internal`/`private` default; SwiftLint `private_over_fileprivate` | knip; script `barrel` |
| 16 siblings use public API only | script `cargo-layer`; cargo | SwiftPM targets | script `layer` (lib/sdk vs components/features, stores vs components); eslint `no-restricted-imports` |
| 17 one-way dependencies | script `cargo-layer`; cargo-deny | SwiftPM | script `layer` |
| 18-19 `internal/` | n/a: replaced by 15 | n/a | n/a |
| 20 no cycles | cargo (crates); script `cycle` (`check-cycles.py`, modules inside a crate) | SwiftPM (targets) | script `cycle` (`check-cycles.py ts`, import graph, changed files) |
| 21 names | clippy `module_name_repetitions` | script `type-name`, SwiftLint `type_name` | script `file-name` |
| 22 banned names | script `module-name` | script `module-name` | script `module-name` |
| 23 no stutter | clippy `module_name_repetitions` | sample | sample |
| 24 no globals/init magic | script `global`; clippy `static_mut_refs`; deny bans | script `global`; SwiftLint `no_global_mutable_state` | script `global` |
| 25 cancellation/context | clippy `await_holding_lock`, `large_futures`; sample | sample | eslint `no-floating-promises`; sample |
| 26 errors are values | clippy `let_underscore_must_use`, `unused_must_use`, `map_err_ignore` | SwiftLint `force_try`, `unused_result`-style compiler warnings | eslint `no-empty`, `SWALLOWED_FAILURE_RULES`, `no-floating-promises` |
| 27 no panic | script `panic`; clippy `unwrap_used`, `expect_used`, `panic`, `todo` | script `panic`; SwiftLint `force_unwrapping`, `fatal_error_message` | script `non-null`, `any`, `suppress`; eslint `ban-ts-comment`, `no-explicit-any` |
| 28 small consumer-side traits | sample | sample | sample |
| 29 names reveal intent | clippy `disallowed_names`; sample | SwiftLint `identifier_name`; sample | eslint `id-length`; sample |
| 30 small functions | script `fn-length`; clippy `too_many_lines` 40 | script `fn-length`; SwiftLint `function_body_length` | script `fn-length` |
| 31 single responsibility | script `multi-type`; sample | script `multi-type`; SwiftLint `type_body_length`; sample | script `multi-concept`; sample |
| 32 no magic values | script `magic-number`; magic strings: sample | script `magic-number`; SwiftLint `no_magic_numbers`; magic strings: sample | script `magic-number`; magic strings: sample |
| 33 no bool params | script `bool-param`; clippy `fn_params_excessive_bools` | script `bool-param`; SwiftLint `no_bool_parameter` | script `bool-param` |
| 34 command-query separation | sample | sample | sample |
| 35 no hidden side effects | sample | sample | sample |
| 36 no duplication | sample (jscpd report in review) | sample | sample |
| 37 dead code | clippy `dead_code`, `unused`; script `allow-dead`; cargo-machete | SwiftLint `unused_declaration`, `unused_import` | knip; eslint `no-unused-vars`; script `comment` (commented-out code is a comment) |
| 38 injected dependencies | script `global`; sample | script `global`; sample | script `global`; sample |
| 39 table-driven tests | script `table-driven` | script `table-driven` | script `table-driven` |
| 40 test the interface | sample | sample | script `test-style`; sample |
| 41 no real infrastructure | script `real-infra`; clippy `disallowed_methods` | script `real-infra` | script `real-infra`, `sleep` |
| 42 1:1 tests | script `test-missing` (inline `mod tests`) | script `test-missing` (`<Source>Tests.swift`) | script `test-missing`, `test-orphan`, `test-location` |
| 43 no free test files | script `test-sibling`, `test-in-src`, `cfg-test`, `test-orphan` | script `test-orphan` | script `test-orphan`, `test-scenario`, `test-location` |
| 44 size 500 | script `size` (non-test lines) | script `size`; SwiftLint `file_length` | script `size` |
| 45 split by responsibility | sample | sample | sample |

## Sampled review (rules that are not mechanizable)

Rules marked sample: 10-11, 13, 23, 25, 28, 29, 31, 32 (strings), 34-36, 38, 40, 45. Procedure: every cycle the standards agent reads at least five changed files per language chosen by `git diff --stat` size, plus every new type, and checks each sample rule; findings go to the owner as `file:line, rule number, required change`. The reviewer agent repeats this per crate against `docs/plans/2026-11-14-code-review-checklist.md` before the merge gate.

## TS rule 1 decision

Rule 1 asks for one parameter per line, always. In TypeScript the formatter is prettier, which re-joins any parameter list that fits in `printWidth`, so an always-vertical rule cannot be satisfied and `prettier --check` green at the same time. The alternatives were tried on paper and rejected: `@stylistic/function-paren-newline` with `minItems: 2` also hits every call site and runs against prettier on 1,900 legacy files (ESLint 10 removed the core rule); replacing prettier for the changed scope needs a full stylistic formatter preset and a per-file scope list, which is a baseline that grows. Decision: mechanically impossible with prettier, so the enforced form is the strongest one prettier allows. A parameter list that wraps is one per line (prettier native), `[param-count]` caps a function at 4 parameters so lists stay short, and `[param-layout]` rejects a multi-line list that packs two parameters on a line. Anyone who wants always-vertical must first drop prettier for TypeScript; that is an owner decision about tooling, not a review judgement.

## Cycle check

`tools/scripts/check-cycles.py` finds import cycles (Tarjan strongly connected components). Rust: one node per module file of a crate, edge from `crate::<module>` paths outside the test module. TypeScript: one node per file, edges from static imports (`@/` alias and relative), reported only when the cycle contains a changed file. A module cycle is fixed by extracting the shared contract into a third, lower module.

## Clippy `excessive_nesting`

Verified on a scratch crate: clippy counts the function body as depth 1, so `excessive-nesting-threshold = 2` rejects an `if` inside a `for` (manifest level 2, allowed) and the manifest's level-3 block is rejected only at threshold 3. `app/clippy.toml` therefore stays at 3, a backstop that fires on blocks nested inside a level-3 block. The awk `depth` rule is the authority for the manifest definition (function body is level 1, a block inside it level 2, a third level is rejected).

## Open gaps

- Rule 1 (TS) always-vertical: see the decision above.
