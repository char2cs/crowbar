import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { ChoiceRequest } from '@/features/agent/composer/composer-choice-request'
import { requestView, type AskRequest } from '@/features/agent/composer/lib/ask-request'

function show({ request, expanded = false }: { request: AskRequest; expanded?: boolean }) {
  return render(
    <ChoiceRequest view={requestView(request)} expanded={expanded} onExpand={() => {}} />,
  )
}

describe('ChoiceRequest request body', () => {
  it('shows the command and its description for a command approval', () => {
    show({
      request: {
        toolName: 'Bash',
        toolInput: { command: 'rm -rf build', description: 'Clean the build' },
      },
    })
    const body = screen.getByTestId('agent-choice-request')
    expect(body).toHaveTextContent('rm -rf build')
    expect(body).toHaveTextContent('Clean the build')
  })

  it('shows the server and formatted args for an MCP approval', () => {
    show({
      request: {
        toolName: 'mcp__echo__shout',
        mcpServer: 'echo',
        toolInput: { text: 'hi', loud: true },
      },
    })
    const body = screen.getByTestId('agent-choice-request')
    expect(body).toHaveTextContent('echo · shout')
    expect(body).toHaveTextContent('text: hi')
    expect(body).toHaveTextContent('loud: true')
  })

  it('shows a truncated tool input as text, not as parsed arguments', () => {
    const text = '{"command":"cat a very long'
    show({ request: { toolName: 'Bash', toolInput: text, truncated: true } })
    const body = screen.getByTestId('agent-choice-request')
    expect(body).toHaveTextContent(text)
    expect(body).toHaveTextContent('Input truncated')
  })
})

describe('ChoiceRequest with a very long input', () => {
  const command = 'x'.repeat(1 << 20)

  it('draws a 1 MB single line bounded, and quickly', () => {
    const start = performance.now()
    show({ request: { toolName: 'Bash', toolInput: { command } } })
    // Drawn whole, this took ~300 ms in escaping alone; bounded it is a few.
    expect(performance.now() - start).toBeLessThan(150)
    const code = screen.getByTestId('agent-choice-request').querySelector('pre')!
    expect(code.textContent!.length).toBeLessThan(300)
    expect(screen.getByRole('button', { name: 'Show all 1048576 characters' })).toBeInTheDocument()
  })

  it('draws all of it once asked to', () => {
    show({ request: { toolName: 'Bash', toolInput: { command } }, expanded: true })
    const code = screen.getByTestId('agent-choice-request').querySelector('pre')!
    expect(code.textContent).toBe(`${command}\n`)
    // It scrolls inside its own bound, so it must be reachable by keyboard.
    expect(code).toHaveAttribute('tabindex', '0')
  })

  it('shows every MCP argument and the whole of each once asked to', () => {
    const toolInput = Object.fromEntries(
      Array.from({ length: 14 }, (_, i) => [`k${i}`, i === 0 ? 'z'.repeat(500) : i]),
    )
    const request = { toolName: 'mcp__echo__shout', mcpServer: 'echo', toolInput }
    const { unmount } = show({ request })
    const body = screen.getByTestId('agent-choice-request')
    expect(body).not.toHaveTextContent('k13')
    expect(body.textContent).not.toContain('z'.repeat(500))
    unmount()
    show({ request, expanded: true })
    expect(screen.getByTestId('agent-choice-request')).toHaveTextContent('k13: 13')
    expect(screen.getByTestId('agent-choice-request').textContent).toContain('z'.repeat(500))
  })
})

describe('ChoiceRequest shows a command as it is', () => {
  it('writes bidi, control and zero-width characters out visibly', () => {
    const command = 'echo build done \u202E; rm -rf ~ #\u202C\u200B\u0007'
    show({ request: { toolName: 'Bash', toolInput: { command } } })
    const body = screen.getByTestId('agent-choice-request')
    for (const raw of ['\u202E', '\u202C', '\u200B', '\u0007'])
      expect(body.textContent).not.toContain(raw)
    expect(body).toHaveTextContent('\\u{202E}')
    expect(body).toHaveTextContent('\\u{200B}')
    expect(body).toHaveTextContent('\\u{7}')
  })

  it.each([
    ['line and paragraph separators', 0x2028],
    ['deprecated format controls', 0x206a],
    ['a soft hyphen', 0x00ad],
    ['a combining grapheme joiner', 0x034f],
    ['the Mongolian vowel separator', 0x180e],
    ['Hangul fillers', 0x115f],
    ['the Hangul jungseong filler', 0x1160],
    ['the Hangul compatibility filler', 0x3164],
    ['the halfwidth Hangul filler', 0xffa0],
    ['the blank Braille pattern', 0x2800],
    ['variation selectors', 0xfe0f],
    ['supplementary variation selectors', 0xe0100],
    ['tag characters', 0xe0041],
    ['interlinear annotation and object replacement', 0xfffc],
    ['a Khmer inherent vowel', 0x17b4],
    ['the other Khmer inherent vowel', 0x17b5],
    ['a Mongolian free variation selector', 0x180b],
    ['the second Mongolian free variation selector', 0x180c],
    ['the third Mongolian free variation selector', 0x180d],
    ['the fourth Mongolian free variation selector', 0x180f],
  ])('writes out %s', (_, cp) => {
    const raw = String.fromCodePoint(cp)
    show({ request: { toolName: 'Bash', toolInput: { command: `a${raw}b` } } })
    const body = screen.getByTestId('agent-choice-request')
    expect(body.textContent).not.toContain(raw)
    expect(body).toHaveTextContent(`a\\u{${cp.toString(16).toUpperCase()}}b`)
  })

  it('draws a CRLF line break as a break, and a lone carriage return written out', () => {
    show({ request: { toolName: 'Bash', toolInput: { command: 'one\r\ntwo\rthree' } } })
    const code = screen.getByTestId('agent-choice-request').querySelector('pre')!
    expect(code.textContent).toBe('one\ntwo\\u{D}three\n')
  })

  it('writes out a hidden character in an MCP server name', () => {
    show({
      request: { toolName: 'mcp__e\u202Ecal__x', mcpServer: 'e\u202Ecal', toolInput: {} },
    })
    const server = screen.getByTestId('agent-mcp-server')
    expect(server.textContent).toBe('e\\u{202E}cal')
  })

  it('escapes MCP args and edit previews the same way', () => {
    show({
      request: {
        toolName: 'mcp__echo__shout',
        mcpServer: 'echo',
        toolInput: { text: 'a\u202Eb' },
      },
    })
    expect(screen.getByTestId('agent-choice-request')).toHaveTextContent('a\\u{202E}b')
  })
})

describe('ChoiceRequest edit and write bodies', () => {
  const body = () => screen.getByTestId('agent-choice-request')

  it('draws no removed line for an edit with nothing to remove, and says replace-all', () => {
    show({
      request: {
        toolName: 'Edit',
        toolInput: { file_path: 'a.ts', old_string: '', new_string: 'x', replace_all: true },
      },
    })
    expect(body().textContent).not.toContain('- ')
    expect(body()).toHaveTextContent('+ x')
    expect(body()).toHaveTextContent('Every occurrence')
  })

  it('shows a write’s path and its content, bounded', () => {
    const content = Array.from({ length: 15 }, (_, i) => `l${i}`).join('\n')
    show({ request: { toolName: 'Write', toolInput: { file_path: 'new.ts', content } } })
    expect(body()).toHaveTextContent('new.ts')
    expect(body()).toHaveTextContent('l11')
    expect(body()).not.toHaveTextContent('l12')
    expect(screen.getByRole('button', { name: 'Show all 15 lines' })).toBeInTheDocument()
  })

  it('shows every edit of a multi-edit', () => {
    show({
      request: {
        toolName: 'MultiEdit',
        toolInput: {
          file_path: 'm.ts',
          edits: [
            { old_string: 'a', new_string: 'b' },
            { old_string: 'c', new_string: 'd' },
          ],
        },
      },
    })
    expect(body()).toHaveTextContent('- a')
    expect(body()).toHaveTextContent('+ d')
  })

  it('shows a notebook cell’s new source', () => {
    show({
      request: {
        toolName: 'NotebookEdit',
        toolInput: { notebook_path: 'n.ipynb', new_source: 'print(1)' },
      },
    })
    expect(body()).toHaveTextContent('n.ipynb')
    expect(body()).toHaveTextContent('print(1)')
  })

  it('says the input is not shown rather than drawing nothing', () => {
    show({ request: { toolName: 'Mystery', toolTarget: 'thing', toolInput: { x: 1 } } })
    expect(body()).toHaveTextContent('thing')
    expect(body()).toHaveTextContent('Input not shown')
  })
})
