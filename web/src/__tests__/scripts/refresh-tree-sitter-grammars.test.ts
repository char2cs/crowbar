import { describe, expect, it } from 'vitest'

// @ts-expect-error — plain .mjs maintenance script, no type declarations
import { stripSetDirectives } from '../../../scripts/refresh-tree-sitter-grammars.mjs'

/**
 * `#set!` directives with three arguments are rejected outright by
 * web-tree-sitter ("Wrong number of arguments to `#set!` predicate"), and a
 * single bad directive fails the WHOLE query, silently disabling syntax
 * highlighting for that language. Upstream editor query sets use them freely,
 * so the refresh script strips them. These cases pin that behaviour.
 */
describe('stripSetDirectives', () => {
  it('removes a three-argument #set! directive that web-tree-sitter rejects', () => {
    const source = `((comment) @comment
 (#set! @comment "priority" 105))`
    expect(stripSetDirectives(source)).not.toContain('#set!')
  })

  it('keeps the surrounding pattern and its captures intact', () => {
    const result = stripSetDirectives(`((identifier) @variable
 (#set! @variable "priority" 105))`)
    expect(result).toContain('(identifier) @variable')
  })

  it('leaves queries without directives byte-identical', () => {
    const source = '(function_definition name: (identifier) @function)\n'
    expect(stripSetDirectives(source)).toBe(source)
  })

  it('removes every directive when several are present', () => {
    const result = stripSetDirectives(`(a) @x
 (#set! @x "k" 1)
(b) @y
 (#set! @y "k" 2)`)
    expect(result).not.toContain('#set!')
    expect(result).toContain('(a) @x')
    expect(result).toContain('(b) @y')
  })

  it('does not stop at a parenthesis inside a string argument', () => {
    // A naive scanner that counts parens without tracking strings would end the
    // directive at the ")" inside the literal and leave a stray tail behind.
    const result = stripSetDirectives('((x) @c (#set! @c "text" "a) b"))\n(y) @d')
    expect(result).not.toContain('#set!')
    expect(result).not.toContain('a) b')
    expect(result).toContain('(y) @d')
  })

  it('preserves other predicates, which are still meaningful to the query', () => {
    const result = stripSetDirectives(`((identifier) @constant
 (#match? @constant "^[A-Z]+$")
 (#set! @constant "priority" 105))`)
    expect(result).toContain('#match?')
    expect(result).not.toContain('#set!')
  })
})
