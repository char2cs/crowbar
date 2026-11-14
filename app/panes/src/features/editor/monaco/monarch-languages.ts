// `editor.api`, not bare 'monaco-editor': that entry statically imports every built-in language.
import { languages } from 'monaco-editor/esm/vs/editor/editor.api.js'

// Matched both as a bare filename and as an extension: `.gitignore` is either.
const IGNORE_FILES = [
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
]

function ensureLanguage(id: string, extensions: string[], aliases: string[], filenames?: string[]) {
  if (languages.getLanguages().some((language) => language.id === id)) return
  languages.register({ id, extensions, aliases, filenames })
}

/** Registers the languages monaco ships no grammar for, each with a small Monarch tokenizer.
 *  Synchronous, so they need no loader in language-contributions.ts. */
export function registerMonarchLanguages(): void {
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

  ensureLanguage('gitignore', IGNORE_FILES, ['Git Ignore', 'gitignore', 'ignore'], IGNORE_FILES)
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
}
