import { LanguageDescription, LanguageSupport, StreamLanguage } from '@codemirror/language'
import { javascript } from '@codemirror/lang-javascript'
import { python } from '@codemirror/lang-python'
import { go } from '@codemirror/lang-go'
import { json } from '@codemirror/lang-json'
import { shell } from '@codemirror/legacy-modes/mode/shell'

// Languages offered in the toolbar "Insert ▸ Code block" menu. Passed to
// markdown() so fenced blocks (```go …```) are parsed by the matching grammar,
// which lets syntaxHighlighting colour their tokens. `plain` is intentionally
// absent — it means "no language", i.e. no nested parsing.
export const codeLanguages: LanguageDescription[] = [
  LanguageDescription.of({
    name: 'typescript',
    alias: ['ts', 'tsx'],
    load: async () => javascript({ typescript: true, jsx: true }),
  }),
  LanguageDescription.of({
    name: 'javascript',
    alias: ['js', 'jsx'],
    load: async () => javascript({ jsx: true }),
  }),
  LanguageDescription.of({
    name: 'python',
    alias: ['py'],
    load: async () => python(),
  }),
  LanguageDescription.of({
    name: 'go',
    load: async () => go(),
  }),
  LanguageDescription.of({
    name: 'json',
    load: async () => json(),
  }),
  LanguageDescription.of({
    name: 'shell',
    alias: ['sh', 'bash', 'zsh'],
    load: async () => new LanguageSupport(StreamLanguage.define(shell)),
  }),
]
