import { describe, expect, it } from 'vitest'
import { activityComponents } from '@/features/agent/lib/activity-components'

describe('activityComponents', () => {
  it('projects every existing activity record into one ordered provider-neutral stream', () => {
    const activity = {
      toolCalls: [
        {
          id: 'tool',
          turnId: 'turn',
          seq: 2,
          name: 'Bash',
          kind: 'execute',
          locations: [{ path: 'src/main.ts', line: 9 }],
          status: 'ok',
          hasRequest: true,
          hasResult: true,
          startedAt: '2026-01-01T00:00:02Z',
          endedAt: '2026-01-01T00:00:03Z',
        },
      ],
      choices: [
        {
          id: 'choice',
          turnId: 'turn',
          seq: 3,
          kind: 'tool_permission',
          options: [],
          pending: true,
          answerable: true,
          at: '2026-01-01T00:00:01Z',
        },
      ],
      subagents: [],
      interruptions: [],
    } as never

    expect(activityComponents(activity)).toMatchObject([
      {
        id: 'tool',
        kind: 'tool_call',
        status: 'completed',
        payload: { kind: 'execute', locations: [{ path: 'src/main.ts', line: 9 }] },
      },
      {
        id: 'tool:output',
        parentId: 'tool',
        kind: 'tool_output',
        status: 'completed',
        payload: { toolCallId: 'tool', side: 'result' },
      },
      { id: 'choice', kind: 'permission_request', status: 'pending' },
    ])
    expect(activityComponents(activity)[1].updates?.[0]).toMatchObject({
      id: 'tool:output:1',
      kind: 'available',
      status: 'completed',
    })
  })

  it('keeps open work active and preserves nested ownership', () => {
    const components = activityComponents({
      toolCalls: [
        {
          id: 'nested-tool',
          turnId: '',
          subagentId: 'subagent',
          seq: 1,
          name: 'Read',
          status: 'running',
          hasRequest: false,
          hasResult: false,
          startedAt: '2026-01-01T00:00:00Z',
        },
      ],
      choices: [],
      subagents: [
        {
          id: 'subagent',
          turnId: 'root-turn',
          seq: 2,
          startedAt: '2026-01-01T00:00:00Z',
          messages: [],
        },
      ],
      interruptions: [],
    } as never)

    expect(components[0]).toMatchObject({
      id: 'nested-tool',
      turnId: 'root-turn',
      parentId: 'subagent',
      status: 'active',
    })
  })

  it('distinguishes a denied permission from an approved request', () => {
    const components = activityComponents({
      toolCalls: [],
      subagents: [],
      interruptions: [],
      choices: [
        {
          id: 'permission',
          turnId: 'turn',
          seq: 1,
          kind: 'tool_permission',
          options: [
            { id: 'allow', kind: 'allow', label: 'Allow' },
            { id: 'deny', kind: 'deny', label: 'Deny' },
          ],
          pending: false,
          answerable: false,
          resolution: 'answered',
          answeredOptionIds: ['deny'],
          at: '2026-01-01T00:00:00Z',
          resolvedAt: '2026-01-01T00:00:01Z',
        },
      ],
    } as never)

    expect(components[0]).toMatchObject({ kind: 'permission_request', status: 'declined' })
  })

  it('projects compaction as its own canonical component kind', () => {
    const components = activityComponents({
      toolCalls: [],
      choices: [],
      subagents: [],
      interruptions: [
        {
          id: 'compact',
          turnId: 'turn',
          seq: 1,
          kind: 'compaction',
          at: '2026-01-01T00:00:00Z',
          resolvedAt: '2026-01-01T00:00:01Z',
        },
      ],
    } as never)

    expect(components[0]).toMatchObject({ id: 'compact', kind: 'compaction', status: 'completed' })
  })
})
