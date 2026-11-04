import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/features/agent/api/agent-chat-commands', () => ({ killChat: vi.fn() }))
vi.mock('@/components/ui/primitive-dialog-service', () => ({ primitiveConfirm: vi.fn() }))
vi.mock('@/lib/errors/report', () => ({ report: vi.fn() }))

import { confirmAndKillAgent, killAgentConfirmation } from '@/features/agent/lib/kill-agent'
import { killChat } from '@/features/agent/api/agent-chat-commands'
import { primitiveConfirm } from '@/components/ui/primitive-dialog-service'
import { report } from '@/lib/errors/report'
import * as hostResolve from '@/lib/host-resolve'

const session = { chatId: 'c1', title: 'Fix login', attached: 0 }

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(killChat).mockResolvedValue(undefined)
  vi.spyOn(hostResolve, 'hostIdForChat').mockImplementation((id) =>
    id === 'c1' ? 'host-1' : 'local',
  )
})

describe('confirmAndKillAgent', () => {
  it('kills the chat’s agent once the user confirms', async () => {
    vi.mocked(primitiveConfirm).mockResolvedValue(true)

    await expect(confirmAndKillAgent(session)).resolves.toBe(true)

    expect(killChat).toHaveBeenCalledExactlyOnceWith('host-1', 'c1')
  })

  it('kills nothing when the user keeps it running', async () => {
    vi.mocked(primitiveConfirm).mockResolvedValue(false)

    await expect(confirmAndKillAgent(session)).resolves.toBe(false)

    expect(killChat).not.toHaveBeenCalled()
  })

  it('warns how many clients are attached before killing', async () => {
    vi.mocked(primitiveConfirm).mockResolvedValue(false)

    await confirmAndKillAgent({ ...session, attached: 2 })

    expect(primitiveConfirm).toHaveBeenCalledWith(
      expect.stringContaining('2 clients are attached'),
      expect.objectContaining({ destructive: true }),
    )
  })

  it('says so when the chat’s host cannot be told', async () => {
    vi.mocked(primitiveConfirm).mockResolvedValue(true)
    const unresolved = new Error('chat c1 is on no host')
    vi.mocked(hostResolve.hostIdForChat).mockImplementation(() => {
      throw unresolved
    })

    await expect(confirmAndKillAgent(session)).resolves.toBe(false)

    expect(killChat).not.toHaveBeenCalled()
    expect(report).toHaveBeenCalledWith(unresolved, {
      action: expect.stringContaining('Fix login'),
    })
  })

  it('says so when the kill is refused', async () => {
    vi.mocked(primitiveConfirm).mockResolvedValue(true)
    const refusal = new Error('daemon unreachable')
    vi.mocked(killChat).mockRejectedValue(refusal)

    await expect(confirmAndKillAgent(session)).resolves.toBe(false)

    expect(report).toHaveBeenCalledWith(refusal, { action: expect.stringContaining('Fix login') })
  })
})

describe('killAgentConfirmation', () => {
  it('names the chat and who is attached', () => {
    expect(killAgentConfirmation({ ...session, attached: 1 })).toEqual({
      title: 'Kill the agent on \u201cFix login\u201d?',
      message: expect.stringContaining('One client is attached'),
    })
  })
})
