import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/features/agent/api/agent-api', () => ({ killChat: vi.fn() }))
vi.mock('@/components/ui/primitive-dialog-service', () => ({ primitiveConfirm: vi.fn() }))
vi.mock('@/features/window/stores/toast-store', () => ({ toast: { error: vi.fn() } }))

import { confirmAndKillAgent } from '@/features/agent/lib/kill-agent'
import { killChat } from '@/features/agent/api/agent-api'
import { primitiveConfirm } from '@/components/ui/primitive-dialog-service'
import { toast } from '@/features/window/stores/toast-store'

const session = { chatId: 'c1', workspaceId: 'ws-1', title: 'Fix login', attached: 0 }

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(killChat).mockResolvedValue(undefined)
})

describe('confirmAndKillAgent', () => {
  it('kills the chat’s agent once the user confirms', async () => {
    vi.mocked(primitiveConfirm).mockResolvedValue(true)

    await expect(confirmAndKillAgent(session)).resolves.toBe(true)

    expect(killChat).toHaveBeenCalledExactlyOnceWith('ws-1', 'c1')
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

  it('says so when the kill is refused', async () => {
    vi.mocked(primitiveConfirm).mockResolvedValue(true)
    vi.mocked(killChat).mockRejectedValue(new Error('daemon unreachable'))

    await expect(confirmAndKillAgent(session)).resolves.toBe(false)

    expect(toast.error).toHaveBeenCalledWith(
      expect.stringContaining('Fix login'),
      'daemon unreachable',
    )
  })
})
