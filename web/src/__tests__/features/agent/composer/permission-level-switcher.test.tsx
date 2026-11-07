import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { PermissionLevel } from '@/features/agent/api/agent-providers-api'
import { PermissionLevelSwitcher } from '@/features/agent/composer/permission-level-switcher'
import { destroyHostRuntime } from '@/features/hosts/stores/host-runtime-registry'
import { toast } from '@/features/window/stores/toast-store'
import { proxyMarkerError } from '@/lib/api/errors'
import { pairRemoteHost, resetHostWorld } from '@/__tests__/__fixtures__/remote-host'
import { resetChatStore, writeChat } from '@/__tests__/__fixtures__/agent-chat'

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
  resetChatStore()
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

  // A chat that inherits the default has no level of its own to show.
  it('starts on no level while the chat inherits the default', () => {
    draw()
    expect(screen.getByTestId('chat-permission-level-trigger')).toHaveTextContent(
      'Permission level',
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
