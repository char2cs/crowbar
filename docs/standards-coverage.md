# Standards coverage

Every rule of `docs/rabbyte-manifest.md` against the mechanism that enforces it in each language. `tools/scripts/check-standards.sh` runs the script mechanisms (Rust `check-rust-rules.sh`, Swift `check-swift-rules.sh`, TypeScript `check-ts-rules.sh`), all of which fail the build on any finding that is not within its committed baseline (see "Baselines"); `--full` adds the formatter and linter column, `--strict` ignores the baselines. CI (`rust.yml`, job `standards`) runs them on pushes to `develop` and `migration/rust-shell` and on draft and ready pull requests, with `--mobile` and `app/mobile/src-tauri` in the Rust scan roots. Script rule ids are the `[tag]` in the output. "sample" means the rule cannot be decided by a machine; the sampled-review procedure is at the bottom. GAP means no mechanism yet.

Rust: rustfmt (`app/rustfmt.toml`), clippy (`app/Cargo.toml` lint table, `app/clippy.toml`), `check-rust-rules.sh`, cargo-deny, cargo-machete. Swift: swift-format, SwiftLint (`app/mobile/ios/.swiftlint.yml`), `check-swift-rules.sh`. TypeScript: eslint, tsc, prettier, knip, `check-ts-rules.sh`.

| # | Rust | Swift | TypeScript |
|---|------|-------|------------|
| 1 params one per line | rustfmt `fn_params_layout = "Vertical"`; script `param-layout`, `param-count` (every `fn` form: `pub`, `pub(crate)`, `pub(super)`, `pub(in ..)`, generics, bounds with `Fn(..)`, where clauses; a tuple or destructuring pattern counts each bound name, so `(a, b): (A, B)` is 2); clippy `too_many_arguments` 4 | SwiftLint `multiline_parameters`, `function_parameter_count` 4; script `param-layout`, `param-count` | script `param-count`, `param-layout` (a wrapped list is one per line); see "TS rule 1 decision" below |
| 2 depth max 2 | script `depth`; clippy `excessive_nesting` 2, `cognitive_complexity` | script `depth`; SwiftLint `nesting` function_level 2 | script `depth` |
| 3 guards, no else | script `else`; clippy `redundant_else`, `question_mark` | script `else` (`guard ... else` allowed) | script `else`; eslint `no-else-return` |
| 4 named returns | script `named-return`: an accumulator named `result/ret/res/out/output/acc/accum/retval`, or any `let mut`/`var` initialised empty (`Vec::new()`, `String::new()`, `0`, `[]`, `{}`, ..) that is the tail expression of its function (`x`, `Ok(x)`, `Some(x)`, `return x`); clippy `let_and_return` | script `named-return` (same rule) | script `named-return` (same rule, `let`/`const` initialised `[]`, `{}`, `0`, `new Map()`) |
| 5 named conditions | script `long-condition`; clippy `nonminimal_bool` | script `long-condition` | script `long-condition` |
| 6 stdlib over loops | clippy `needless_range_loop`, `manual_*` | script `hand-loop`; SwiftLint `reduce_into`, `contains_over_filter_count` | script `hand-loop` |
| 7 comments | script `comment`, `doc-private`; `spec-ref` (`SPEC:` must start with an existing repo path, with a valid `#anchor` if given, or a URL), `platform-ref` (`PLATFORM:` = a known platform name + a reason of 3+ words), `tag-reason` (`SAFETY:`/`DEPRECATED:` need a 3+ word reason); clippy `missing_docs`, `missing_errors_doc` | script `comment`, `doc-private`, `doc-missing`, `spec-ref`, `platform-ref`, `tag-reason`; SwiftLint `no_inline_comment`, `missing_docs` | script `comment`, `doc-missing`, `doc-private`, `todo`, `spec-ref`, `platform-ref`, `tag-reason` (line comments only; interface members and class methods are not doc-checked: review) |
| 8 one type per file | script `multi-type`, `type-name` | script `multi-type`, `type-name`; SwiftLint `one_declaration_per_file`, `file_name` | script `multi-concept`, `type-name`, `file-name` |
| 9 error with its struct | script `type-name` (`Error` in `error.rs`) | script `type-name` | script `type-name` |
| 10 entry in file named after module | script `lib-decl`, `entry-file` (only crates whose lib.rs re-exports a free function; an entry type lives in its own file by rule 8) | one type per file named after it (8) | sample |
| 11 stateless type is a function | script `stateless-type` | sample | script `new-stateless` (empty constructor); sample |
| 12 `new` only with state | script `new-stateless`; clippy `new_without_default` | script `new-stateless` | script `new-stateless` |
| 13 public API exposes interfaces | clippy `unreachable_pub`; sample | SwiftLint `missing_docs`; sample | knip (unused exports); sample |
| 14 root exposes only API | script `lib-decl`; clippy `unreachable_pub` | sample | knip |
| 15 rest is internal | `pub(crate)`/private `mod`; clippy `unreachable_pub` | `internal`/`private` default; SwiftLint `private_over_fileprivate` | knip; script `barrel` |
| 16 siblings use public API only | script `cargo-layer`; cargo | SwiftPM targets | script `layer`: the import is resolved to its target (`@/` alias, relative `./`/`../`, single or double quotes, `import()`/`require()`), then lib/sdk/utils never reach components/features/routes and stores never reach components; eslint `no-restricted-imports` |
| 17 one-way dependencies | script `cargo-layer`; cargo-deny | SwiftPM | script `layer` (as 16) |
| 18-19 `internal/` | n/a: replaced by 15 | n/a | n/a |
| 20 no cycles | cargo (crates); script `cycle` (`check-cycles.py rust`: every module file of a crate including nested directories; edges from `use` trees, groups and `crate::`/`self::`/`super::` paths outside the test module; a `pub use` of an own child is structural, not an edge) | SwiftPM (targets) | script `cycle` (`check-cycles.py ts`, import graph, only cycles containing a changed file: the 3 legacy cycles show with `--all`) |
| 21 names | clippy `module_name_repetitions` | script `type-name`, SwiftLint `type_name` | script `file-name` |
| 22 banned names | script `module-name` | script `module-name` | script `module-name` |
| 23 no stutter | clippy `module_name_repetitions` | sample | sample |
| 24 no globals/init magic | script `global`; clippy `static_mut_refs`; deny bans | script `global`; SwiftLint `no_global_mutable_state` | script `global` |
| 25 cancellation/context | clippy `await_holding_lock`, `large_futures`; sample | sample | eslint `no-floating-promises`; sample |
| 26 errors are values | script `result-string` (`Result<_, String>`/`&str`), `map-err-ignore` (`map_err(\|_\| ..)`), `swallowed-error` (`Err(_) =>`, `if let Err(_)`, `.ok();`); clippy `let_underscore_must_use`, `unused_must_use`, `map_err_ignore` | SwiftLint `force_try`, `unused_result`-style compiler warnings | eslint `no-empty`, `SWALLOWED_FAILURE_RULES`, `no-floating-promises` |
| 27 no panic | script `panic` (`unwrap`, `expect`, `panic!`, `todo!`, `unimplemented!`, `unreachable!`, `dbg!` outside tests/main); clippy `unwrap_used`, `expect_used`, `panic`, `todo` | script `panic`; SwiftLint `force_unwrapping`, `fatal_error_message` | script `non-null`, `any`, `suppress`; eslint `ban-ts-comment`, `no-explicit-any` |
| 28 small consumer-side traits | script `trait-size` (a trait with more than 3 methods); whether a trait is consumer-side: sample | sample | sample |
| 29 names reveal intent | clippy `disallowed_names`; sample | SwiftLint `identifier_name`; sample | eslint `id-length`; sample |
| 30 small functions | script `fn-length`; clippy `too_many_lines` 40 | script `fn-length`; SwiftLint `function_body_length` | script `fn-length` |
| 31 single responsibility | script `multi-type`, `fn-count` (more than 12 functions in one file); sample | script `multi-type`; SwiftLint `type_body_length`; sample | script `multi-concept`; sample |
| 32 no magic values | script `magic-number` (decimal, hex/octal/binary, underscored, float and exponent literals; match arms, `assert!`/`debug_assert!` and every line except a `const`/`static` definition with an UPPER name, attributes and const tables), `magic-string` (the same string literal, 5+ chars with quotes, 3 or more times in one non-test file) | script `magic-number`, `magic-string`; SwiftLint `no_magic_numbers` | script `magic-number`, `magic-string` (module-level `const` definitions exempt) |
| 33 no bool params | script `bool-param`: `bool` anywhere in a signature (tuple types, `Option<bool>`, `pub(crate)` fns) and closure params `\|x: bool\|`; `impl Trait for` methods exempt by design; clippy `fn_params_excessive_bools` | script `bool-param` (`Bool` anywhere in the parameter list, so `(Bool) -> Void` too); SwiftLint `no_bool_parameter` | script `bool-param` (`: boolean` params) |
| 34 command-query separation | sample | sample | sample |
| 35 no hidden side effects | sample | sample | sample |
| 36 no duplication | sample (jscpd report in review) | sample | sample |
| 37 dead code | clippy `dead_code`, `unused`; script `allow-dead`, `allow-lint` (every `#[allow]`/`#![allow]`/`#[expect]` lint needs a `reason = ".."` of 12+ characters AND an entry in `allow-baseline.txt`; `unsafe_code` is exempt in a file whose unsafe blocks carry `// SAFETY:` comments); cargo-machete | SwiftLint `unused_declaration`, `unused_import` | knip; eslint `no-unused-vars`; script `comment` (commented-out code is a comment) |
| 38 injected dependencies | script `global`, `direct-time` (`Instant::now()`, `SystemTime::now()`, `Utc/Local::now()`), `direct-fs` (`std::fs`, `tokio::fs`, `File::open/create`); files under an `adapters/` directory are the only exemption; sample | script `global`; sample | script `global`; sample |
| 39 table-driven tests | script `table-driven`: more than 2 tests need a real table: a `const`/`static` array (or `let cases = [..]`) AND a loop over it (`for .. in CASES`/`.iter()`/`.for_each`), or a `macro_rules!` test generator, or rstest/test_case; the word "cases" no longer counts | script `table-driven`: a `for .. in cases` loop, `cases.forEach` or `@Test(arguments:)` | script `table-driven` (`it.each`/`test.each`) |
| 40 test the interface | sample | sample | script `test-style`; sample |
| 41 no real infrastructure | script `real-infra`: sockets (`TcpStream`, `TcpListener`, `UdpSocket`, `Unix*`), `tempfile`/`tempdir`/`TempDir`, `std::fs`/`tokio::fs`/`File::open`, `thread::sleep` and an unpaused `sleep(`, `Command::new`, `home_dir`, `env::temp_dir`, `reqwest::`; a line naming a `Fake*`/`Mock*`/`InMemory*`/`Stub*`/`Recording*` type is allowed; in Rust the body of a fn carrying `#[ignore = "real-infra: <what and why>"]` (3+ word reason) and any file under a `real_infra/` test-support directory are exempt, because those tests are opt-in and run by the `Real-infrastructure tests` CI step with `--include-ignored` (an unmarked test is still flagged); clippy `disallowed_methods` | script `real-infra` | script `real-infra`, `sleep` |
| 42 1:1 tests | script `test-missing` (inline `mod tests` for any file with a `fn` or an `impl`; struct/enum-only files stay exempt) | script `test-missing` (`<Source>Tests.swift` for any file with `func` or `init`) | script `test-missing`, `test-orphan`, `test-location` |
| 43 no free test files | script `test-sibling`, `test-in-src`, `cfg-test`, `test-orphan` | script `test-orphan` | script `test-orphan`, `test-scenario`, `test-location` |
| 44 size 500 | script `size` (non-test lines) | script `size`; SwiftLint `file_length` | script `size` |
| 45 split by responsibility | sample | sample | sample |

## Sampled review (rules that are not mechanizable)

Rules marked sample: 10-11, 13, 23, 25, 28 (consumer-side), 29, 31 (beyond file shape), 34-36, 38 (beyond clock/fs), 40, 45. Procedure: every cycle the standards agent reads at least five changed files per language chosen by `git diff --stat` size, plus every new type, and checks each sample rule; findings go to the owner as `file:line, rule number, required change`. The reviewer agent repeats this per crate against `docs/plans/2026-11-14-code-review-checklist.md` before the merge gate.

## TS rule 1 decision

Rule 1 asks for one parameter per line, always. In TypeScript the formatter is prettier, which re-joins any parameter list that fits in `printWidth`, so an always-vertical rule cannot be satisfied and `prettier --check` green at the same time. The alternatives were tried on paper and rejected: `@stylistic/function-paren-newline` with `minItems: 2` also hits every call site and runs against prettier on 1,900 legacy files (ESLint 10 removed the core rule); replacing prettier for the changed scope needs a full stylistic formatter preset and a per-file scope list, which is a baseline that grows. Decision: mechanically impossible with prettier, so the enforced form is the strongest one prettier allows. A parameter list that wraps is one per line (prettier native), `[param-count]` caps a function at 4 parameters so lists stay short, and `[param-layout]` rejects a multi-line list that packs two parameters on a line. Anyone who wants always-vertical must first drop prettier for TypeScript; that is an owner decision about tooling, not a review judgement.

## Cycle check

`tools/scripts/check-cycles.py` finds import cycles (Tarjan strongly connected components). Rust: one node per module file of a crate (nested directories included), edges from `use` trees and `crate::`/`self::`/`super::` paths outside the test module; the crate root (`lib.rs`/`main.rs`) is not a node. TypeScript: one node per file, edges from static imports (`@/` alias and relative), reported only when the cycle contains a changed file. A module cycle is fixed by extracting the shared contract into a third, lower module.

## Clippy `excessive_nesting`

Verified on a scratch crate: clippy counts the function body as depth 1, so `excessive-nesting-threshold = 2` rejects an `if` inside a `for` (manifest level 2, allowed) and the manifest's level-3 block is rejected only at threshold 3. `app/clippy.toml` therefore stays at 3, a backstop that fires on blocks nested inside a level-3 block. The awk `depth` rule is the authority for the manifest definition (function body is level 1, a block inside it level 2, a third level is rejected).

## Baselines (ratchet)

Rules that were widened or added in the 2026-11-16 tooling pass would fail the tree on day one, so their findings are compared with a committed per-file count instead of failing outright. `tools/scripts/check-ratchet.py` filters every checker's output:

- Ratcheted rules, Rust: `param-count`, `param-layout`, `bool-param`, `magic-number`, `magic-string`, `named-return`, `spec-ref`, `platform-ref`, `tag-reason`, `real-infra`, `table-driven`, `test-missing`, `panic`, `cycle`, `result-string`, `map-err-ignore`, `swallowed-error`, `direct-time`, `direct-fs`, `trait-size`, `fn-count`, `allow-lint`. TypeScript: the shared ones plus `layer`, `real-infra`, `table-driven`, `test-missing`. Swift: the shared ones plus `real-infra`, `table-driven`, `test-missing`. Every other rule (`depth`, `else`, `comment`, `fn-length`, `size`, `multi-type`, `global`, ...) stays a hard failure with no baseline.
- Baselines: `tools/scripts/rust-baseline.txt`, `ts-baseline.txt`, `swift-baseline.txt` (`path<TAB>rule<TAB>count`) and `allow-baseline.txt` (`path<TAB>allow-lint:<lint><TAB>count`). More findings than the baseline allows fails and lists every finding of that file and rule. Fewer than the baseline fails as `[baseline-stale]` until `check-<lang>-rules.sh --shrink` lowers it, so a fixed finding can never be re-introduced in the same file.
- Baselines only shrink: the `standards` CI job runs `check-ratchet.py --guard OLD NEW` against the base branch's file and fails when any count or key grew. `--seed` exists for the initial seeding only.
- `--strict` ignores every baseline; the pull-request-into-`master` job runs it. The merge gate is a strict run with zero findings, so the baselines must be empty before the merge to `master`.
- New adapters: a file under a directory named `adapters/` may read the clock and touch the filesystem; any other file counts against the baseline.

## Open gaps

- Rule 1 (TS) always-vertical: see the decision above.
- Rule 39 threshold: a module with one or two tests needs no table.
- Rule 20 (TS): cycles are reported only when they contain a changed file in CI; legacy cycles show with `--all`.
- Rule 44 counts non-test lines without blanks and doc comments; raw size is not checked.
- Rules 34 (command-query separation), 35, 36, 40, 45 and the intent half of 13, 23, 25, 28, 29 have no mechanical check.
- Rule 32 `magic-string` only sees a literal repeated 3+ times in one file; a one-off magic string is review-only. Hex/float/underscore magic numbers are caught; numbers written as expressions of allowed numbers (`10 * 100`) are not.
- Rule 33: `impl Trait for` methods are exempt, the signature is dictated by the trait.
- `--full` (clippy, deny, machete, swiftlint, swift-format, eslint) needs tools that are not available in every environment; CI runs them (`rust-lint`, `swift-lint`).
