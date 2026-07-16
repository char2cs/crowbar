// `monaco-editor`'s package.json exposes deep subpaths via a bare wildcard
// (`"./*": "./*"`, no explicit "types" condition). TypeScript's `bundler`
// module resolution accepts that for STATIC `import 'x'` side-effect imports
// (falling back to the adjacent `.contribution.d.ts`), but does NOT apply the
// same fallback when the identical specifier is used in a dynamic `import()`
// expression — that path requires a "types" export condition or an explicit
// ambient declaration, or `tsc` reports TS2307 even though the file exists on
// disk and Vite resolves/bundles it fine at build and dev-server time.
//
// `language-contributions.ts` on-demand-loads every one of these via
// `import()` (see `contributionLoaders`), so each exact specifier gets an
// ambient declaration here — 36 in total (32 basic-languages + 4 language
// services: the original 35 static imports plus python, which pre-Task-5 was
// registered implicitly by editor.main.js). Untyped (`unknown` default) —
// these are side-effect-only registration modules; no caller reads their
// exports.
declare module 'monaco-editor/esm/vs/basic-languages/cpp/cpp.contribution'
declare module 'monaco-editor/esm/vs/basic-languages/css/css.contribution'
declare module 'monaco-editor/esm/vs/basic-languages/csharp/csharp.contribution'
declare module 'monaco-editor/esm/vs/basic-languages/dart/dart.contribution'
declare module 'monaco-editor/esm/vs/basic-languages/dockerfile/dockerfile.contribution'
declare module 'monaco-editor/esm/vs/basic-languages/elixir/elixir.contribution'
declare module 'monaco-editor/esm/vs/basic-languages/go/go.contribution'
declare module 'monaco-editor/esm/vs/basic-languages/graphql/graphql.contribution'
declare module 'monaco-editor/esm/vs/basic-languages/hcl/hcl.contribution'
declare module 'monaco-editor/esm/vs/basic-languages/html/html.contribution'
declare module 'monaco-editor/esm/vs/basic-languages/java/java.contribution'
declare module 'monaco-editor/esm/vs/basic-languages/javascript/javascript.contribution'
declare module 'monaco-editor/esm/vs/basic-languages/kotlin/kotlin.contribution'
declare module 'monaco-editor/esm/vs/basic-languages/less/less.contribution'
declare module 'monaco-editor/esm/vs/basic-languages/lua/lua.contribution'
declare module 'monaco-editor/esm/vs/basic-languages/markdown/markdown.contribution'
declare module 'monaco-editor/esm/vs/basic-languages/objective-c/objective-c.contribution'
declare module 'monaco-editor/esm/vs/basic-languages/php/php.contribution'
declare module 'monaco-editor/esm/vs/basic-languages/protobuf/protobuf.contribution'
declare module 'monaco-editor/esm/vs/basic-languages/python/python.contribution'
declare module 'monaco-editor/esm/vs/basic-languages/ruby/ruby.contribution'
declare module 'monaco-editor/esm/vs/basic-languages/rust/rust.contribution'
declare module 'monaco-editor/esm/vs/basic-languages/scala/scala.contribution'
declare module 'monaco-editor/esm/vs/basic-languages/scheme/scheme.contribution'
declare module 'monaco-editor/esm/vs/basic-languages/scss/scss.contribution'
declare module 'monaco-editor/esm/vs/basic-languages/shell/shell.contribution'
declare module 'monaco-editor/esm/vs/basic-languages/solidity/solidity.contribution'
declare module 'monaco-editor/esm/vs/basic-languages/sql/sql.contribution'
declare module 'monaco-editor/esm/vs/basic-languages/swift/swift.contribution'
declare module 'monaco-editor/esm/vs/basic-languages/typescript/typescript.contribution'
declare module 'monaco-editor/esm/vs/basic-languages/xml/xml.contribution'
declare module 'monaco-editor/esm/vs/basic-languages/yaml/yaml.contribution'
declare module 'monaco-editor/esm/vs/language/css/monaco.contribution'
declare module 'monaco-editor/esm/vs/language/html/monaco.contribution'
declare module 'monaco-editor/esm/vs/language/json/monaco.contribution'
declare module 'monaco-editor/esm/vs/language/typescript/monaco.contribution'
