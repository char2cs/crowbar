// The bare 'monaco-editor' specifier resolves to the package's "main" bundle
// (`editor.main.js`), which EAGERLY statically imports all 60+ built-in
// basic-languages contributions + the 4 heavy language services — undoing
// every bit of on-demand loading below the moment this module (imported
// eagerly by `editor-surface.tsx`/`monaco-diff-editor.tsx` for the custom
// diff/gitignore/… Monarch languages) loads. `editor.api` exposes the exact
// same `languages` singleton (both resolve through the shared
// `editor.api2.js` — confirmed by reading monaco's own source; no split-brain
// risk from other files still importing the bare specifier for types) MINUS
// that eager language bundling.
//
// `edcore.main.js` is the EDITOR-FEATURE contribution chain (find, folding,
// hover, suggest, bracket-matching, multicursor, sticky-scroll, rename,
// context-menu, … = `editor.all.js`, PLUS the 8 standalone modules: the four
// quick-access providers, references widget, inspectTokens, toggleHighContrast,
// iPadShowKeyboard) — deliberately EAGER, and load-bearing: monaco's
// CodeEditorWidget SNAPSHOTS EditorExtensionsRegistry.getEditorContributions()
// AND .getEditorActions() AT CONSTRUCTION (codeEditorWidget.js ~221-232), and
// `EditorManager.mountPane` builds the retained widget BEFORE `showBuffer`
// resolves any language. If this chain only arrived as a side effect of the
// first per-language `import()` below (every `*.contribution` module
// transitively imports the same set), any widget constructed before that
// first language load finished would be PERMANENTLY feature-degraded — no
// find widget, no folding, no hover, no suggest — for the life of the pane.
// edcore.main (not just editor.all) because `_.contribution.js` pulls exactly
// this superset — using the smaller set would leave those 8 modules as a
// stray lazy chunk whose contributions/actions miss early-constructed widgets.
// This module is statically imported by both editor surfaces AND by
// `monaco-adapters.ts` (the armEditor dynamic-import target), so the chain
// registers before ANY widget construction on every path; only the
// per-LANGUAGE contributions stay deferred.
import 'monaco-editor/esm/vs/editor/edcore.main.js'
import { languages } from 'monaco-editor/esm/vs/editor/editor.api.js'
import { registerTreeSitterSemanticTokens } from './semantic-tokens-provider'

/**
 * On-demand grammar/language-service loaders, keyed by MONACO language id (the
 * `id` each contribution registers via `languages.register`, mirrored from the
 * real contribution files under `monaco-editor/esm/vs/{basic-languages,language}`).
 *
 * Each basic-languages `*.contribution` module is already internally lazy
 * (`registerLanguage` in monaco's own `_.contribution.js` sets up a
 * `registerTokensProviderFactory` + `onLanguageEncountered` hook that defers the
 * actual tokenizer/keyword-list module) — but the CONTRIBUTION FILE ITSELF was
 * previously reached via 35 static imports at the top of this file (31
 * basic-languages + 4 language services). That put all of them on the SAME
 * chunk boundary as this module, so opening a single `.md` file paid the
 * parse/registration cost of every language (plus the 4 heavier
 * css/html/json/typescript language services) before any highlighting could
 * appear. Wrapping each in `import()` lets the bundler split them into
 * independent chunks, fetched only for the language actually opened. (The
 * shared editor-FEATURE chain stays eager via `edcore.main.js` above — see
 * that import's comment for why deferring it would be a bug, not a win.)
 *
 * DRIFT GUARD: keys here must cover every monaco language id the app's own
 * resolver (`getLanguageIdFromPath` → `toMonacoLanguageId`, i.e. the values of
 * `MONACO_LANGUAGE_BY_ATHAS_ID` in `monaco/language.ts`) can produce, except
 * ids registered by the custom Monarch section below or deliberately excluded.
 * `language-contributions.test.ts` asserts this — if you add a language to
 * `monaco/language.ts`, that test forces you to add its loader here (or a
 * documented exclusion there).
 *
 * Some monaco ids are combined loaders because ONE contribution module
 * registers TWO ids (`cpp.contribution` → `c` + `cpp`) or because a language's
 * full behavior originally came from TWO separately-imported modules (a
 * basic-languages Monarch tokenizer PLUS one of the 4 richer language
 * services): css/less/scss all route through the shared
 * `language/css/monaco.contribution`; typescript/javascript both route through
 * `language/typescript/monaco.contribution`; html has its own service; json has
 * no basic-languages counterpart at all (language-service only).
 */
const contributionLoaders: Record<string, () => Promise<unknown>> = {
  c: () => import('monaco-editor/esm/vs/basic-languages/cpp/cpp.contribution'),
  cpp: () => import('monaco-editor/esm/vs/basic-languages/cpp/cpp.contribution'),
  css: () =>
    Promise.all([
      import('monaco-editor/esm/vs/basic-languages/css/css.contribution'),
      import('monaco-editor/esm/vs/language/css/monaco.contribution'),
    ]),
  csharp: () => import('monaco-editor/esm/vs/basic-languages/csharp/csharp.contribution'),
  dart: () => import('monaco-editor/esm/vs/basic-languages/dart/dart.contribution'),
  dockerfile: () =>
    import('monaco-editor/esm/vs/basic-languages/dockerfile/dockerfile.contribution'),
  elixir: () => import('monaco-editor/esm/vs/basic-languages/elixir/elixir.contribution'),
  go: () => import('monaco-editor/esm/vs/basic-languages/go/go.contribution'),
  graphql: () => import('monaco-editor/esm/vs/basic-languages/graphql/graphql.contribution'),
  hcl: () => import('monaco-editor/esm/vs/basic-languages/hcl/hcl.contribution'),
  html: () =>
    Promise.all([
      import('monaco-editor/esm/vs/basic-languages/html/html.contribution'),
      import('monaco-editor/esm/vs/language/html/monaco.contribution'),
    ]),
  java: () => import('monaco-editor/esm/vs/basic-languages/java/java.contribution'),
  javascript: () =>
    Promise.all([
      import('monaco-editor/esm/vs/basic-languages/javascript/javascript.contribution'),
      import('monaco-editor/esm/vs/language/typescript/monaco.contribution'),
    ]),
  json: () => import('monaco-editor/esm/vs/language/json/monaco.contribution'),
  kotlin: () => import('monaco-editor/esm/vs/basic-languages/kotlin/kotlin.contribution'),
  less: () =>
    Promise.all([
      import('monaco-editor/esm/vs/basic-languages/less/less.contribution'),
      import('monaco-editor/esm/vs/language/css/monaco.contribution'),
    ]),
  lua: () => import('monaco-editor/esm/vs/basic-languages/lua/lua.contribution'),
  markdown: () => import('monaco-editor/esm/vs/basic-languages/markdown/markdown.contribution'),
  'objective-c': () =>
    import('monaco-editor/esm/vs/basic-languages/objective-c/objective-c.contribution'),
  php: () => import('monaco-editor/esm/vs/basic-languages/php/php.contribution'),
  proto: () => import('monaco-editor/esm/vs/basic-languages/protobuf/protobuf.contribution'),
  // Not in the original 35 static imports — but pre-Task-5 the bare
  // 'monaco-editor' specifier (= editor.main.js) registered ALL built-ins,
  // python included, so python WAS Monarch-highlighted. Kept registered here
  // to preserve that behavior now that editor.main.js is off the graph.
  python: () => import('monaco-editor/esm/vs/basic-languages/python/python.contribution'),
  ruby: () => import('monaco-editor/esm/vs/basic-languages/ruby/ruby.contribution'),
  rust: () => import('monaco-editor/esm/vs/basic-languages/rust/rust.contribution'),
  scala: () => import('monaco-editor/esm/vs/basic-languages/scala/scala.contribution'),
  scheme: () => import('monaco-editor/esm/vs/basic-languages/scheme/scheme.contribution'),
  scss: () =>
    Promise.all([
      import('monaco-editor/esm/vs/basic-languages/scss/scss.contribution'),
      import('monaco-editor/esm/vs/language/css/monaco.contribution'),
    ]),
  shell: () => import('monaco-editor/esm/vs/basic-languages/shell/shell.contribution'),
  sol: () => import('monaco-editor/esm/vs/basic-languages/solidity/solidity.contribution'),
  sql: () => import('monaco-editor/esm/vs/basic-languages/sql/sql.contribution'),
  swift: () => import('monaco-editor/esm/vs/basic-languages/swift/swift.contribution'),
  typescript: () =>
    Promise.all([
      import('monaco-editor/esm/vs/basic-languages/typescript/typescript.contribution'),
      import('monaco-editor/esm/vs/language/typescript/monaco.contribution'),
    ]),
  xml: () => import('monaco-editor/esm/vs/basic-languages/xml/xml.contribution'),
  yaml: () => import('monaco-editor/esm/vs/basic-languages/yaml/yaml.contribution'),
}

/**
 * File extension → monaco language id, mirrored from the `extensions` arrays
 * each contribution module above registers (see the `.contribution.js` source
 * for the authoritative list). Extension-only: contributions that also match by
 * bare filename (`Dockerfile`, `rakefile`, `Gemfile`, …) still register the
 * language id itself via their loader (unchanged), they just aren't a trigger
 * key here — those files fall back to whatever `langForUri` resolves them to
 * (already-registered-elsewhere or plaintext) until something else opens a
 * same-language file with a matching extension.
 *
 * NOTE — parallel source of truth: the app's own resolver
 * (`utils/language-id.ts` extension tables → `monaco/language.ts`
 * `MONACO_LANGUAGE_BY_ATHAS_ID`) independently maps paths to monaco language
 * ids for MODEL ASSIGNMENT, while this map only decides which CONTRIBUTION to
 * fetch. They must agree on the id per extension or a model gets an id whose
 * grammar never loads (silent plaintext). The drift test in
 * `language-contributions.test.ts` pins the id-level agreement; keep this
 * map's values consistent with `toMonacoLanguageId`'s output when editing
 * either side.
 */
const extensionToLanguage: Record<string, string> = {
  '.c': 'c',
  '.h': 'c',
  '.cpp': 'cpp',
  '.cc': 'cpp',
  '.cxx': 'cpp',
  '.hpp': 'cpp',
  '.hh': 'cpp',
  '.hxx': 'cpp',
  '.css': 'css',
  '.cs': 'csharp',
  '.csx': 'csharp',
  '.cake': 'csharp',
  '.dart': 'dart',
  '.dockerfile': 'dockerfile',
  '.ex': 'elixir',
  '.exs': 'elixir',
  '.go': 'go',
  '.graphql': 'graphql',
  '.gql': 'graphql',
  '.tf': 'hcl',
  '.tfvars': 'hcl',
  '.hcl': 'hcl',
  '.html': 'html',
  '.htm': 'html',
  '.shtml': 'html',
  '.xhtml': 'html',
  '.mdoc': 'html',
  '.jsp': 'html',
  '.asp': 'html',
  '.aspx': 'html',
  '.jshtm': 'html',
  '.java': 'java',
  '.jav': 'java',
  '.js': 'javascript',
  '.es6': 'javascript',
  '.jsx': 'javascript',
  '.mjs': 'javascript',
  '.cjs': 'javascript',
  '.json': 'json',
  '.bowerrc': 'json',
  '.jshintrc': 'json',
  '.jscsrc': 'json',
  '.eslintrc': 'json',
  '.babelrc': 'json',
  '.har': 'json',
  '.kt': 'kotlin',
  '.kts': 'kotlin',
  '.less': 'less',
  '.lua': 'lua',
  '.md': 'markdown',
  '.markdown': 'markdown',
  '.mdown': 'markdown',
  '.mkdn': 'markdown',
  '.mkd': 'markdown',
  '.mdwn': 'markdown',
  '.mdtxt': 'markdown',
  '.mdtext': 'markdown',
  '.m': 'objective-c',
  '.php': 'php',
  '.php4': 'php',
  '.php5': 'php',
  '.phtml': 'php',
  '.ctp': 'php',
  '.proto': 'proto',
  '.py': 'python',
  '.rpy': 'python',
  '.pyw': 'python',
  '.cpy': 'python',
  '.gyp': 'python',
  '.gypi': 'python',
  '.rb': 'ruby',
  '.rbx': 'ruby',
  '.rjs': 'ruby',
  '.gemspec': 'ruby',
  '.pp': 'ruby',
  '.rs': 'rust',
  '.rlib': 'rust',
  '.scala': 'scala',
  '.sc': 'scala',
  '.sbt': 'scala',
  '.scm': 'scheme',
  '.ss': 'scheme',
  '.sch': 'scheme',
  '.rkt': 'scheme',
  '.scss': 'scss',
  '.sh': 'shell',
  '.bash': 'shell',
  '.sol': 'sol',
  '.sql': 'sql',
  '.swift': 'swift',
  '.ts': 'typescript',
  '.tsx': 'typescript',
  '.cts': 'typescript',
  '.mts': 'typescript',
  '.xml': 'xml',
  '.xsd': 'xml',
  '.dtd': 'xml',
  '.ascx': 'xml',
  '.csproj': 'xml',
  '.config': 'xml',
  '.props': 'xml',
  '.targets': 'xml',
  '.wxi': 'xml',
  '.wxl': 'xml',
  '.wxs': 'xml',
  '.xaml': 'xml',
  '.svg': 'xml',
  '.svgz': 'xml',
  '.opf': 'xml',
  '.xslt': 'xml',
  '.xsl': 'xml',
  '.yaml': 'yaml',
  '.yml': 'yaml',
}

const loaded = new Set<string>()
const inflight = new Map<string, Promise<unknown>>()

/**
 * Load (once) the grammar/language-service contribution(s) for the language
 * that owns `path`'s extension. Dedupes across repeat calls for the same
 * language (a no-op once loaded) and across concurrent calls (shares one
 * in-flight import). Unknown/unregistered extensions resolve immediately —
 * never throws for a path this module doesn't recognize.
 */
export async function loadLanguageForPath(path: string): Promise<void> {
  // Isolate the basename before looking for the extension dot: a dot in a
  // DIRECTORY segment must not be mistaken for one (`/v1.2/README` has no
  // extension; naive lastIndexOf('.') over the whole path would yield
  // ".2/readme").
  const basename = path.slice(Math.max(path.lastIndexOf('/'), path.lastIndexOf('\\')) + 1)
  const dot = basename.lastIndexOf('.')
  if (dot === -1) return
  const ext = basename.slice(dot).toLowerCase()
  const lang = extensionToLanguage[ext]
  if (!lang || loaded.has(lang)) return
  const loader = contributionLoaders[lang]
  if (!loader) return
  let p = inflight.get(lang)
  if (!p) {
    p = loader()
    inflight.set(lang, p)
  }
  try {
    await p
    loaded.add(lang)
  } finally {
    inflight.delete(lang)
  }
}

/** Test-only introspection of which languages have finished loading. */
export function __loadedLanguagesForTests(): string[] {
  return [...loaded]
}

/** Test-only: the monaco language ids this module can load on demand
 *  (the drift test cross-checks these against `monaco/language.ts`). */
export function __loaderLanguageIdsForTests(): string[] {
  return Object.keys(contributionLoaders)
}

function ensureLanguage(id: string, extensions: string[], aliases: string[], filenames?: string[]) {
  if (languages.getLanguages().some((language) => language.id === id)) return
  languages.register({ id, extensions, aliases, filenames })
}

ensureLanguage('diff', ['.diff', '.patch'], ['Diff', 'diff', 'patch'])
languages.setMonarchTokensProvider('diff', {
  tokenizer: {
    root: [
      [/^@@.*$/, 'keyword'],
      [/^diff --git.*$/, 'keyword'],
      [/^index\s.*$/, 'comment'],
      [/^---.*$/, 'comment'],
      [/^\+\+\+.*$/, 'comment'],
      [/^\+.*/, 'string'],
      [/^-.*/, 'regexp'],
    ],
  },
})

ensureLanguage(
  'gitignore',
  [
    '.gitignore',
    '.dockerignore',
    '.ignore',
    '.npmignore',
    '.eslintignore',
    '.prettierignore',
    '.stylelintignore',
    '.vscodeignore',
    '.rgignore',
    '.fdignore',
  ],
  ['Git Ignore', 'gitignore', 'ignore'],
  [
    '.gitignore',
    '.dockerignore',
    '.ignore',
    '.npmignore',
    '.eslintignore',
    '.prettierignore',
    '.stylelintignore',
    '.vscodeignore',
    '.rgignore',
    '.fdignore',
  ],
)
languages.setMonarchTokensProvider('gitignore', {
  tokenizer: {
    root: [
      [/^\s*#.*$/, 'comment'],
      [/^\s*!/, 'keyword'],
      [/\\[# !]/, 'string.escape'],
      [/[/?*[\]]/, 'operator'],
      [/[^/?*[\]\s]+/, 'string'],
    ],
  },
})

ensureLanguage(
  'gitattributes',
  ['.gitattributes'],
  ['Git Attributes', 'gitattributes'],
  ['.gitattributes'],
)
languages.setMonarchTokensProvider('gitattributes', {
  tokenizer: {
    root: [
      [/^\s*#.*$/, 'comment'],
      [/^\s*\[attr\][^\s]+/, 'attribute'],
      [/^\S+/, 'string'],
      [/[!-](?=[A-Za-z0-9_.-])/, 'operator'],
      [/[A-Za-z0-9_.-]+(?==)/, 'key'],
      [/=/, 'operator'],
      [/[A-Za-z0-9_.-]+/, 'key'],
    ],
  },
})

ensureLanguage('toml', ['.toml'], ['TOML', 'toml'])
languages.setMonarchTokensProvider('toml', {
  tokenizer: {
    root: [
      [/^\s*#.*$/, 'comment'],
      [/\[[^\]]+\]/, 'type'],
      [/^\s*[A-Za-z0-9_.-]+(?=\s*=)/, 'key'],
      [/".*?"/, 'string'],
      [/'[^']*'/, 'string'],
      [/\b(true|false)\b/, 'keyword'],
      [/\b\d+(\.\d+)?\b/, 'number'],
    ],
  },
})

ensureLanguage('zig', ['.zig'], ['Zig', 'zig'])
languages.setMonarchTokensProvider('zig', {
  tokenizer: {
    root: [
      [/\/\/.*$/, 'comment'],
      [/\/\*/, 'comment', '@comment'],
      [/"([^"\\]|\\.)*$/, 'string.invalid'],
      [/"/, 'string', '@string'],
      [/'([^'\\]|\\.)*'/, 'string'],
      [
        /\b(addrspace|align|allowzero|and|anyframe|anytype|asm|async|await|break|callconv|catch|comptime|const|continue|defer|else|enum|errdefer|error|export|extern|fn|for|if|inline|linksection|noalias|noinline|nosuspend|opaque|or|orelse|packed|pub|resume|return|struct|suspend|switch|test|threadlocal|try|union|unreachable|usingnamespace|var|volatile|while)\b/,
        'keyword',
      ],
      [/\b(true|false|null|undefined)\b/, 'constant'],
      [
        /\b[ui](8|16|32|64|128|size)\b|\b(f16|f32|f64|f80|f128|bool|void|noreturn|type|anyerror|comptime_int|comptime_float)\b/,
        'type',
      ],
      [/@[A-Za-z_][\w]*/, 'keyword'],
      [/\b0x[0-9a-fA-F_]+\b|\b\d[\d_]*(\.\d[\d_]*)?\b/, 'number'],
    ],
    comment: [
      [/[^*/]+/, 'comment'],
      [/\*\//, 'comment', '@pop'],
      [/[*/]/, 'comment'],
    ],
    string: [
      [/[^\\"]+/, 'string'],
      [/\\./, 'string.escape'],
      [/"/, 'string', '@pop'],
    ],
  },
})

ensureLanguage('elm', ['.elm'], ['Elm', 'elm'])
languages.setMonarchTokensProvider('elm', {
  tokenizer: {
    root: [
      [/--.*$/, 'comment'],
      [/\{-/, 'comment', '@comment'],
      [/"([^"\\]|\\.)*$/, 'string.invalid'],
      [/"/, 'string', '@string'],
      [/'([^'\\]|\\.)*'/, 'string'],
      [
        /\b(alias|as|case|else|exposing|if|import|in|infix|let|module|of|port|then|type|where)\b/,
        'keyword',
      ],
      [/\b(True|False)\b/, 'constant'],
      [/\b[A-Z][\w']*/, 'type'],
      [/\b\d+(\.\d+)?\b/, 'number'],
    ],
    comment: [
      [/[^{-]+/, 'comment'],
      [/\{-/, 'comment', '@push'],
      [/-\}/, 'comment', '@pop'],
      [/[{-]/, 'comment'],
    ],
    string: [
      [/[^\\"]+/, 'string'],
      [/\\./, 'string.escape'],
      [/"/, 'string', '@pop'],
    ],
  },
})

ensureLanguage('elisp', ['.el'], ['Emacs Lisp', 'elisp'])
languages.setMonarchTokensProvider('elisp', {
  tokenizer: {
    root: [
      [/;.*/, 'comment'],
      [/"([^"\\]|\\.)*$/, 'string.invalid'],
      [/"/, 'string', '@string'],
      [
        /\b(defun|defmacro|defvar|defcustom|defgroup|defconst|let|let\*|lambda|if|when|unless|cond|pcase|progn|save-excursion|interactive|setq|setq-local|require|provide|use-package)\b/,
        'keyword',
      ],
      [/\b(nil|t)\b/, 'constant'],
      [/:[A-Za-z0-9_-]+/, 'type'],
      [/\b\d+(\.\d+)?\b/, 'number'],
      [/[()'`,#]/, 'delimiter'],
    ],
    string: [
      [/[^\\"]+/, 'string'],
      [/\\./, 'string.escape'],
      [/"/, 'string', '@pop'],
    ],
  },
})

ensureLanguage('lockfile', ['.lock'], ['Lockfile', 'lockfile'])
languages.setMonarchTokensProvider('lockfile', {
  tokenizer: {
    root: [
      [/^\s*#.*$/, 'comment'],
      [/^\s*("[^"]+"|'[^']+'|[^:\s][^:]*)(?=:)/, 'key'],
      [/"([^"\\]|\\.)*"/, 'string'],
      [/'([^'\\]|\\.)*'/, 'string'],
      [/\b(true|false|null)\b/, 'constant'],
      [/\b\d+(\.\d+)?\b/, 'number'],
      [/[{}[\],:]/, 'delimiter'],
    ],
  },
})

ensureLanguage('ocaml', ['.ml', '.mli'], ['OCaml', 'ocaml'])
languages.setMonarchTokensProvider('ocaml', {
  tokenizer: {
    root: [
      [/\(\*/, 'comment', '@comment'],
      [/"([^"\\]|\\.)*$/, 'string.invalid'],
      [/"/, 'string', '@string'],
      [
        /\b(let|in|rec|type|module|open|match|with|function|fun|if|then|else|struct|sig|end)\b/,
        'keyword',
      ],
      [/\b(true|false)\b/, 'constant'],
      [/\b\d+(\.\d+)?\b/, 'number'],
    ],
    comment: [
      [/[^(*]+/, 'comment'],
      [/\*\)/, 'comment', '@pop'],
      [/[(*)]/, 'comment'],
    ],
    string: [
      [/[^\\"]+/, 'string'],
      [/\\./, 'string.escape'],
      [/"/, 'string', '@pop'],
    ],
  },
})

registerTreeSitterSemanticTokens()
