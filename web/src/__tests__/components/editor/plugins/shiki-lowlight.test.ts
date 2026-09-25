import { describe, expect, it } from 'vitest'
import {
  classesForScopes,
  onShikiLanguageReady,
  shikiLowlight,
  tokenizeToHast,
} from '@/components/editor/plugins/shiki-lowlight'

describe('classesForScopes', () => {
  it('maps the innermost matching scope onto the hljs class the code block colors', () => {
    expect(classesForScopes(['source.go', 'keyword.control.go'])).toEqual(['hljs-keyword'])
    expect(classesForScopes(['source.ts', 'string.quoted.double.ts'])).toEqual(['hljs-string'])
    expect(classesForScopes(['source.ts', 'meta.function', 'entity.name.function.ts'])).toEqual([
      'hljs-title',
      'function_',
    ])
    expect(classesForScopes(['source.ts', 'comment.line.double-slash.ts'])).toEqual([
      'hljs-comment',
    ])
  })

  it('leaves an unclassified token plain', () => {
    expect(classesForScopes(['source.ts', 'variable.other.readwrite.ts'])).toEqual([])
  })
})

describe('tokenizeToHast', () => {
  it('emits spans for classified tokens, plain text otherwise, and a newline between lines', () => {
    const grammar = {
      tokenizeLine: (line: string) => ({
        tokens: line.startsWith('let')
          ? [
              { startIndex: 0, endIndex: 3, scopes: ['source', 'storage.type'] },
              { startIndex: 3, endIndex: line.length, scopes: ['source'] },
            ]
          : [{ startIndex: 0, endIndex: line.length, scopes: ['source', 'comment.line'] }],
        ruleStack: null as never,
      }),
    }

    expect(tokenizeToHast(grammar, 'let x\n// hi')).toEqual({
      type: 'root',
      children: [
        {
          type: 'element',
          tagName: 'span',
          properties: { className: ['hljs-keyword'] },
          children: [{ type: 'text', value: 'let' }],
        },
        { type: 'text', value: ' x' },
        { type: 'text', value: '\n' },
        {
          type: 'element',
          tagName: 'span',
          properties: { className: ['hljs-comment'] },
          children: [{ type: 'text', value: '// hi' }],
        },
      ],
    })
  })
})

describe('shikiLowlight', () => {
  it('is plain until the grammar loads, announces it, then highlights with the real grammar', async () => {
    const ready = new Promise<string>((resolve) => {
      const off = onShikiLanguageReady((lang) => {
        off()
        resolve(lang)
      })
    })

    expect(shikiLowlight.highlight('javascript', 'const x = 1')).toEqual({
      type: 'root',
      children: [],
    })
    expect(await ready).toBe('javascript')

    const hast = shikiLowlight.highlight('javascript', 'const x = "s" // c')
    const classes = hast.children.flatMap((c) =>
      c.type === 'element' ? c.properties.className : [],
    )
    expect(classes).toEqual(expect.arrayContaining(['hljs-keyword', 'hljs-string', 'hljs-comment']))
    expect(shikiLowlight.listLanguages()).toContain('javascript')
  }, 30_000)

  it('stays plain for a language shiki has no grammar for', () => {
    expect(shikiLowlight.highlight('not-a-language', 'x').children).toEqual([])
    expect(shikiLowlight.highlightAuto('x').children).toEqual([])
  })
})
