import { HighlightStyle, syntaxHighlighting, syntaxTree } from '@codemirror/language'
import { tags as t } from '@lezer/highlight'
import { Decoration, type DecorationSet, EditorView } from '@codemirror/view'
import { type EditorState, RangeSetBuilder, StateField } from '@codemirror/state'
import { WIDGET_ID_RE } from '../types'

// order 0 = line decorations (must precede replace decos at the same position),
// order 1 = replace decorations.
interface Entry {
  from: number
  to: number
  order: number
  deco: Decoration
}

// Style fenced code blocks (```lang … ```) as monospace panels and hide the
// ``` fence lines (Obsidian-style live preview). Hiding is cursor-aware: while
// the cursor is inside a block the fences stay visible so they can be edited;
// otherwise (and always in the read-only history) they're hidden.
//
// This MUST be a StateField, not a ViewPlugin: the fence-hiding replace
// decorations span line breaks, and CodeMirror forbids line-break-replacing
// decorations from plugins ("Decorations that replace line breaks may not be
// specified via plugins"). Widget fences (`widget-id:`) are left to widget-ext.
function buildCodeBlockDecorations(state: EditorState): DecorationSet {
  const cursorLine = state.doc.lineAt(state.selection.main.head).number
  const docLen = state.doc.length
  const entries: Entry[] = []

  const lineDeco = (pos: number, first: boolean, last: boolean) => {
    let cls = 'cm-code-block'
    if (first) cls += ' cm-code-block-first'
    if (last) cls += ' cm-code-block-last'
    entries.push({ from: pos, to: pos, order: 0, deco: Decoration.line({ class: cls }) })
  }

  syntaxTree(state).iterate({
    enter: (node) => {
      if (node.name !== 'FencedCode') return
      const startLine = state.doc.lineAt(node.from)
      if (WIDGET_ID_RE.test(startLine.text)) return
      const endLine = state.doc.lineAt(node.to)
      const cursorInside = cursorLine >= startLine.number && cursorLine <= endLine.number

      if (cursorInside) {
        // Editing: keep fences visible, just tint every line.
        for (let ln = startLine.number; ln <= endLine.number; ln++) {
          lineDeco(state.doc.line(ln).from, ln === startLine.number, ln === endLine.number)
        }
        return
      }

      const contentFirst = startLine.number + 1
      const contentLast = endLine.number - 1

      // Empty block (```lang immediately followed by ```): hide it entirely.
      if (contentFirst > contentLast) {
        entries.push({
          from: startLine.from,
          to: Math.min(endLine.to + 1, docLen),
          order: 1,
          deco: Decoration.replace({}),
        })
        return
      }

      const firstContent = state.doc.line(contentFirst)
      const lastContent = state.doc.line(contentLast)

      // Hide opening "```lang\n" (merges into the first content line) …
      entries.push({
        from: startLine.from,
        to: firstContent.from,
        order: 1,
        deco: Decoration.replace({}),
      })
      // … and the closing "\n```" (keeps the fence line's trailing newline so
      // following text stays on its own line).
      entries.push({ from: lastContent.to, to: endLine.to, order: 1, deco: Decoration.replace({}) })

      for (let ln = contentFirst; ln <= contentLast; ln++) {
        // The opening replace merges the fence line into the first content
        // line, so the first visual line starts at startLine.from.
        const pos = ln === contentFirst ? startLine.from : state.doc.line(ln).from
        lineDeco(pos, ln === contentFirst, ln === contentLast)
      }
    },
  })

  entries.sort((a, b) => a.from - b.from || a.order - b.order)
  const builder = new RangeSetBuilder<Decoration>()
  for (const { from, to, deco } of entries) {
    builder.add(from, to, deco)
  }
  return builder.finish()
}

const codeBlockField = StateField.define<DecorationSet>({
  create: (state) => buildCodeBlockDecorations(state),
  update: (deco, tr) =>
    tr.docChanged || tr.selection ? buildCodeBlockDecorations(tr.state) : deco,
  provide: (f) => EditorView.decorations.from(f),
})

// Code-scoped token colours, mapped to the --syntax-* theme vars. Markdown's own
// tags (heading, link, strong, emphasis) are deliberately omitted so document
// text keeps its plain look — only nested code-block tokens get coloured.
const codeHighlightStyle = HighlightStyle.define([
  { tag: t.keyword, color: 'var(--syntax-keyword)' },
  {
    tag: [t.name, t.deleted, t.character, t.propertyName, t.macroName],
    color: 'var(--syntax-variable)',
  },
  { tag: [t.function(t.variableName), t.labelName], color: 'var(--syntax-function)' },
  { tag: [t.color, t.constant(t.name), t.standard(t.name)], color: 'var(--syntax-constant)' },
  { tag: [t.definition(t.name), t.separator], color: 'var(--syntax-variable)' },
  {
    tag: [
      t.typeName,
      t.className,
      t.number,
      t.changed,
      t.annotation,
      t.modifier,
      t.self,
      t.namespace,
    ],
    color: 'var(--syntax-type)',
  },
  {
    tag: [t.operator, t.operatorKeyword, t.url, t.escape, t.regexp, t.special(t.string)],
    color: 'var(--syntax-operator)',
  },
  { tag: [t.meta, t.comment], color: 'var(--syntax-comment)', fontStyle: 'italic' },
  { tag: [t.atom, t.bool, t.special(t.variableName)], color: 'var(--syntax-constant)' },
  { tag: [t.processingInstruction, t.string, t.inserted], color: 'var(--syntax-string)' },
  { tag: t.invalid, color: 'var(--syntax-invalid)' },
])

const codeBlockTheme = EditorView.theme({
  '.cm-code-block': {
    fontFamily: 'var(--font-editor, monospace)',
    fontSize: '0.9em',
    background: 'var(--code-highlight)',
    // Disable JetBrains Mono ligatures so "!=" stays "!=" (not "≠"), "=>" stays "=>", etc.
    fontVariantLigatures: 'none',
    fontFeatureSettings: '"liga" 0, "calt" 0',
  },
  '.cm-code-block-first': {
    paddingTop: '8px',
    borderTopLeftRadius: '8px',
    borderTopRightRadius: '8px',
  },
  '.cm-code-block-last': {
    paddingBottom: '8px',
    borderBottomLeftRadius: '8px',
    borderBottomRightRadius: '8px',
  },
})

export function codeBlockExt() {
  return [codeBlockField, codeBlockTheme, syntaxHighlighting(codeHighlightStyle)]
}
