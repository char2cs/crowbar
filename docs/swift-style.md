# Swift style manifest

Applies to the iOS Swift package (`app/mobile/ios`). The Rust plugin that bridges it (`app/mobile/ios-plugin`) follows `docs/rust-style.md`. Same numbering (M#) as the owner manifest and `docs/rust-style.md`; rules not listed there are not applicable to Swift.

Sources: [Swift API Design Guidelines](https://www.swift.org/documentation/api-design-guidelines/) (clarity at the point of use, every declaration documented, naming, argument labels), [swift-format rules](https://github.com/swiftlang/swift-format/blob/main/Documentation/RuleDocumentation.md) and [configuration](https://github.com/swiftlang/swift-format/blob/main/Documentation/Configuration.md), [SwiftLint rules](https://realm.github.io/SwiftLint/rule-directory.html), [Swift Package Manager](https://docs.swift.org/package-manager/) (Sources/Tests layout).

Enforcement: **format** = `swift-format lint --strict` (`.swift-format`); **lint** = `swiftlint lint --strict` (`.swiftlint.yml`); **script** = `tools/scripts/check-swift-rules.sh`; **review**.

| # | Rule (Swift form) | Enforcement |
|---|-------------------|-------------|
| M1 | Calls and declarations that do not fit on one line put one argument or parameter per line. At most 4 parameters. | format `lineBreakBeforeEachArgument`; lint `multiline_arguments`, `multiline_parameters`, `function_parameter_count` (4) |
| M2 | Nesting at most 2 levels in a function, 1 for types. | lint `nesting`, `cyclomatic_complexity` (10) |
| M3 | Guard clauses; no `else` after a branch that returns. | format `UseEarlyExits` |
| M4 | Return named types, not tuples of more than two members; no `inout` out-parameters. | lint `large_tuple`; review |
| M5 | A compound condition is bound to a named `let` before the `if`/`guard`. | review |
| M6 | Prefer `map`, `filter`, `reduce`, `first(where:)`, `contains(where:)` over hand loops; `for` over `forEach`. | format `ReplaceForEachWithForLoop`; lint `for_where`, `reduce_into`, `contains_over_first_not_nil`, `first_where` |
| M7 | No inline narrative comments (`//`), no `///` on non-public declarations, a `///` on every `public`/`open` declaration (one-line summary, then `- Parameter`, `- Returns`, `- Throws`). Allowed `//` comments start with `SPEC:`, `PLATFORM:` or `DEPRECATED:`. A lint suppression carries ` - reason` on the same line. | format `AllPublicDeclarationsHaveDocumentation`, `BeginDocumentationCommentWithOneLineSummary`, `ValidateDocumentationComments`, `NoBlockComments`; lint custom rule `no_inline_comment`, `missing_docs`; script `comment`, `doc-private`, `doc-missing`, `lint-reason` |
| M8 | One top-level type per file, the file named after it (`HostPicker` in `HostPicker.swift`, UpperCamelCase as the API guidelines name types). Extensions of the type may share its file. | lint `file_name`, `one_declaration_per_file`; script `multi-type`, `type-name` |
| M9 | One `Error` enum per feature, named `<Feature>Error`, conforming to `Error`, carrying context in associated values. | review |
| M10 | Each feature has one entry type named after the feature; no free functions beyond that. | review |
| M11 | A type with no stored state is an `enum` namespace with `static` functions, or a free function, not a class. | review |
| M12 | `init` only when there is state to establish; prefer the synthesized memberwise init (`UseSynthesizedInitializer`); factories start with `make` (API guidelines). | format `UseSynthesizedInitializer` |
| M13 | Public API is a protocol or a type with private storage; implementations are `private`/`internal`. Default access stays minimal; no `public` without a consumer outside the package. | lint `private_over_fileprivate`; format `FileScopedDeclarationPrivacy`; review |
| M14-17 | One-way dependency: Rust plugin -> Swift package -> UIKit/SwiftUI/Tauri API. Files do not import peers through `@testable` in production code; no cross-feature reach into internals; no cycles. | review; SwiftPM rejects target cycles |
| M22 | No `Util`, `Utils`, `Common`, `Helper(s)`, `Base`, `Misc` file or type names. | script `module-name` |
| M24 | No global mutable state or singletons: no top-level `var`, no `static var`, no `static let shared`. State is injected. | script `global`; lint custom rule `no_global_mutable_state` |
| M26-27 | Errors are values: `throws`/`Result`, never swallowed with `try?` that hides a failure the user must see. No `try!`, `as!`, `fatalError`, `preconditionFailure`, `!` force unwrap, implicitly unwrapped optionals, `print`. | format `NeverForceUnwrap`, `NeverUseForceTry`, `NeverUseImplicitlyUnwrappedOptionals`; lint `force_unwrapping`, `implicitly_unwrapped_optional`, `fatal_error_message`; script `panic` |
| M29 | Intent-revealing names, no abbreviations; omit needless words; booleans read as assertions (`isOpen`) (API guidelines). | format `AlwaysUseLowerCamelCase`, `TypeNamesShouldBeCapitalized`; lint `identifier_name`, `type_name` |
| M30 | A function does one thing and is at most 40 lines (warning at 30). | lint `function_body_length` |
| M31 | Single responsibility per type; type body at most 400 lines. | lint `type_body_length` |
| M32 | No magic numbers: named `let`/`static let` constants. | lint `no_magic_numbers` |
| M33 | No `Bool` flag parameters; use an enum or two functions. | script `bool-param`; lint custom rule `no_bool_parameter` |
| M34 | Command-query separation; mutating methods read as imperative verbs, queries as nouns (API guidelines). | review |
| M37 | Dead code is deleted; no unused imports or declarations. | lint `unused_import`, `unused_declaration` (analyzer rules, `swiftlint analyze`) |
| M38 | Depend on protocols; collaborators arrive through `init`, never fetched from a global. | review |
| M39-41 | Table-driven XCTest (an array of cases and one loop); test internal behaviour through the type's API; no real network, keychain, file system or UIKit device features; no sleeps (use expectations). | review |
| M42 | Every source file containing a `func` has exactly one test file: `Sources/X.swift` -> `Tests/<Target>/XTests.swift` holding all its tests. | script `test-missing` |
| M43 | No free test files: every `Tests/<Target>/YTests.swift` has `Sources/**/Y.swift`; shared fakes live in `Tests/<Target>/Support/`. | script `test-orphan` |
| M44-45 | A source file is at most 500 lines (lint warns at 400); overflow means a wrong abstraction. | script `size`; lint `file_length` |

Exempt from the test-file requirement: files without any `func` (pure data types, SwiftUI views whose only member is `body`). Anything a view needs to compute goes into a testable model type with its own file and tests.

Rust plugin (`app/mobile/ios-plugin`): all of `docs/rust-style.md` applies; it is not a workspace member, so its `Cargo.toml` carries the lint table (see `app/crates/README.md`) and `tools/scripts/check-rust-rules.sh` scans it by default. The FFI boundary to Swift is the only place `unsafe_code` may be allowed, with a crate-level `#![allow(unsafe_code, reason = "...")]` and a `// SAFETY:` comment on every block.

## How to run

```
tools/scripts/check-swift-rules.sh              # structure rules
tools/scripts/check-swift-rules.sh --self-test  # checker against its fixtures
tools/scripts/check-swift-rules.sh --full       # also swiftlint and swift-format (missing tool = failure; macOS runner)
```

Configs: `.swiftlint.yml` and `.swift-format` sit in the package directory.
