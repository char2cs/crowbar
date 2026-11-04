// Monaco's editor-feature chain (find, folding, hover, suggest, …), eager on purpose: a
// CodeEditorWidget snapshots the registered contributions and actions when it is built, and a pane
// builds its widget before any language loads. Only the per-language contributions stay deferred.
import 'monaco-editor/esm/vs/editor/edcore.main.js'
import { registerMonarchLanguages } from './monarch-languages'
import { registerViewportSemanticTokens } from './semantic-tokens-provider'

/**
 * On-demand grammar/language-service loaders, keyed by monaco language id. Each `import()` is its
 * own chunk, fetched only for a language actually opened. Keys must cover every id
 * `toMonacoLanguageId` can produce, bar the Monarch ones (monarch-languages.ts) and documented
 * exclusions: language-contributions.test.ts enforces it. One module may serve two ids
 * (`cpp.contribution` → c, cpp), and css/less/scss/html pair a Monarch grammar with a service.
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
  // TypeScript/JavaScript get grammar only: completion, hover, diagnostics &c.
  // come from the daemon's language server (monaco-lsp-providers.ts). Monaco's
  // in-browser TS service would answer the same queries a second time, with
  // single-file knowledge and false "cannot find module" errors.
  javascript: () =>
    import('monaco-editor/esm/vs/basic-languages/javascript/javascript.contribution'),
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
    import('monaco-editor/esm/vs/basic-languages/typescript/typescript.contribution'),
  xml: () => import('monaco-editor/esm/vs/basic-languages/xml/xml.contribution'),
  yaml: () => import('monaco-editor/esm/vs/basic-languages/yaml/yaml.contribution'),
}

/**
 * File extension → the language whose contribution to fetch, mirrored from each contribution's
 * `extensions`. Must agree per extension with `toMonacoLanguageId` (which assigns the model's id),
 * or a model gets an id whose grammar never loads.
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

registerMonarchLanguages()
registerViewportSemanticTokens()
