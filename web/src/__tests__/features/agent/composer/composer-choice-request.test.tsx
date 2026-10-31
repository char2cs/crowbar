import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { ChoiceRequest } from '@/features/agent/composer/composer-choice-request'
import { requestView, type AskRequest } from '@/features/agent/composer/lib/ask-request'

function show({ request }: { request: AskRequest }) {
  return render(<ChoiceRequest view={requestView(request)} expanded={false} onExpand={() => {}} />)
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
