import { describe, expect, it, vi } from 'vitest'

const { sendPrompt, setChatPermissionLevel } = vi.hoisted(() => ({
  sendPrompt: vi.fn(),
  setChatPermissionLevel: vi.fn(),
}))
vi.mock('@/features/agent/stores/chat-outbox', () => ({ sendPrompt }))
vi.mock('@/features/agent/api/agent-chat-commands', () => ({ setChatPermissionLevel }))

import { draftLanding, finishDraftThread, type DraftSpec } from '@/components/layout/draft-thread'

const draft: DraftSpec = {
  paneId: 'p-gone',
  draftId: 'd1',
  projectId: 'p1',
  workspaceId: 'w1',
  parentId: '',
}
const prompt = { text: 'hello', provider: 'claude', model: '', effort: '' }

describe('finishDraftThread', () => {
  it('sends the first prompt as it is when the draft staged no level', () => {
    finishDraftThread(draft, 'c1', prompt, () => '')
    expect(setChatPermissionLevel).not.toHaveBeenCalled()
    expect(sendPrompt).toHaveBeenCalledWith('w1', 'c1', { ...prompt, steer: false })
  })

  it('writes the staged level to the new chat before the first prompt can start the agent', async () => {
    sendPrompt.mockClear()
    let landed = () => {}
    setChatPermissionLevel.mockReturnValue(new Promise<void>((resolve) => (landed = resolve)))
    finishDraftThread(draft, 'c1', { ...prompt, permissionLevel: 'trusted' }, () => '')
    expect(setChatPermissionLevel).toHaveBeenCalledWith('w1', 'c1', 'trusted')
    expect(sendPrompt).not.toHaveBeenCalled()
    landed()
    await vi.waitFor(() =>
      expect(sendPrompt).toHaveBeenCalledWith('w1', 'c1', { ...prompt, steer: false }),
    )
  })

  it('opens on the terminal without sending when the draft chose it', () => {
    sendPrompt.mockClear()
    const terminal = { ...prompt, presentation: 'terminal' as const }
    finishDraftThread(draft, 'c1', terminal, () => '')
    expect(sendPrompt).not.toHaveBeenCalled()
    expect(draftLanding(terminal).presentation).toBe('terminal')
    expect(draftLanding(prompt).presentation).toBe('chat')
  })
})
