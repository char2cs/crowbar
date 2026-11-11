import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { PermissionLevel } from '@/features/agent/api/agent-providers-api'
import {
  PermissionLevelChip,
  PermissionLevelSwitcher,
} from '@/features/agent/composer/permission-level-switcher'
import { destroyHostRuntime } from '@/features/hosts/stores/host-runtime-registry'
import { toast } from '@/features/window/stores/toast-store'
import { proxyMarkerError } from '@/lib/api/errors'
import { pairRemoteHost, resetHostWorld } from '@/__tests__/__fixtures__/remote-host'
import { resetChatStore, writeChat } from '@/__tests__/__fixtures__/agent-chat'

const { setChatPermissionLevelFn, defaultLevelFn } = vi.hoisted(() => ({
  setChatPermissionLevelFn: vi.fn(),
  defaultLevelFn: vi.fn(),
}))

vi.mock('@/features/agent/api/agent-chat-commands', () => ({
  setChatPermissionLevel: (...args: unknown[]) => setChatPermissionLevelFn(...args),
}))
vi.mock('@/features/agent/api/agent-providers-api', () => ({
  PERMISSION_LEVEL_OPTIONS: [
    { value: 'guarded', label: 'Guarded' },
    { value: 'trusted', label: 'Trusted' },
    { value: 'full-auto', label: 'Full Auto' },
  ],
  getDefaultPermissionLevel: (...args: unknown[]) => defaultLevelFn(...args),
}))

// jsdom has no PointerEvent constructor; base-ui's Select opens/commits via it.
if (typeof window.PointerEvent !== 'function') {
  class PointerEventShim extends MouseEvent {}
  Object.assign(window, { PointerEvent: PointerEventShim })
}

beforeEach(() => {
  resetChatStore()
  setChatPermissionLevelFn.mockReset()
  setChatPermissionLevelFn.mockResolvedValue(undefined)
  defaultLevelFn.mockReset()
  defaultLevelFn.mockResolvedValue('guarded')
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

  // A chat that inherits the default runs at that default, so it says which one.
  it('shows the default level of the chat’s host while the chat inherits it', async () => {
    writeChat({ id: 'c1' })
    defaultLevelFn.mockResolvedValue('trusted')
    draw()
    await waitFor(() =>
      expect(screen.getByTestId('chat-permission-level-trigger')).toHaveTextContent(/^Trusted$/),
    )
    expect(defaultLevelFn).toHaveBeenCalledWith('local')
  })

  it('shows the nearest more permissive level when the provider lacks the default', async () => {
    writeChat({ id: 'c1' })
    draw('c1', ['trusted', 'full-auto'])
    await waitFor(() =>
      expect(screen.getByTestId('chat-permission-level-trigger')).toHaveTextContent(/^Trusted$/),
    )
  })

  it('names the level the chat is pinned to by its label, never its id', () => {
    writeChat({ id: 'c1', permissionLevel: 'trusted' })
    draw()
    expect(screen.getByTestId('chat-permission-level-trigger')).toHaveTextContent(/^Trusted$/)
  })

  it('picking a level PUTs it for this chat', async () => {
    const user = userEvent.setup()
    draw()
    await pickLevel(user, 'trusted')
    expect(setChatPermissionLevelFn).toHaveBeenCalledWith('w1', 'c1', 'trusted')
  })

  // The composer remounts it between prompts of one chat; it keeps only what it confirmed.
  it('names the remote host a pick could not reach, instead of blaming the daemon', async () => {
    pairRemoteHost('h1', 'Studio Mac')
    const toastError = vi.spyOn(toast, 'error').mockImplementation(() => '')
    setChatPermissionLevelFn.mockRejectedValue(proxyMarkerError('host-unreachable', 'h1'))
    const user = userEvent.setup()
    draw()

    await pickLevel(user, 'trusted')

    await waitFor(() =>
      expect(toastError).toHaveBeenCalledExactlyOnceWith(
        'Couldn’t reach Studio Mac',
        'Check that it is online in Settings → Hosts.',
      ),
    )
    toastError.mockRestore()
    destroyHostRuntime('h1')
    resetHostWorld()
  })
})

describe('PermissionLevelChip', () => {
  function drawChip(chatId = 'c1', levels: PermissionLevel[] = ALL) {
    return render(<PermissionLevelChip wsId="w1" chatId={chatId} levels={levels} />)
  }

  it('names the level the chat is pinned to on the footer chip', () => {
    writeChat({ id: 'c1', permissionLevel: 'trusted' })
    drawChip()
    expect(screen.getByTestId('chat-permission-level-chip')).toHaveTextContent(/^Trusted$/)
  })

  it('draws nothing when the provider declares no levels', () => {
    const { container } = drawChip('c1', [])
    expect(container).toBeEmptyDOMElement()
  })

  it('opens as a menu of only the declared levels and PUTs the pick', async () => {
    const user = userEvent.setup()
    drawChip('c1', ['trusted', 'full-auto'])

    await user.click(screen.getByTestId('chat-permission-level-chip'))
    expect(screen.queryByRole('menuitem', { name: 'Guarded' })).toBeNull()
    await user.click(await screen.findByRole('menuitem', { name: 'Full Auto' }))

    expect(setChatPermissionLevelFn).toHaveBeenCalledWith('w1', 'c1', 'full-auto')
    await waitFor(() =>
      expect(screen.getByTestId('chat-permission-level-chip')).toHaveTextContent(/^Full Auto$/),
    )
  })
})
