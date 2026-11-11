import { describe, expect, it } from 'vitest'
import {
  foldCode,
  foldedCue,
  requestView,
  visibleParts,
} from '@/features/agent/composer/lib/ask-request'

const folded = (command: string) =>
  foldCode(requestView({ toolName: 'Bash', toolInput: { command } }).code)

describe('requestView', () => {
  it('shows the plan a plan approval asks about', () => {
    const view = requestView({
      toolName: 'ExitPlanMode',
      toolInput: { plan: '# Plan\n1. Create a.txt', planFilePath: '/h/plans/p.md' },
    })
    expect(view.markdown).toBe('# Plan\n1. Create a.txt')
    expect(view.code).toEqual([])
    expect(view.note).toBeUndefined()
  })

  it('says why the agent asks when it shows no input of the call', () => {
    const why = 'Allow the exact curl command to access example.com outside the sandbox?'
    const view = requestView({ toolName: 'commandExecution', toolTarget: 'curl example.com', why })
    expect(view).toEqual({ head: 'curl example.com', code: [], note: why })
  })

  it('heads a file change with the path its call is named by, not the raw input path', () => {
    const view = requestView({
      toolName: 'Edit',
      toolTarget: 'src/a.go',
      toolInput: { file_path: '/ws/repo/src/a.go', old_string: 'a', new_string: 'b' },
    })
    expect(view.head).toBe('src/a.go')
  })
})

describe('foldedCue', () => {
  it('counts the characters cut from long lines as well as the lines left out', () => {
    const command = Array.from({ length: 13 }, () => 'y'.repeat(300)).join('\n')
    expect(foldedCue(folded(command))).toBe('1 more line and 720 more characters not shown')
  })

  it('says nothing when the card leaves nothing out', () => {
    expect(foldedCue(folded('ls'))).toBe('')
  })
})

describe('foldCode', () => {
  it('never cuts a line between the halves of a surrogate pair', () => {
    const line = `${'a'.repeat(239)}😀tail`
    const [shown] = folded(line).lines
    expect(shown.text).toBe('a'.repeat(239))
    expect(shown.cut).toBe(true)
  })
})

describe('visibleParts', () => {
  // Every code point Unicode says draws as nothing or controls layout; TAB and
  // LF draw as themselves, and a CRLF's CR is dropped before this.
  const INVISIBLE = /[\p{Default_Ignorable_Code_Point}\p{Cf}\p{Zl}\p{Zp}\p{Cc}]/u
  const invisible: number[] = []
  for (let cp = 0; cp <= 0x10ffff; cp++) {
    if (cp >= 0xd800 && cp <= 0xdfff) continue
    if (cp !== 0x09 && cp !== 0x0a && INVISIBLE.test(String.fromCodePoint(cp))) invisible.push(cp)
  }

  it('writes out every invisible code point Unicode names', () => {
    const drawn = visibleParts(String.fromCodePoint(...invisible))
    const raw = drawn.filter((part) => !part.escaped).map((part) => part.text)
    expect(raw.join('')).toBe('')
    expect(drawn).toHaveLength(invisible.length)
  })

  it('leaves a tab and a line feed as they are', () => {
    expect(visibleParts('a\tb\nc')).toEqual([{ text: 'a\tb\nc', escaped: false }])
  })
})

describe('requestView for an MCP call', () => {
  it('counts each line of a multi-line argument as a line of its own', () => {
    const view = requestView({
      toolName: 'mcp__echo__shout',
      mcpServer: 'echo',
      toolInput: { text: 'one\r\ntwo\nthree', loud: true },
    })
    expect(view.code.map((line) => [line.key, line.text])).toEqual([
      ['text', 'one'],
      [undefined, 'two'],
      [undefined, 'three'],
      ['loud', 'true'],
    ])
  })
})
