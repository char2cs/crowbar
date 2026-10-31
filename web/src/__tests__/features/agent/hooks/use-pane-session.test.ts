import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { usePaneSession } from '@/features/agent/hooks/use-pane-session'
import type { PaneSessionInputs } from '@/features/agent/hooks/use-pane-session'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { seedChats, writeChat } from '@/__tests__/__fixtures__/agent-chat'
import { pairRemoteHost, resetHostWorld } from '@/__tests__/__fixtures__/remote-host'
import {
  destroyHostRuntime,
  getOrCreateHostRuntime,
} from '@/features/hosts/stores/host-runtime-registry'
import { useConnectionStore } from '@/lib/ws/connection-store'

const { resumeChatFn, toastErrorFn } = vi.hoisted(() => ({
  resumeChatFn: vi.fn(),
  toastErrorFn: vi.fn(),
}))

vi.mock('@/features/agent/api/agent-api', () => ({
  resumeChat: (...a: unknown[]) => resumeChatFn(...a),
}))
vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: (...a: unknown[]) => toastErrorFn(...a) },
}))

function setup(chats: Parameters<typeof seedChats>[1], overrides: Partial<PaneSessionInputs> = {}) {
  const store = createWorkspaceStore('w1')
  seedChats(store, chats)
  const inputs: PaneSessionInputs = {
    store,
    wsId: 'w1',
    hostId: 'local',
    chatId: 'c1',
    providerName: 'Codex',
    presentation: 'chat',
    promptReplacing: false,
    ...overrides,
  }
  return { store, hook: renderHook(() => usePaneSession(inputs)) }
}

beforeEach(() => {
  resumeChatFn.mockReset()
  toastErrorFn.mockReset()
})

describe('usePaneSession', () => {
  it('is pending, and cannot send, for a chat the list does not carry', () => {
    const { hook } = setup([])
    expect(hook.result.current.attachment).toEqual({ state: 'pending' })
    expect(hook.result.current.canSend).toBe(false)
  })

  it('attaches a live runner’s PTY', () => {
    const { hook } = setup([{ id: 'c1', liveRunnerId: 'r1', terminalSessionId: 'pty1' }])
    expect(hook.result.current.attachment).toEqual({ state: 'attached', sessionId: 'pty1' })
    expect(hook.result.current.canSend).toBe(true)
  })

  it('swaps straight to a replacement PTY, never flashing pending', () => {
    const store = createWorkspaceStore('w1')
    seedChats(store, [{ id: 'c1', liveRunnerId: 'r1', terminalSessionId: 'pty1' }])
    const seen: unknown[] = []
    const hook = renderHook(() => {
      const session = usePaneSession({
        store,
        wsId: 'w1',
        hostId: 'local',
        chatId: 'c1',
        providerName: 'Codex',
        presentation: 'terminal',
        promptReplacing: false,
      })
      seen.push(session.attachment)
      return session
    })

    expect(hook.result.current.attachment).toEqual({ state: 'attached', sessionId: 'pty1' })
    seen.length = 0

    act(() => {
      writeChat(store, { id: 'c1', liveRunnerId: 'r2', terminalSessionId: 'pty2' })
    })

    expect(seen).not.toContainEqual({ state: 'pending' })
    expect(hook.result.current.attachment).toEqual({ state: 'attached', sessionId: 'pty2' })
  })

  it('lets a dormant chat send, and says why it is dormant', () => {
    const { hook } = setup([{ id: 'c1', phase: 'dormant', session: { exitReason: 'stopped' } }])
    expect(hook.result.current.attachment.state).toBe('idle')
    expect(hook.result.current.canSend).toBe(true)
    expect(hook.result.current.sessionNote).toMatch(/stopped/i)
  })

  it('shows the daemon placing a CLI as a revival, chat side only', () => {
    const { hook } = setup([{ id: 'c1', phase: 'starting' }])
    expect(hook.result.current.revival).toEqual({ state: 'reviving', message: 'Starting Codex…' })
    expect(hook.result.current.canSend).toBe(false)

    const terminal = setup([{ id: 'c1', phase: 'starting' }], { presentation: 'terminal' })
    expect(terminal.hook.result.current.revival).toBeUndefined()
  })

  // The chat's transcript stays readable from the cache, but nothing sent now would arrive.
  describe('when the chat’s host does not answer', () => {
    afterEach(() => {
      resetHostWorld()
      destroyHostRuntime('h1')
    })

    it('cannot send, and is offline, while a paired host is down', () => {
      pairRemoteHost('h1')
      getOrCreateHostRuntime('h1').getState().setStatus('down')
      const { hook } = setup([{ id: 'c1', liveRunnerId: 'r1' }], { hostId: 'h1' })

      expect(hook.result.current.offline).toBe(true)
      expect(hook.result.current.canSend).toBe(false)
    })

    it('sends again once the host is back up', () => {
      pairRemoteHost('h1')
      getOrCreateHostRuntime('h1').getState().setStatus('down')
      const { hook } = setup([{ id: 'c1', liveRunnerId: 'r1' }], { hostId: 'h1' })

      act(() => getOrCreateHostRuntime('h1').getState().setStatus('up'))

      expect(hook.result.current.offline).toBe(false)
      expect(hook.result.current.canSend).toBe(true)
    })

    it('is offline when the local daemon’s socket is down', () => {
      useConnectionStore.setState({ status: 'disconnected' })
      const { hook } = setup([{ id: 'c1', liveRunnerId: 'r1' }])

      expect(hook.result.current.offline).toBe(true)
      expect(hook.result.current.canSend).toBe(false)
      useConnectionStore.setState({ status: 'connected' })
    })
  })

  describe('startSession', () => {
    afterEach(() => {
      resetHostWorld()
      destroyHostRuntime('h1')
    })

    it('does not ask a host that does not answer', () => {
      pairRemoteHost('h1')
      getOrCreateHostRuntime('h1').getState().setStatus('down')
      const { hook } = setup([{ id: 'c1' }], { hostId: 'h1' })

      act(() => hook.result.current.startSession())

      expect(resumeChatFn).not.toHaveBeenCalled()
      expect(toastErrorFn).not.toHaveBeenCalled()
    })

    it('reads the chat as it is when called, not as it was when handed out', () => {
      const { store, hook } = setup([{ id: 'c1' }])
      const stale = hook.result.current.startSession
      act(() => {
        writeChat(store, { id: 'c1', liveRunnerId: 'r1', terminalSessionId: 'pty1' })
      })

      act(() => stale())

      expect(resumeChatFn).not.toHaveBeenCalled()
    })

    it('does not resume a chat the list does not carry', () => {
      const { hook } = setup([])

      act(() => hook.result.current.startSession())

      expect(resumeChatFn).not.toHaveBeenCalled()
    })
  })

  it('startSession asks the daemon to resume, and toasts a refusal', async () => {
    resumeChatFn.mockRejectedValue(new Error('no'))
    const { hook } = setup([{ id: 'c1' }])

    await act(async () => {
      hook.result.current.startSession()
    })

    expect(resumeChatFn).toHaveBeenCalledWith('w1', 'c1')
    expect(toastErrorFn).toHaveBeenCalledTimes(1)
  })
})
