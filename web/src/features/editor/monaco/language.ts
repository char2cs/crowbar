// Language id → Monaco language id for model assignment. language-contributions.ts maps
// extensions to the same ids to fetch grammars: every non-plaintext id here needs a loader there
// (or a listed exclusion) or renders as plaintext. language-contributions.test.ts enforces it.
export const MONACO_LANGUAGE_BY_LANGUAGE_ID: Record<string, string> = {
  angular: 'html',
  bash: 'shell',
  c: 'c',
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
  gitattributes: 'gitattributes',
  gitignore: 'gitignore',
  go: 'go',
  graphql: 'graphql',
  html: 'html',
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
  // 'proto' is the id monaco's protobuf.contribution actually registers —
  // mapping to 'protobuf' (as this did historically) produced an id nothing
  // ever registers, so .proto files silently rendered as plaintext.
  protobuf: 'proto',
  python: 'python',
  ql: 'sql',
  rescript: 'javascript',
  ruby: 'ruby',
  rust: 'rust',
  sass: 'scss',
  scala: 'scala',
  scheme: 'scheme',
  scss: 'scss',
  solidity: 'sol',
  sql: 'sql',
  svelte: 'html',
  swift: 'swift',
  systemrdl: 'plaintext',
  terraform: 'hcl',
  tla: 'plaintext',
  toml: 'toml',
  typescriptreact: 'typescript',
  typescript: 'typescript',
  vue: 'html',
  xml: 'xml',
  yaml: 'yaml',
  zig: 'zig',
}

export function toMonacoLanguageId(languageId: string | null | undefined): string {
  if (!languageId) return 'plaintext'
  return MONACO_LANGUAGE_BY_LANGUAGE_ID[languageId] ?? languageId
}
