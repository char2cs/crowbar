/**
 * Athas language id → monaco language id, used for MODEL ASSIGNMENT
 * (`toMonacoLanguageId` feeds `langForUri` / the editor+diff surfaces).
 *
 * DRIFT GUARD: `language-contributions.ts` independently maps file EXTENSIONS
 * to these same monaco ids to decide which grammar contribution to fetch on
 * demand. Every non-plaintext monaco id produced here must have a loader
 * there, be one of its custom Monarch registrations, or be on the documented
 * exclusion list in `language-contributions.test.ts` — otherwise models get an
 * id whose grammar never loads (silent plaintext). That test enforces this;
 * adding a value here will fail it until the loader side is updated too.
 */
export const MONACO_LANGUAGE_BY_ATHAS_ID: Record<string, string> = {
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
  // ERB (`.erb`) is HTML with embedded Ruby; monaco has no dedicated grammar,
  // so host it in the html grammar (its loader is registered). Without this the
  // resolver's 'embedded_template' id hit `toMonacoLanguageId`'s `?? languageId`
  // fallback — an id nothing registers — and .erb rendered as plaintext.
  embedded_template: 'html',
  gitattributes: 'gitattributes',
  gitignore: 'gitignore',
  go: 'go',
  graphql: 'graphql',
  html: 'html',
  java: 'java',
  javascript: 'javascript',
  // JSX (`.jsx`) routes through monaco's javascript grammar, mirroring
  // `typescriptreact: 'typescript'`. Without this the resolver's
  // 'javascriptreact' id fell through `toMonacoLanguageId`'s `?? languageId`
  // fallback to an unregistered id, so .jsx rendered as plaintext.
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
  return MONACO_LANGUAGE_BY_ATHAS_ID[languageId] ?? languageId
}
