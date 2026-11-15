# Rabbyte Manifest

Source of truth for every language in this repository. The owner does not merge code that breaks these rules, so each rule is enforced by a check (see `docs/standards-coverage.md`). Go-specific rules are replaced by the standard of the language being written (Rust: `docs/rust-style.md`, Swift: `docs/swift-style.md`, TypeScript: `docs/ts-style.md`). Everything else applies as written.

## I. Code
1. Parameters: one per line, always. Opening paren on the `func` line, closing `)` on its own line.
2. Depth: maximum 2 indentation levels. Extract a method instead of reaching level 3.
3. Flow: guard clauses first, no `else`, success return last.
4. Returns: named returns instead of `var` in the body. Never naked returns.
5. Conditions: extract long booleans into a named variable that reads like a sentence.
6. Stdlib: prefer `slices`, `maps` and `cmp` over hand-rolled loops.
7. Comments: no inline comments, none on unexported symbols, detailed doc comments on every exported symbol. Only deprecations, spec links and platform constraints are allowed.

## II. Types
8. One type per file, named after the type in `snake_case`.
9. A sentinel `ErrXxx` lives in the same file as its error struct.
10. The package's main free function lives in the file named after the package.
11. A stateless type is a package-level function named after the package, not a struct.
12. `NewXxx` only when there is real state to initialize.
13. Public APIs expose interfaces. Implementation structs are unexported.

## III. Modules
14. The package root exposes only the interface, the entry-point function, the exported errors and the types those signatures use.
15. Everything else lives in the module's own `internal/` subtree.
16. Sibling modules never import each other's internals. Same-layer modules never import each other.
17. Dependencies flow one way, from higher layers to lower ones.

## IV. Go internals
18. `internal/` is compiler-enforced: only the parent tree can import it. Use it as the default home for anything not deliberately public.
19. Nest `internal/` at the lowest module that needs it, not only at the repo root.
20. No import cycles. If two packages need each other, extract the shared contract into a third, lower package.
21. Package names are short, lowercase, singular nouns, with no underscores.
22. No `util`, `common`, `helpers`, `base` or `misc` packages. A package is named for what it provides.
23. No stutter: `vault.Vault` is the one allowed exception (rule 11), but never `vault.VaultManager` or `user.UserService`.
24. No `init()` and no mutable package-level state. Wire dependencies explicitly.
25. `context.Context` is always the first parameter, never stored in a struct.
26. Errors are values. Never ignore one. Wrap with `fmt.Errorf("...: %w", err)` and compare with `errors.Is` and `errors.As`.
27. No `panic` outside `main` and unrecoverable startup. Libraries return errors.
28. Define small interfaces (1 to 3 methods), in the package that consumes them where possible.

## V. Clean Code
29. Names reveal intent. A longer descriptive name beats a short cryptic one. No abbreviations beyond language conventions (`ctx`, `err`).
30. A function does one thing, at one level of abstraction, and stays small.
31. A type has a single responsibility, meaning one reason to change.
32. No magic numbers or strings. Use named constants.
33. No boolean flag parameters. Split into two functions.
34. Command-query separation: a function either changes state or returns information, not both.
35. No hidden side effects. A function does only what its name says.
36. No duplication, but extract only once the third repetition appears.
37. Delete dead code, commented-out code, and unused parameters. Never keep them "just in case".
38. Depend on abstractions, not concretions. Inject dependencies through constructors.

## VI. Tests
39. Table-driven tests, one behavior per case.
40. Test the exported interface, not internals.
41. No test touches real infrastructure by default. Use in-memory fakes.

## VII. Files
42. 1:1 tests. Every source file `x.go` has exactly one test file `x_test.go` beside it. All tests for the code in `x.go` live in `x_test.go`.
43. No free test files. A `_test.go` file with no matching source file must not exist. No `misc_test.go`, `integration_test.go`, or tests grouped by feature.
44. Size ceiling. Source files stay under 500 to 600 lines of code, with no exceptions. Test files are exempt.
45. Overflow means wrong abstraction. If a file needs more lines, the design is wrong. Split by responsibility (rules 8 and 31), never by arbitrary line count.
