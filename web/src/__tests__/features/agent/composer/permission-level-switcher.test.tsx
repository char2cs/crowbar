import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { PermissionLevel } from '@/features/agent/api/agent-providers-api'
import { PermissionLevelSwitcher } from '@/features/agent/composer/permission-level-switcher'

const { setChatPermissionLevelFn } = vi.hoisted(() => ({ setChatPermissionLevelFn: vi.fn() }))

vi.mock('@/features/agent/api/agent-chat-commands', () => ({
  setChatPermissionLevel: (...args: unknown[]) => setChatPermissionLevelFn(...args),
}))
vi.mock('@/features/agent/api/agent-providers-api', () => ({
  PERMISSION_LEVEL_OPTIONS: [
    { value: 'guarded', label: 'Guarded' },
    { value: 'trusted', label: 'Trusted' },
    { value: 'full-auto', label: 'Full Auto' },
  ],
}))

// jsdom has no PointerEvent constructor; base-ui's Select opens/commits via it.
if (typeof window.PointerEvent !== 'function') {
  class PointerEventShim extends MouseEvent {}
  // @ts-expect-error install the shim onto the jsdom window
  window.PointerEvent = PointerEventShim
}

beforeEach(() => {
  setChatPermissionLevelFn.mockReset()
  setChatPermissionLevelFn.mockResolvedValue(undefined)
})

const ALL: PermissionLevel[] = ['guarded', 'trusted', 'full-auto']

function draw(chatId = 'c1', levels = ALL) {
  return render(<PermissionLevelSwitcher wsId="w1" chatId={chatId} availableLevels={levels} />)
}

async function pickLevel(user: ReturnType<typeof userEvent.setup>, value: string) {
  await user.click(screen.getByTestId('chat-permission-level-trigger'))
  const option = await screen.findByTestId(`chat-permission-level-option-${value}`)
  await waitFor(() => expect(getComputedStyle(option).pointerEvents).not.toBe('none'))
  await user.click(option)
}

describe('PermissionLevelSwitcher', () => {
  it('offers only the levels this provider actually declares', async () => {
    const user = userEvent.setup()
    draw('c1', ['trusted', 'full-auto'])

    await user.click(screen.getByTestId('chat-permission-level-trigger'))
    expect(await screen.findByTestId('chat-permission-level-option-trusted')).toBeInTheDocument()
    expect(screen.getByTestId('chat-permission-level-option-full-auto')).toBeInTheDocument()
    expect(screen.queryByTestId('chat-permission-level-option-guarded')).toBeNull()
  })

  // There is no per-chat GET, only the PUT: the dial never claims a level it cannot read.
  it('starts on no level, never guessing at one the backend cannot report', () => {
    draw()
    expect(screen.getByTestId('chat-permission-level-trigger')).toHaveTextContent(
      'Permission level',
    )
  })

  it('picking a level PUTs it for this chat', async () => {
    const user = userEvent.setup()
    draw()
    await pickLevel(user, 'trusted')
    expect(setChatPermissionLevelFn).toHaveBeenCalledWith('w1', 'c1', 'trusted')
  })

  // The composer remounts it between prompts of one chat; it keeps only what it confirmed.
  it('remembers its own last confirmed pick across a remount of the same chat, keyed per chat', async () => {
    const user = userEvent.setup()
    const first = draw('persist-1')
    await pickLevel(user, 'full-auto')
    await waitFor(() =>
      expect(setChatPermissionLevelFn).toHaveBeenCalledWith('w1', 'persist-1', 'full-auto'),
    )
    first.unmount()

    const second = draw('persist-1')
    expect(screen.getByTestId('chat-permission-level-trigger')).toHaveTextContent('full-auto')
    second.unmount()

    draw('persist-2')
    expect(screen.getByTestId('chat-permission-level-trigger')).toHaveTextContent(
      'Permission level',
    )
  })
})
