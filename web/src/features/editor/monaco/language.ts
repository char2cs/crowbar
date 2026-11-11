// Language id → Monaco language id for model assignment, and so the one list of languages that
// colour: every non-plaintext id here needs a registration in language-contributions.ts or renders
// as plaintext. language-contributions.test.ts enforces it.
export const MONACO_LANGUAGE_BY_LANGUAGE_ID = {
  angular: 'html',
  bash: 'shell',
  batch: 'bat',
  c: 'c',
  clojure: 'clojure',
  cpp: 'cpp',
  csharp: 'csharp',
  css: 'css',
  dart: 'dart',
  diff: 'diff',
  dockerfile: 'dockerfile',
  dotenv: 'plaintext',
  elixir: 'elixir',
  elisp: 'elisp',
  elm: 'elm',
  // ERB is HTML with embedded Ruby and Monaco has no grammar for it, so it is hosted in html;
  // an unmapped id would fall through to one nothing registers (plaintext).
  embedded_template: 'html',
  fsharp: 'fsharp',
  gitattributes: 'gitattributes',
  gitignore: 'gitignore',
  go: 'go',
  graphql: 'graphql',
  html: 'html',
  ini: 'ini',
  java: 'java',
  javascript: 'javascript',
  // JSX routes through Monaco's javascript grammar, as typescriptreact does through typescript;
  // unmapped, the id would fall through to one nothing registers (plaintext).
  javascriptreact: 'javascript',
  json: 'json',
  jsonc: 'json',
  kotlin: 'kotlin',
  less: 'less',
  lua: 'lua',
  lockfile: 'lockfile',
  markdown: 'markdown',
  nix: 'plaintext',
  objc: 'objective-c',
  ocaml: 'ocaml',
  php: 'php',
  powershell: 'powershell',
  // 'proto' is the id monaco's protobuf.contribution actually registers —
  // mapping to 'protobuf' (as this did historically) produced an id nothing
  // ever registers, so .proto files silently rendered as plaintext.
  protobuf: 'proto',
  python: 'python',
  ql: 'sql',
  r: 'r',
  rescript: 'javascript',
  restructuredtext: 'restructuredtext',
  ruby: 'ruby',
  rust: 'rust',
  sass: 'scss',
  scala: 'scala',
  scheme: 'scheme',
  scss: 'scss',
  shell: 'shell',
  solidity: 'sol',
  sql: 'sql',
  svelte: 'html',
  swift: 'swift',
  systemrdl: 'plaintext',
  terraform: 'hcl',
  text: 'plaintext',
  tla: 'plaintext',
  toml: 'toml',
  typescriptreact: 'typescript',
  typescript: 'typescript',
  vue: 'html',
  xml: 'xml',
  yaml: 'yaml',
  zig: 'zig',
} as const satisfies Record<string, string>

/** A language id the app knows how to hand Monaco. */
export type LanguageId = keyof typeof MONACO_LANGUAGE_BY_LANGUAGE_ID

export function toMonacoLanguageId(languageId: string | null | undefined): string {
  if (!languageId) return 'plaintext'
  return Object.hasOwn(MONACO_LANGUAGE_BY_LANGUAGE_ID, languageId)
    ? MONACO_LANGUAGE_BY_LANGUAGE_ID[languageId as LanguageId]
    : languageId
}

/** Whether a language id colours: Monaco has a grammar registered for what it maps to. */
export function hasGrammar(languageId: string): boolean {
  return (
    Object.hasOwn(MONACO_LANGUAGE_BY_LANGUAGE_ID, languageId) &&
    MONACO_LANGUAGE_BY_LANGUAGE_ID[languageId as LanguageId] !== 'plaintext'
  )
}
