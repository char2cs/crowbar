const MONACO_LANGUAGE_BY_LANGUAGE_ID = {
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

export type LanguageId = keyof typeof MONACO_LANGUAGE_BY_LANGUAGE_ID

export function toMonacoLanguageId(languageId: string | null | undefined): string {
  if (!languageId) return 'plaintext'
  return Object.hasOwn(MONACO_LANGUAGE_BY_LANGUAGE_ID, languageId)
    ? MONACO_LANGUAGE_BY_LANGUAGE_ID[languageId as LanguageId]
    : languageId
}

export function hasGrammar(languageId: string): boolean {
  return (
    Object.hasOwn(MONACO_LANGUAGE_BY_LANGUAGE_ID, languageId) &&
    MONACO_LANGUAGE_BY_LANGUAGE_ID[languageId as LanguageId] !== 'plaintext'
  )
}
