import { hasGrammar, type LanguageId } from '@/features/editor/monaco/language'

const ANGULAR_TEMPLATE_LANGUAGE_ID: LanguageId = 'angular'
const PLAIN_TEXT_LANGUAGE_ID: LanguageId = 'text'

function isAngularTemplatePath(filePath: string): boolean {
  const fileName = filePath.split('/').pop()?.toLowerCase() ?? filePath.toLowerCase()
  return fileName.endsWith('.component.html') || fileName.endsWith('.ng.html')
}

// Typed by LanguageId so an id the resolver could produce without a Monaco mapping fails tsc.
const EXTENSION_TO_LANGUAGE: Record<string, LanguageId> = {
  js: 'javascript',
  mjs: 'javascript',
  cjs: 'javascript',
  jsx: 'javascriptreact',
  ts: 'typescript',
  tsx: 'typescriptreact',
  mts: 'typescript',
  cts: 'typescript',
  py: 'python',
  rs: 'rust',
  go: 'go',
  java: 'java',
  c: 'c',
  h: 'c',
  cpp: 'cpp',
  cc: 'cpp',
  cxx: 'cpp',
  hpp: 'cpp',
  hh: 'cpp',
  hxx: 'cpp',
  cs: 'csharp',
  rb: 'ruby',
  php: 'php',
  html: 'html',
  htm: 'html',
  xml: 'xml',
  xsl: 'xml',
  xslt: 'xml',
  xsd: 'xml',
  dtd: 'xml',
  wsdl: 'xml',
  svg: 'xml',
  plist: 'xml',
  csproj: 'xml',
  vbproj: 'xml',
  fsproj: 'xml',
  props: 'xml',
  targets: 'xml',
  css: 'css',
  scss: 'scss',
  sass: 'sass',
  less: 'less',
  dockerfile: 'dockerfile',
  dockerignore: 'gitignore',
  diff: 'diff',
  gitattributes: 'gitattributes',
  gitignore: 'gitignore',
  ignore: 'gitignore',
  json: 'json',
  jsonc: 'json',
  yaml: 'yaml',
  yml: 'yaml',
  toml: 'toml',
  md: 'markdown',
  mdx: 'markdown',
  markdown: 'markdown',
  sh: 'bash',
  bash: 'bash',
  zsh: 'bash',
  patch: 'diff',
  swift: 'swift',
  kt: 'kotlin',
  kts: 'kotlin',
  scala: 'scala',
  lua: 'lua',
  nix: 'nix',
  scm: 'scheme',
  dart: 'dart',
  el: 'elisp',
  elm: 'elm',
  graphql: 'graphql',
  gql: 'graphql',
  ex: 'elixir',
  exs: 'elixir',
  ml: 'ocaml',
  mli: 'ocaml',
  proto: 'protobuf',
  ql: 'ql',
  qll: 'ql',
  sql: 'sql',
  sol: 'solidity',
  tf: 'terraform',
  tfvars: 'terraform',
  zig: 'zig',
  vue: 'vue',
  svelte: 'svelte',
  erb: 'embedded_template',
  lock: 'lockfile',
}

const FILENAME_TO_LANGUAGE: Record<string, LanguageId> = {
  '.bashrc': 'bash',
  '.zshrc': 'bash',
  '.bash_profile': 'bash',
  '.profile': 'bash',
  '.dockerignore': 'gitignore',
  '.eslintignore': 'gitignore',
  '.fdignore': 'gitignore',
  '.gitattributes': 'gitattributes',
  '.gitignore': 'gitignore',
  '.ignore': 'gitignore',
  '.npmignore': 'gitignore',
  '.prettierignore': 'gitignore',
  '.rgignore': 'gitignore',
  '.stylelintignore': 'gitignore',
  '.vscodeignore': 'gitignore',
  containerfile: 'dockerfile',
  dockerfile: 'dockerfile',
  'go.mod': 'go',
  'go.sum': 'go',
  'go.work': 'go',
}

function isEnvFileName(fileName: string): boolean {
  return fileName === '.env' || fileName.startsWith('.env.')
}

function getLanguageIdFromExtension(extension: string): string | null {
  const normalized = extension.replace(/^\./, '').toLowerCase()
  return EXTENSION_TO_LANGUAGE[normalized] || null
}

const LANGUAGE_DISPLAY_NAMES: Partial<Record<LanguageId, string>> = {
  [ANGULAR_TEMPLATE_LANGUAGE_ID]: 'Angular Template',
  javascript: 'JavaScript',
  javascriptreact: 'JSX',
  typescript: 'TypeScript',
  typescriptreact: 'TSX',
  python: 'Python',
  rust: 'Rust',
  go: 'Go',
  java: 'Java',
  c: 'C',
  cpp: 'C++',
  csharp: 'C#',
  ruby: 'Ruby',
  php: 'PHP',
  html: 'HTML',
  css: 'CSS',
  diff: 'Diff',
  dotenv: 'Dotenv',
  json: 'JSON',
  yaml: 'YAML',
  toml: 'TOML',
  markdown: 'Markdown',
  bash: 'Bash',
  swift: 'Swift',
  kotlin: 'Kotlin',
  scala: 'Scala',
  lua: 'Lua',
  nix: 'Nix',
  dart: 'Dart',
  elixir: 'Elixir',
  ocaml: 'OCaml',
  sql: 'SQL',
  solidity: 'Solidity',
  zig: 'Zig',
  vue: 'Vue',
  svelte: 'Svelte',
  embedded_template: 'ERB',
  [PLAIN_TEXT_LANGUAGE_ID]: 'Plain Text',
  dockerfile: 'Dockerfile',
  gitattributes: 'Git Attributes',
  graphql: 'GraphQL',
  gitignore: 'Git Ignore',
  scss: 'SCSS',
  sass: 'Sass',
  less: 'Less',
  xml: 'XML',
  restructuredtext: 'reStructuredText',
  fsharp: 'F#',
  clojure: 'Clojure',
  scheme: 'Scheme',
  shell: 'Shell',
  powershell: 'PowerShell',
  batch: 'Batch',
  ini: 'INI',
  protobuf: 'Protocol Buffers',
  ql: 'QL',
  r: 'R',
  terraform: 'Terraform',
  elm: 'Elm',
  elisp: 'Emacs Lisp',
  lockfile: 'Lockfile',
}

export function getLanguageDisplayName(languageId: string): string {
  return (
    (Object.hasOwn(LANGUAGE_DISPLAY_NAMES, languageId) &&
      LANGUAGE_DISPLAY_NAMES[languageId as LanguageId]) ||
    languageId
  )
}

/** The picker's choices: every named language with a grammar, plus Plain Text to turn colour off. */
export function getAllLanguages(): Array<{ id: string; displayName: string }> {
  return Object.entries(LANGUAGE_DISPLAY_NAMES)
    .flatMap(([id, displayName]) =>
      id === PLAIN_TEXT_LANGUAGE_ID || hasGrammar(id) ? [{ id, displayName }] : [],
    )
    .sort((a, b) => a.displayName.localeCompare(b.displayName))
}

export function getLanguageIdFromPath(filePath: string): string | null {
  if (isAngularTemplatePath(filePath)) {
    return ANGULAR_TEMPLATE_LANGUAGE_ID
  }

  const fileName = filePath.split('/').pop()?.toLowerCase() || ''
  const normalizedPath = filePath.replace(/\\/g, '/').toLowerCase()
  if (normalizedPath.endsWith('/.git/info/exclude')) {
    return 'gitignore' satisfies LanguageId
  }
  if (normalizedPath.endsWith('/.git/info/attributes')) {
    return 'gitattributes' satisfies LanguageId
  }

  if (isEnvFileName(fileName)) {
    return 'dotenv' satisfies LanguageId
  }

  const fromFilename = FILENAME_TO_LANGUAGE[fileName]
  if (fromFilename) {
    return fromFilename
  }

  const extension = filePath.split('.').pop()?.toLowerCase() || ''
  return getLanguageIdFromExtension(extension)
}
