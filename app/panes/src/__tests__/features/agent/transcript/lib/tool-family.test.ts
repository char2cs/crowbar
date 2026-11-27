import { describe, expect, it } from 'vitest'
import { toolFamily } from '@/features/agent/transcript/lib/tool-family'

describe('toolFamily', () => {
  it('takes the family from the kind the descriptor mapped', () => {
    expect(toolFamily({ kind: 'read', name: 'Read' })).toBe('read')
    expect(toolFamily({ kind: 'execute', name: 'Bash' })).toBe('execute')
    expect(toolFamily({ kind: 'subagent', name: 'Agent' })).toBe('subagent')
    expect(toolFamily({ kind: 'plan', name: 'TaskUpdate' })).toBe('plan')
  })

  it('reads an MCP call by its kind, server or name', () => {
    expect(toolFamily({ kind: 'mcp', name: 'mcp__echo__shout' })).toBe('mcp')
    expect(toolFamily({ name: 'shout', mcpServer: 'echo' })).toBe('mcp')
    expect(toolFamily({ name: 'mcp__echo__shout' })).toBe('mcp')
  })

  it("files Crowbar's own server and calls that only address a subagent as meta", () => {
    expect(toolFamily({ kind: 'mcp', name: 'mcp__crowbar__set_chat_title' })).toBe('meta')
    expect(toolFamily({ kind: 'mcp', mcpServer: 'crowbar', name: 'set_chat_title' })).toBe('meta')
    expect(toolFamily({ kind: 'execute', name: 'wait', addressesScope: 'child-1' })).toBe('meta')
  })

  it('calls anything it does not know other', () => {
    expect(toolFamily({ name: 'Frobnicate' })).toBe('other')
    expect(toolFamily({ kind: 'ask', name: 'AskUserQuestion' })).toBe('other')
  })
})
