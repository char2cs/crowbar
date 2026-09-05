import { act } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { AgentComposer } from '@/features/agent/composer/agent-composer'
import { uploadChatAttachment } from '@/features/agent/api/upload-chat-attachment'
import { useTauriFileDrop } from '@/features/file-system/lib/tauri-file-drop'
import { NO_ACTIVITY } from '@/features/agent/lib/agent-activity'

vi.mock('@/features/agent/api/upload-chat-attachment', () => ({
  uploadChatAttachment: vi.fn(),
}))

// Task 27's own hook (subscribes to Tauri's real OS-drag channel) is a no-op
// under jsdom — `isTauri()` is false, so its effect never fires. Stubbed here
// so this suite can invoke the `onDrop` callback AgentComposer wires into it
// directly, which is the one thing this task owns: the hook's own subscribe/
// filter behaviour is Task 27's and is covered in tauri-file-drop.test.ts.
vi.mock('@/features/file-system/lib/tauri-file-drop', () => ({
  useTauriFileDrop: vi.fn(),
}))

function draw(overrides: Partial<Parameters<typeof AgentComposer>[0]> = {}) {
  return render(
    <AgentComposer
      wsId="w1"
      chatId="c1"
      activity={NO_ACTIVITY}
      providerLabel="Claude"
      live
      working={false}
      compacting={false}
      sending={false}
      submitUnavailable={false}
      canStop={false}
      draft=""
      fieldHeight={20}
      slashOpen={false}
      onDraftChange={vi.fn()}
      onHeightChange={vi.fn()}
      onKeyDown={vi.fn()}
      onSend={vi.fn()}
      onStop={vi.fn()}
      onOpenTerminal={vi.fn()}
      draftSeed={0}
      seedText=""
      {...overrides}
    />,
  )
}

// The bar delegates its own dispatched-but-unproven visual to the handle — this
// only has to prove the wiring reaches it, not re-litigate the handle's own
// precedence rules (covered in composer-handle.test.tsx).
describe('AgentComposer', () => {
  it('passes sending through to the handle as an input', () => {
    const { container } = draw({ sending: true })

    expect(container.querySelector('[data-flicker-spinner]')).toBeInTheDocument()
  })

  it('shows the plain send affordance when nothing is in flight', () => {
    const { container } = draw({ sending: false })

    expect(container.querySelector('[data-flicker-spinner]')).toBeNull()
    expect(screen.getByRole('button', { name: 'Send prompt' })).toBeInTheDocument()
  })

  // Task 24 gives the composer its own modal state, opened via the handle's
  // plus button. Tasks 29/34 replace the placeholder `null` branches with the
  // real modals — until then, opening either must be a true no-op: no dialog
  // appears and the rest of the bar keeps working exactly as before.
  it('opens the excalidraw modal slot from the plus button without a visible modal yet', async () => {
    const user = userEvent.setup()
    const onSend = vi.fn()
    draw({ draft: 'hi', onSend })

    await user.click(screen.getByRole('button', { name: /add to this message/i }))
    await user.click(await screen.findByRole('menuitem', { name: /excalidraw/i }))

    expect(screen.queryByRole('dialog')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Send prompt' }))
    expect(onSend).toHaveBeenCalledTimes(1)
  })

  it('opens the attach-file modal slot from the plus button without a visible modal yet', async () => {
    const user = userEvent.setup()
    const onSend = vi.fn()
    draw({ draft: 'hi', onSend })

    await user.click(screen.getByRole('button', { name: /add to this message/i }))
    await user.click(await screen.findByRole('menuitem', { name: /attach file/i }))

    expect(screen.queryByRole('dialog')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Send prompt' }))
    expect(onSend).toHaveBeenCalledTimes(1)
  })

  // Pre-existing branch, unrelated to the plus button: proves the switch still
  // reaches `case 'choice'` (and thus never the input/handle branch) once a
  // pending choice is waiting.
  it('renders the choice card instead of the field when a choice is pending', () => {
    draw({
      activity: {
        ...NO_ACTIVITY,
        choices: [
          {
            id: 'k1',
            turnId: 't1',
            seq: 1,
            kind: 'tool_permission',
            toolName: 'Bash',
            options: [{ id: 'allow', kind: 'allow', label: 'Allow' }],
            pending: true,
            answerable: true,
            at: '2026-08-18T12:00:00Z',
          },
        ],
      },
    })

    expect(screen.getByRole('group')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Send prompt' })).toBeNull()
  })

  // Pre-existing branches, unrelated to the plus button: compaction shares the
  // 'input' render path (it queues rather than blocking), and the pill grows a
  // `multi` class once the field itself reports more than one line.
  it('keeps the field mounted while compacting, queuing behind the busy provider', () => {
    draw({ compacting: true })

    expect(screen.getByRole('textbox', { name: 'Message the agent' })).toBeInTheDocument()
  })

  it('marks the pill multiline once the field grows past one line', () => {
    const { container } = draw({ fieldHeight: 40 })

    expect(container.querySelector('.pill')).toHaveClass('multi')
  })
})

// Task 29: dropping a file directly on the pill uploads it and inserts the
// same attachment markdown the (not-yet-built) Attach File modal will
// produce — see uploadChatAttachment/fileMarkdown/imageMarkdown.
describe('AgentComposer drag-and-drop', () => {
  beforeEach(() => {
    vi.mocked(uploadChatAttachment).mockReset()
  })

  it('shows a drag-over state while a file is dragged over the pill', () => {
    const { container } = draw()
    const pill = container.querySelector('.pill')!

    fireEvent.dragOver(pill, { dataTransfer: { types: ['Files'] } })
    expect(pill).toHaveClass('drop-target')

    fireEvent.dragLeave(pill)
    expect(pill).not.toHaveClass('drop-target')
  })

  it('ignores a drag-over that carries no files', () => {
    const { container } = draw()
    const pill = container.querySelector('.pill')!

    fireEvent.dragOver(pill, { dataTransfer: { types: ['text/plain'] } })

    expect(pill).not.toHaveClass('drop-target')
  })

  // The other half of drag-leave's guard: a leave that lands on a target the
  // pill still contains (its own field, its own handle) is not really a
  // leave — clearing the state there would flicker the border on every pixel
  // the pointer crosses between the two.
  it('keeps the drag-over state when drag-leave lands on a still-contained child', () => {
    const { container } = draw()
    const pill = container.querySelector('.pill')!

    fireEvent.dragOver(pill, { dataTransfer: { types: ['Files'] } })
    expect(pill).toHaveClass('drop-target')

    // jsdom's `DragEvent` constructor drops a `relatedTarget` passed via its
    // init dict (unlike `dataTransfer`, testing-library has no special case
    // for it) — defined directly on a plain event instead, which React's
    // synthetic-event layer reads the same way for `onDragLeave`.
    const leave = new Event('dragleave', { bubbles: true, cancelable: false })
    Object.defineProperty(leave, 'relatedTarget', { value: pill, configurable: true })
    fireEvent(pill, leave)

    expect(pill).toHaveClass('drop-target')
  })

  it('uploads a dropped image and inserts image markdown, clearing the drag-over state', async () => {
    vi.mocked(uploadChatAttachment).mockResolvedValue({
      ref: 'chats/c1/attachments/x-a.png',
      filename: 'a.png',
      size: 10,
      contentType: 'image/png',
    })
    const onDraftChange = vi.fn()
    const { container } = draw({ onDraftChange })
    const pill = container.querySelector('.pill')!
    const file = new File(['bytes'], 'a.png', { type: 'image/png' })

    fireEvent.dragOver(pill, { dataTransfer: { types: ['Files'] } })
    fireEvent.drop(pill, { dataTransfer: { types: ['Files'], files: [file] } })

    expect(pill).not.toHaveClass('drop-target')
    await waitFor(() => expect(uploadChatAttachment).toHaveBeenCalledWith('w1', 'c1', { file }))
    await waitFor(() => {
      const lastCall = onDraftChange.mock.calls.at(-1)?.[0] as string | undefined
      expect(lastCall).toContain('![a.png](chats/c1/attachments/x-a.png)')
    })
  })

  // The other branch of insertUploaded's contentType switch: a non-image
  // result gets the plain-link encoding, not the image one.
  it('uploads a dropped non-image file and inserts link markdown', async () => {
    vi.mocked(uploadChatAttachment).mockResolvedValue({
      ref: 'chats/c1/attachments/y-b.pdf',
      filename: 'b.pdf',
      size: 20,
      contentType: 'application/pdf',
    })
    const onDraftChange = vi.fn()
    const { container } = draw({ onDraftChange })
    const pill = container.querySelector('.pill')!
    const file = new File(['bytes'], 'b.pdf', { type: 'application/pdf' })

    fireEvent.drop(pill, { dataTransfer: { types: ['Files'], files: [file] } })

    await waitFor(() => expect(uploadChatAttachment).toHaveBeenCalledWith('w1', 'c1', { file }))
    await waitFor(() => {
      const lastCall = onDraftChange.mock.calls.at(-1)?.[0] as string | undefined
      expect(lastCall).toContain('[b.pdf](chats/c1/attachments/y-b.pdf)')
      expect(lastCall).not.toContain('![b.pdf]')
    })
  })

  it('ignores a drop that carries no files', () => {
    const { container } = draw()
    const pill = container.querySelector('.pill')!

    fireEvent.drop(pill, { dataTransfer: { types: ['text/plain'], files: [] } })

    expect(uploadChatAttachment).not.toHaveBeenCalled()
  })

  // The real Tauri desktop path (Task 27): a native OS drop never reaches the
  // DOM as a `drop` event at all, only as this hook's own callback — see
  // useTauriFileDrop's own note on why. Exercised via the stubbed hook above.
  it('uploads each Tauri-reported host path and clears the drag-over state', async () => {
    vi.mocked(uploadChatAttachment).mockResolvedValue({
      ref: 'chats/c1/attachments/z-c.txt',
      filename: 'c.txt',
      size: 5,
      contentType: 'text/plain',
    })
    const onDraftChange = vi.fn()
    const { container } = draw({ onDraftChange })
    const pill = container.querySelector('.pill')!
    fireEvent.dragOver(pill, { dataTransfer: { types: ['Files'] } })
    expect(pill).toHaveClass('drop-target')

    const onDrop = vi.mocked(useTauriFileDrop).mock.calls.at(-1)?.[1]
    expect(onDrop).toBeTypeOf('function')
    await act(async () => {
      onDrop?.(['/Users/me/c.txt'])
    })

    expect(pill).not.toHaveClass('drop-target')
    expect(uploadChatAttachment).toHaveBeenCalledWith('w1', 'c1', { path: '/Users/me/c.txt' })
    await waitFor(() => {
      const lastCall = onDraftChange.mock.calls.at(-1)?.[0] as string | undefined
      expect(lastCall).toContain('[c.txt](chats/c1/attachments/z-c.txt)')
    })
  })
})
