import { describe, it, expect, vi } from 'vitest'
import { render } from '@testing-library/react'
import type { HighlightToken } from '@/features/editor/lib/wasm-parser/types'
import type { TokenNode } from 'react-diff-view'
import {
  highlightTokensToTokenNodes,
  renderTreeSitterToken,
} from '@/features/git/lib/render-tree-sitter-token'

// ──────────────────────────────────────────────
// highlightTokensToTokenNodes
// ──────────────────────────────────────────────

function makeToken(
  type: string,
  startCol: number,
  endCol: number,
): HighlightToken {
  return {
    type,
    startIndex: startCol,
    endIndex: endCol,
    startPosition: { row: 0, column: startCol },
    endPosition: { row: 0, column: endCol },
  }
}

describe('highlightTokensToTokenNodes', () => {
  it('converts a keyword token at the start of a line with a plain-text tail', () => {
    const content = 'const x = 1'
    const tokens: HighlightToken[] = [makeToken('token-keyword', 0, 5)]

    const nodes = highlightTokensToTokenNodes(content, tokens)

    // Expect: syntax node for "const", plain text for " x = 1"
    expect(nodes).toHaveLength(2)

    const kw = nodes[0]!
    expect(kw.type).toBe('syntax')
    expect(kw.className).toBe('token-keyword')
    expect(kw.value).toBe('const')

    const plain = nodes[1]!
    expect(plain.type).toBe('text')
    expect(plain.value).toBe(' x = 1')
  })

  it('inserts a plain-text gap between two tokens', () => {
    // "function foo" — keyword at 0-8, identifier at 9-12
    const content = 'function foo'
    const tokens: HighlightToken[] = [
      makeToken('token-keyword', 0, 8),
      makeToken('token-identifier', 9, 12),
    ]

    const nodes = highlightTokensToTokenNodes(content, tokens)

    // keyword "function", gap " ", identifier "foo"
    expect(nodes).toHaveLength(3)
    expect(nodes[0]).toMatchObject({ type: 'syntax', className: 'token-keyword', value: 'function' })
    expect(nodes[1]).toMatchObject({ type: 'text', value: ' ' })
    expect(nodes[2]).toMatchObject({ type: 'syntax', className: 'token-identifier', value: 'foo' })
  })

  it('returns a single plain-text node when tokens array is empty (graceful degrade)', () => {
    const content = 'plain text line'
    const nodes = highlightTokensToTokenNodes(content, [])

    expect(nodes).toHaveLength(1)
    expect(nodes[0]).toMatchObject({ type: 'text', value: 'plain text line' })
  })

  it('handles a token that spans the entire line (no gap nodes)', () => {
    const content = 'hello'
    const tokens: HighlightToken[] = [makeToken('token-string', 0, 5)]

    const nodes = highlightTokensToTokenNodes(content, tokens)

    expect(nodes).toHaveLength(1)
    expect(nodes[0]).toMatchObject({ type: 'syntax', className: 'token-string', value: 'hello' })
  })

  it('handles empty content with empty tokens', () => {
    const nodes = highlightTokensToTokenNodes('', [])
    // single text node but with empty value
    expect(nodes).toHaveLength(1)
    expect(nodes[0]).toMatchObject({ type: 'text', value: '' })
  })
})

// ──────────────────────────────────────────────
// renderTreeSitterToken
// ──────────────────────────────────────────────

describe('renderTreeSitterToken', () => {
  it('renders a syntax node as <span className={token.className}>', () => {
    const token: TokenNode = { type: 'syntax', className: 'token-keyword', value: 'const' }
    const renderDefault = vi.fn()

    const result = renderTreeSitterToken(token, renderDefault, 0)
    const { container } = render(<>{result}</>)

    const span = container.querySelector('span')
    expect(span).not.toBeNull()
    expect(span!.className).toBe('token-keyword')
    expect(span!.textContent).toBe('const')
    expect(renderDefault).not.toHaveBeenCalled()
  })

  it('delegates non-syntax tokens to renderDefault', () => {
    const token: TokenNode = { type: 'text', value: 'plain' }
    const renderDefault = vi.fn(() => <span>plain</span>)

    renderTreeSitterToken(token, renderDefault, 1)

    expect(renderDefault).toHaveBeenCalledWith(token, 1)
  })

  it('delegates mark tokens to renderDefault', () => {
    const token: TokenNode = { type: 'mark', markType: 'delete', value: 'old' }
    const renderDefault = vi.fn()

    renderTreeSitterToken(token, renderDefault, 2)

    expect(renderDefault).toHaveBeenCalledWith(token, 2)
  })

  it('delegates syntax tokens without className to renderDefault', () => {
    // Edge case: a syntax node with no className should fall through to default
    const token: TokenNode = { type: 'syntax', value: 'x' }
    const renderDefault = vi.fn()

    renderTreeSitterToken(token, renderDefault, 3)

    expect(renderDefault).toHaveBeenCalledWith(token, 3)
  })
})
