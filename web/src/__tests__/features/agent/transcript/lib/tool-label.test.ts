import { describe, expect, it } from 'vitest'
import type { ToolCallBody } from '@/features/agent/api/chat-rows'
import {
  commandProgramName,
  diffStat,
  formatDuration,
  labelText,
  liveLabel,
} from '@/features/agent/transcript/lib/tool-label'
import { toolFamily } from '@/features/agent/transcript/lib/tool-family'
import type { ToolItem } from '@/features/agent/stores/lib/transcript-items'

function tool(body: Partial<ToolCallBody>, status: ToolItem['status'] = 'running'): ToolItem {
  const full = { toolId: 'tool-1', phase: 'pre' as const, ...body }
  return {
    kind: 'tool',
    id: 't:tool-1',
    seq: 1,
    at: '',
    toolId: 'tool-1',
    scope: '',
    body: full,
    status,
    family: toolFamily(full),
  }
}

describe('tool labels', () => {
  it('symptom 8: labels a command by its title, then its unwrapped program name', () => {
    const titled = tool({ kind: 'execute', title: 'Run the tests', target: "bash -lc 'npm test'" })
    expect(liveLabel(titled)).toBe('Running Run the tests')
    const untitled = tool({ kind: 'execute', target: "bash -lc 'npm test'" })
    expect(liveLabel(untitled)).toBe('Running npm test')
    expect(labelText({ ...untitled, status: 'ok' })).toBe('Ran npm test')
  })

  it('unwraps shells, sudo and env assignments to the program', () => {
    expect(commandProgramName("bash -lc 'npm test'")).toBe('npm test')
    expect(commandProgramName('sudo -E env CI=1 /usr/bin/make check')).toBe('make check')
    expect(commandProgramName('sh -c "go vet ./..."\nsecond line')).toBe('go vet ./...')
    expect(commandProgramName('')).toBe('')
  })

  it('labels a read live and settled with its file and line', () => {
    const read = tool({
      kind: 'read',
      locations: [{ path: '/home/me/repo/src/notes.ts', line: 10 }],
    })
    expect(liveLabel(read)).toBe('Reading repo/src/notes.ts L10')
    expect(labelText({ ...read, status: 'ok' })).toBe('Read repo/src/notes.ts L10')
  })

  it('quotes a search pattern and names a fetched host', () => {
    expect(liveLabel(tool({ kind: 'search', target: 'TODO' }))).toBe('Searching for "TODO"')
    expect(labelText(tool({ kind: 'search', target: 'TODO' }, 'ok'))).toBe('Searched "TODO"')
    const fetch = tool({ kind: 'fetch', target: 'https://docs.rs/serde/latest' })
    expect(liveLabel(fetch)).toBe('Fetching docs.rs')
    expect(labelText({ ...fetch, status: 'ok' })).toBe('Fetched docs.rs/…')
  })

  it('says a new file is created, not edited', () => {
    const write = tool({
      kind: 'edit',
      target: '/r/notes.md',
      diff: '--- /dev/null\n+++ b/notes.md\n',
    })
    expect(liveLabel(write)).toBe('Creating r/notes.md')
    expect(labelText({ ...write, status: 'ok' })).toBe('Created r/notes.md')
    expect(labelText({ ...write, status: 'declined' })).toBe('Create r/notes.md')
    expect(labelText(tool({ kind: 'edit', target: 'a.md', diff: '--- a/a.md\n' }, 'ok'))).toBe(
      'Edited a.md',
    )
  })

  it('falls back to the humanized name when a tool names no target', () => {
    expect(labelText(tool({ name: 'NotebookRead' }, 'ok'))).toBe('Notebook read')
    expect(labelText(tool({}, 'ok'))).toBe('Tool')
  })

  it('symptom 8: names the MCP server and summarizes the first two scalar args', () => {
    const call = tool({
      kind: 'mcp',
      name: 'mcp__echo__shout',
      mcpServer: 'echo',
      mcpTool: 'shout',
    })
    expect(liveLabel(call)).toBe('echo · shout')
    expect(labelText({ ...call, status: 'ok' })).toBe('echo · shout')
  })

  it('counts the lines a diff adds and removes', () => {
    const diff = '--- a/x\n+++ b/x\n@@ -1,2 +1,2 @@\n-old\n+new\n+more\n same'
    expect(diffStat(diff)).toEqual({ added: 2, removed: 1 })
  })
})

describe('formatDuration', () => {
  it.each([
    [undefined, ''],
    [0, ''],
    [-5, ''],
    // Sub-second work in milliseconds: "0s" reads as "nothing happened".
    [37, '37ms'],
    [1500, '1.5s'],
    [59_000, '59.0s'],
    [60_000, '1m'],
    [90_000, '1m 30s'],
  ])('renders %s as %s', (ms, expected) => {
    expect(formatDuration(ms)).toBe(expected)
  })
})
