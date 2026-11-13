import { act, fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { PlateController, useEditorRef, type PlateEditor } from 'platejs/react'
import * as promote from '@/components/layout/draft-promote'
import * as drafts from '@/components/layout/draft-thread'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { chatValueToMarkdown } from '@/features/agent/composer/plate/chat-composer-serialization'

const claude: AgentProvider = {
  id: 'claude',
  displayName: 'Claude',
  icon: '<svg></svg>',
  connected: true,
  enabled: true,
  mcpEnabled: true,
  modelSelect: true,
  effortSelect: true,
  models: ['sonnet'],
  efforts: { sonnet: ['low', 'high'] },
  compaction: false,
  hasTerminal: true,
  hotswap: false,
  terminalStartHere: true,
}

vi.mock('@/utils/platform', () => ({ IS_MOBILE: true }))
let offered: AgentProvider[] = [claude]
vi.mock('@/features/agent/hooks/use-pane-providers', () => ({ usePaneProviders: () => offered }))
const grabbed: { editor?: PlateEditor } = {}
function GrabEditor() {
  grabbed.editor = useEditorRef()
  return null
}
const boxText = () => chatValueToMarkdown(grabbed.editor!.children as never).trim()

const { DraftChatPane } = await import('@/features/agent/components/draft-chat-pane')

describe('DraftChatPane', () => {
  // The chip's layout lives under the pane scope; outside it the chip is an unstyled
  // button whose chevron drops to a second line.
  it('draws the model chip inside the scope the chat pane styles it under', () => {
    render(<DraftChatPane paneId="p1" wsId="w1" hostId="local" belowOverlayHeader={false} />)
    const chip = screen.getByRole('button', { name: /Agent:/ })
    expect(chip.closest('.agent-chat-pane')).not.toBeNull()
  })

  it('gives a refused create its prompt back, ahead of what was typed meanwhile', async () => {
    vi.spyOn(drafts, 'draftOfPane').mockReturnValue({ draftId: 'd1' } as never)
    let refuse = () => {}
    vi.spyOn(promote, 'promoteDraftThread').mockImplementation(
      () => new Promise<void>((resolve) => (refuse = resolve)),
    )
    render(
      <PlateController>
        <DraftChatPane paneId="p1" wsId="w1" hostId="local" belowOverlayHeader={false} />
        <GrabEditor />
      </PlateController>,
    )
    await act(async () => grabbed.editor!.tf.insertText('fix the bug'))
    fireEvent.click(screen.getByRole('button', { name: 'Send prompt' }))
    expect(boxText()).toBe('')

    await act(async () => grabbed.editor!.tf.insertText('and add a test'))
    await act(async () => refuse())

    expect(boxText()).toBe('# fix the bug\n\n# and add a test')
  })

  it('opens the new chat on its terminal from the strip, keeping the box as the chat’s draft', async () => {
    const spy = vi.spyOn(promote, 'promoteDraftThread').mockResolvedValue()
    spy.mockClear()
    render(
      <PlateController>
        <DraftChatPane paneId="p1" wsId="w1" hostId="local" belowOverlayHeader={false} />
        <GrabEditor />
      </PlateController>,
    )
    await act(async () => grabbed.editor!.tf.insertText('fix the bug'))
    expect(screen.getByRole('tab', { name: 'Chat' }).getAttribute('aria-selected')).toBe('true')
    fireEvent.click(screen.getByRole('tab', { name: 'Terminal' }))

    expect(spy).toHaveBeenCalledOnce()
    const [paneId, prompt, typedSince] = spy.mock.calls[0]!
    expect(paneId).toBe('p1')
    expect(prompt).toMatchObject({ presentation: 'terminal', text: '' })
    expect(typedSince()).toContain('fix the bug')
  })

  describe('when the provider needs a model chosen first', () => {
    const typeAndSend = async () => {
      await act(async () => grabbed.editor!.tf.insertText('fix the bug'))
      fireEvent.click(screen.getByRole('button', { name: 'Send prompt' }))
    }
    const mount = () =>
      render(
        <PlateController>
          <DraftChatPane paneId="p1" wsId="w1" hostId="local" belowOverlayHeader={false} />
          <GrabEditor />
        </PlateController>,
      )

    it('opens the model picker instead of sending, and says why beside the composer', async () => {
      offered = [{ ...claude, models: ['m1', 'm2'], modelRequired: true }]
      const spy = vi.spyOn(promote, 'promoteDraftThread').mockResolvedValue()
      spy.mockClear()
      mount()
      await act(async () => grabbed.editor!.tf.insertText('fix the bug'))
      const send = screen.getByRole('button', { name: 'Send prompt' })
      const notice = screen.getByRole('status')
      expect(notice.textContent).toBe('Choose a model to start')
      expect(notice.closest('.dochandle')).not.toBeNull()
      expect(send.getAttribute('aria-disabled')).toBe('true')
      expect(send.getAttribute('aria-description')).toBe('Choose a model to start')
      expect(screen.queryByRole('menuitem', { name: 'm1' })).toBeNull()

      fireEvent.click(send)

      expect(await screen.findByRole('menuitem', { name: 'm1' })).toBeTruthy()
      expect(spy).not.toHaveBeenCalled()
      offered = [claude]
    })

    it('sends with no notice once the provider has a model to start on', async () => {
      offered = [{ ...claude, models: ['m1'], modelRequired: false }]
      const spy = vi.spyOn(promote, 'promoteDraftThread').mockResolvedValue()
      spy.mockClear()
      mount()
      expect(screen.queryByRole('status')).toBeNull()
      await typeAndSend()
      expect(spy).toHaveBeenCalledOnce()
      offered = [claude]
    })
  })
})
