import { beforeEach, describe, expect, it, vi } from 'vitest'

const apiFetch = vi.fn()
vi.mock('@/lib/api/fetch', () => ({ apiFetch: (...a: unknown[]) => apiFetch(...a) }))
// Only the scope REGISTRY is faked; the URL builders themselves run for real,
// so these assertions pin the actual paths this client dials rather than a
// stand-in shape. 'w1' carries a real repo, so chatBase builds the Task-17
// repo-scoped shape; 'ws-home' has repoId '' (a project-home workspace) and
// falls back to the still-live /home mount; anything else is unrecorded scope.
vi.mock('@/lib/workspace-scope', () => ({
  bindActiveWorkspaceId: () => {},
  getWorkspaceScope: (id: string) => {
    if (id === 'w1') return { projectId: 'p1', repoId: 'r1', wsId: 'w1' }
    if (id === 'ws-home') return { projectId: 'p1', repoId: '', wsId: 'ws-home' }
    return null
  },
  getOwningChatId: (id: string) => (id === 'w1' ? 'chat-w1' : null),
}))

import * as api from '@/features/agent/api/agent-chat-commands'
import * as providersApi from '@/features/agent/api/agent-providers-api'

describe('agent-chat-commands', () => {
  beforeEach(() => apiFetch.mockReset())

  it('submits one completed prompt with a stable client request identity', async () => {
    apiFetch.mockResolvedValue({ runnerId: 'r2', terminalSessionId: 'pty2' })
    const result = await api.submitAgentPrompt('w1', 'c1', 'Line one\nline two', 'request-1')
    expect(result).toEqual({ runnerId: 'r2', terminalSessionId: 'pty2' })
    expect(apiFetch).toHaveBeenCalledWith(
      '/v0/projects/p1/repos/r1/chats/c1/prompts',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ text: 'Line one\nline two', clientRequestId: 'request-1' }),
      }),
    )
  })

  it('threads a staged model/effort onto the same prompt body, committed atomically', async () => {
    apiFetch.mockResolvedValue({ runnerId: 'r2', terminalSessionId: 'pty2' })
    await api.submitAgentPrompt('w1', 'c1', 'go', 'request-1', '', 'opus', 'high')
    expect(apiFetch).toHaveBeenCalledWith(
      '/v0/projects/p1/repos/r1/chats/c1/prompts',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({
          text: 'go',
          clientRequestId: 'request-1',
          provider: '',
          model: 'opus',
          effort: 'high',
        }),
      }),
    )
  })

  // THE REGRESSION this session fixed: a staged CROSS-provider pick used to
  // call a separate, immediate switchProvider — killing the live CLI the
  // instant a row was clicked, before the user ever sent anything. It now
  // travels on this SAME call, exactly like model/effort, and the backend
  // decides whether that actually means switching anything.
  it('threads a staged provider onto the same prompt body, committed atomically', async () => {
    apiFetch.mockResolvedValue({ runnerId: 'r2', terminalSessionId: 'pty2' })
    await api.submitAgentPrompt('w1', 'c1', 'go', 'request-1', 'codex', '', '')
    expect(apiFetch).toHaveBeenCalledWith(
      '/v0/projects/p1/repos/r1/chats/c1/prompts',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({
          text: 'go',
          clientRequestId: 'request-1',
          provider: 'codex',
          model: '',
          effort: '',
        }),
      }),
    )
  })

  it('switchProvider POSTs to /switch and returns the NEW RUNNER id', async () => {
    apiFetch.mockResolvedValue({ id: 'r2' })
    const runnerId = await api.switchProvider('w1', 'c1', 'claude')
    expect(runnerId).toBe('r2')
    expect(apiFetch.mock.calls[0][0]).toBe('/v0/projects/p1/repos/r1/chats/c1/switch')
    expect(apiFetch.mock.calls[0][1]).toMatchObject({
      method: 'POST',
      body: JSON.stringify({ provider: 'claude' }),
    })
  })

  it('resumeChat POSTs to /resume and returns the id of the RUNNER now on the chat', async () => {
    apiFetch.mockResolvedValue({ id: 'r9' })
    const runnerId = await api.resumeChat('w1', 'c1')
    expect(runnerId).toBe('r9')
    expect(apiFetch.mock.calls[0]).toEqual([
      '/v0/projects/p1/repos/r1/chats/c1/resume',
      { method: 'POST' },
    ])
  })

  it('renameChat POSTs the title; deleteChat DELETEs; listProviders GETs', async () => {
    apiFetch.mockResolvedValue(undefined)
    await api.renameChat('w1', 'c1', 'New')
    expect(apiFetch.mock.calls[0][0]).toBe('/v0/projects/p1/repos/r1/chats/c1/rename')
    expect(apiFetch.mock.calls[0][1]).toMatchObject({
      method: 'POST',
      body: JSON.stringify({ title: 'New' }),
    })
    await api.deleteChat('w1', 'c1')
    expect(apiFetch.mock.calls[1]).toEqual([
      '/v0/projects/p1/repos/r1/chats/c1',
      { method: 'DELETE' },
    ])
    apiFetch.mockResolvedValue([{ id: 'claude', displayName: 'Claude', icon: '<svg/>' }])
    const p = await providersApi.listProviders('w1')
    expect(apiFetch.mock.calls[2][0]).toBe('/v0/projects/p1/repos/r1/chats/providers')
    expect(p[0]).toMatchObject({ id: 'claude', displayName: 'Claude' })
  })

  it('deleteChat answers the chats its cascade took, and none for a reply without a list', async () => {
    apiFetch.mockResolvedValueOnce({ takenChatIds: ['c2', 'c3'] })
    expect(await api.deleteChat('w1', 'c1')).toEqual(['c2', 'c3'])
    for (const reply of [undefined, {}, { takenChatIds: 'c2' }, { takenChatIds: null }]) {
      apiFetch.mockResolvedValueOnce(reply)
      expect(await api.deleteChat('w1', 'c1')).toEqual([])
    }
  })

  it('answerChoice POSTs the picked options to the prompt’s own route', async () => {
    apiFetch.mockResolvedValue(undefined)

    await api.answerChoice('w1', 'c1', 'k 1', { optionIds: ['allow'] })

    expect(apiFetch).toHaveBeenCalledWith(
      '/v0/projects/p1/repos/r1/chats/c1/choices/k%201/answer',
      expect.objectContaining({ method: 'POST' }),
    )
    expect(JSON.parse(apiFetch.mock.calls[0]?.[1]?.body)).toEqual({ optionIds: ['allow'] })
  })

  // ONE call carries the whole prompt however many questions it asked: the option
  // ids say which question each pick answers, so there is never a second call —
  // and never a partial one, which is the shape that stranded the agent.
  it('answerChoice sends every question’s picks in a single flat list', async () => {
    apiFetch.mockResolvedValue(undefined)

    await api.answerChoice('w1', 'c1', 'k1', {
      optionIds: ['q0-answer-0', 'q1-answer-0', 'q1-answer-2', 'q2-answer-1'],
    })

    expect(apiFetch).toHaveBeenCalledTimes(1)
    expect(JSON.parse(apiFetch.mock.calls[0]?.[1]?.body)).toEqual({
      optionIds: ['q0-answer-0', 'q1-answer-0', 'q1-answer-2', 'q2-answer-1'],
    })
  })

  // Several ids in ONE answer is what `multi` means, and the elicitation form
  // travels beside them as the provider's own document.
  it('answerChoice carries several options, a reason and a form document', async () => {
    apiFetch.mockResolvedValue(undefined)

    await api.answerChoice('w1', 'c1', 'k1', {
      optionIds: ['answer-0', 'answer-1'],
      reason: 'both apply',
      content: { name: 'crowbar' },
    })

    expect(JSON.parse(apiFetch.mock.calls[0]?.[1]?.body)).toEqual({
      optionIds: ['answer-0', 'answer-1'],
      reason: 'both apply',
      content: { name: 'crowbar' },
    })
  })

  describe('chat folders + placement', () => {
    it('setChatPlacement PATCHes {parentId, order} to the placement route and unwraps {chat, shifted}', async () => {
      apiFetch.mockResolvedValueOnce({
        chat: {
          id: 'c1',
          workspaceId: 'w1',
          title: 'T',
          liveRunnerId: '',
          terminalSessionId: '',
          activeProviderId: 'claude',
          createdAt: '2026-01-01T00:00:00Z',
          // parentId/order omitted on the wire chat — grounded by mapChat below.
        },
        shifted: [{ id: 'f1', workspaceId: 'w1', title: 'F1', parentId: '', order: 3 }],
      })
      const { chat, shifted } = await api.setChatPlacement('w1', 'c1', { parentId: 'f1', order: 2 })
      expect(apiFetch.mock.calls[0][0]).toBe('/v0/projects/p1/repos/r1/chats/c1/placement')
      expect(apiFetch.mock.calls[0][1]).toMatchObject({
        method: 'PATCH',
        body: JSON.stringify({ parentId: 'f1', order: 2 }),
      })
      expect(chat).toMatchObject({ id: 'c1', parentId: '', order: 0 })
      expect(shifted[0]).toMatchObject({ id: 'f1', order: 3 })
    })

    it('setChatPlacement defaults shifted to [] when the backend omits it', async () => {
      apiFetch.mockResolvedValueOnce({
        chat: {
          id: 'c1',
          workspaceId: 'w1',
          title: 'T',
          liveRunnerId: '',
          terminalSessionId: '',
          activeProviderId: 'claude',
          createdAt: '2026-01-01T00:00:00Z',
        },
      })
      const { shifted } = await api.setChatPlacement('w1', 'c1', { parentId: '', order: 0 })
      expect(shifted).toEqual([])
    })
  })
})
