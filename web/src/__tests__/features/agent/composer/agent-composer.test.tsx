import { act } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { DndProvider } from 'react-dnd'
import { HTML5Backend } from 'react-dnd-html5-backend'

import { AgentComposer } from '@/features/agent/composer/agent-composer'
import {
  uploadChatAttachment,
  type UploadedChatAttachment,
} from '@/features/agent/api/upload-chat-attachment'
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

// The real canvas mounts `@excalidraw/excalidraw` (a canvas-rendering tree
// jsdom can't host, and whose dependency graph hits a raw JSON import this
// test transform doesn't touch) — mocked out here since this suite only
// proves the composer's plus button wires through to the modal, not the
// canvas's own behaviour (that's excalidraw-canvas.test.tsx's job). `onSave`
// is captured so one test below can trigger it directly, the same way a real
// Save click on the (unmocked) canvas would.
let latestExcalidrawOnSave: ((result: { sceneJson: string; pngFile: File }) => void) | null = null
vi.mock('@/features/agent/composer/excalidraw-canvas', () => ({
  ExcalidrawCanvas: ({
    onSave,
  }: {
    onSave: (result: { sceneJson: string; pngFile: File }) => void
  }) => {
    latestExcalidrawOnSave = onSave
    return <div data-testid="excalidraw-canvas-mock" />
  },
}))

const toastError = vi.hoisted(() => vi.fn())
vi.mock('@/features/window/stores/toast-store', () => ({ toast: { error: toastError } }))

type AgentComposerProps = Parameters<typeof AgentComposer>[0]

const baseProps: AgentComposerProps = {
  wsId: 'w1',
  chatId: 'c1',
  activity: NO_ACTIVITY,
  providerLabel: 'Claude',
  live: true,
  working: false,
  compacting: false,
  sending: false,
  submitUnavailable: false,
  canStop: false,
  draft: '',
  fieldHeight: 20,
  slashOpen: false,
  onDraftChange: vi.fn(),
  onHeightChange: vi.fn(),
  onKeyDown: vi.fn(),
  onSend: vi.fn(),
  onStop: vi.fn(),
  onOpenTerminal: vi.fn(),
  draftSeed: 0,
  seedText: '',
}

function draw(overrides: Partial<AgentComposerProps> = {}) {
  const props = { ...baseProps, ...overrides }
  const result = render(<AgentComposer {...props} />)
  return {
    ...result,
    // For asserting identity STABILITY (e.g. a memoized callback) across a
    // re-render with the SAME element type — `result.rerender` alone forces
    // callers to reconstruct the full props object themselves.
    rerenderWith: (moreOverrides: Partial<AgentComposerProps> = {}) =>
      result.rerender(<AgentComposer {...props} {...moreOverrides} />),
  }
}

// Same as `draw`, but under a REAL (unmocked) `<DndProvider>` — the ancestor
// production always has here (`DndScope` at `AgentChatView`, one level above
// where this suite tests `AgentComposer` in isolation). Only used by the
// review-finding test below: `react-dnd`'s `HTML5Backend` only attaches its
// global window drag/drop listeners once `refCount > 0` (see
// `DragDropManagerImpl.handleRefCountChange`, dnd-core) — i.e. once some
// `useDraggable`/`useDropLine` call has actually registered a drag source or
// drop target — so a caller also has to seed real attachment content for the
// interaction under test to be genuine, not just a `<DndProvider>` sitting
// inertly above an otherwise-empty composer.
function drawWithDnd(overrides: Partial<AgentComposerProps> = {}) {
  const props = { ...baseProps, ...overrides }
  return render(
    <DndProvider backend={HTML5Backend}>
      <AgentComposer {...props} />
    </DndProvider>,
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

  // Task 34 (ExcalidrawModal) replaces its own placeholder — opening it now
  // shows a real, modal dialog, same as Task 30's AttachFileModal below. Its
  // own behaviour (id-correlation, upload, toast-on-failure) is covered in
  // excalidraw-modal.test.tsx; this only proves the composer wires the plus
  // button through to it, and that the bar is itself again once closed.
  it('opens the real excalidraw modal from the plus button and restores the bar on close', async () => {
    const user = userEvent.setup()
    const onSend = vi.fn()
    draw({ draft: 'hi', onSend })

    await user.click(screen.getByRole('button', { name: /add to this message/i }))
    await user.click(await screen.findByRole('menuitem', { name: /excalidraw/i }))

    expect(await screen.findByRole('dialog')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Excalidraw' })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Close' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())

    fireEvent.click(screen.getByRole('button', { name: 'Send prompt' }))
    expect(onSend).toHaveBeenCalledTimes(1)
  })

  // Task 30 (AttachFileModal) replaces its own placeholder — opening it now
  // shows a real, modal dialog, which correctly makes the rest of the bar
  // inert (Base UI marks background content aria-hidden) until it closes.
  // Its own behaviour (upload paths, drag-drop, the error toast) is covered
  // in attach-file-modal.test.tsx; this only proves the composer wires the
  // plus button through to it, and that the bar is itself again once closed.
  it('opens the real attach-file modal from the plus button and restores the bar on close', async () => {
    const user = userEvent.setup()
    const onSend = vi.fn()
    draw({ draft: 'hi', onSend })

    await user.click(screen.getByRole('button', { name: /add to this message/i }))
    await user.click(await screen.findByRole('menuitem', { name: /attach file/i }))

    expect(await screen.findByRole('dialog')).toBeInTheDocument()
    expect(screen.getByText(/attach a file/i)).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Close' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())

    fireEvent.click(screen.getByRole('button', { name: 'Send prompt' }))
    expect(onSend).toHaveBeenCalledTimes(1)
  })

  // The modal's own upload/markdown behaviour is attach-file-modal.test.tsx's
  // job; this proves the composer's own `onInsertMarkdown` wiring actually
  // reaches the field's imperative handle (and the modal closes itself once
  // it does), not just that the dialog opens.
  it('uploads via the attach-file modal and inserts the markdown into the field', async () => {
    const user = userEvent.setup()
    vi.mocked(uploadChatAttachment).mockResolvedValueOnce({
      ref: 'chats/c1/attachments/x-notes.txt',
      filename: 'notes.txt',
      size: 5,
      contentType: 'text/plain',
    })
    const onDraftChange = vi.fn()
    draw({ onDraftChange })

    await user.click(screen.getByRole('button', { name: /add to this message/i }))
    await user.click(await screen.findByRole('menuitem', { name: /attach file/i }))

    const input = screen.getByLabelText(/choose a file/i) as HTMLInputElement
    fireEvent.change(input, {
      target: { files: [new File(['hi'], 'notes.txt', { type: 'text/plain' })] },
    })

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    await waitFor(() => {
      const lastCall = onDraftChange.mock.calls.at(-1)?.[0] as string | undefined
      expect(lastCall).toContain('[notes.txt](chats/c1/attachments/x-notes.txt)')
    })
  })

  // The modal's own id-correlation/upload/toast behaviour is
  // excalidraw-modal.test.tsx's job; this proves the composer's own
  // `onInsertMarkdown` wiring reaches the field's imperative handle for
  // EACH of the two markdown blocks a save produces (the fence, then the
  // sibling image), not just that the dialog opens.
  it('uploads via the excalidraw modal and inserts both markdown blocks into the field', async () => {
    const user = userEvent.setup()
    vi.mocked(uploadChatAttachment).mockResolvedValueOnce({
      ref: 'chats/c1/attachments/x-diagram.png',
      filename: 'x-diagram.png',
      size: 10,
      contentType: 'image/png',
    })
    const onDraftChange = vi.fn()
    draw({ onDraftChange })

    await user.click(screen.getByRole('button', { name: /add to this message/i }))
    await user.click(await screen.findByRole('menuitem', { name: /excalidraw/i }))
    await screen.findByTestId('excalidraw-canvas-mock')

    const pngFile = new File(['x'], 'diagram.png', { type: 'image/png' })
    await act(async () => {
      await latestExcalidrawOnSave?.({ sceneJson: '{"elements":[]}', pngFile })
    })

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    const calls = onDraftChange.mock.calls.map((c) => c[0] as string)
    expect(calls.some((md) => md.includes('excalidraw:'))).toBe(true)
    expect(calls.at(-1)).toContain('![diagram](chats/c1/attachments/x-diagram.png)')
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
    vi.mocked(useTauriFileDrop).mockClear()
    toastError.mockClear()
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

  // Review finding 1: an inline arrow passed to `useTauriFileDrop` would get a
  // fresh identity every render, tearing down and re-establishing Tauri's
  // `onDragDropEvent` subscription on every keystroke (this component
  // re-renders on `props.draft`/`onDraftChange`). Proven directly against the
  // stubbed hook rather than inferred from the fix.
  it('passes the same onDrop identity to useTauriFileDrop across an unrelated re-render', () => {
    const { rerenderWith } = draw({ draft: 'a' })
    const callsBefore = vi.mocked(useTauriFileDrop).mock.calls.length
    const firstOnDrop = vi.mocked(useTauriFileDrop).mock.calls.at(-1)?.[1]
    expect(firstOnDrop).toBeTypeOf('function')

    rerenderWith({ draft: 'ab' })

    expect(vi.mocked(useTauriFileDrop).mock.calls.length).toBeGreaterThan(callsBefore)
    const secondOnDrop = vi.mocked(useTauriFileDrop).mock.calls.at(-1)?.[1]
    expect(secondOnDrop).toBe(firstOnDrop)
  })

  // Review finding 2: a failed upload used to be a silent no-op (an unhandled
  // rejection, nothing inserted, nothing said). `uploadAndInsert` now catches
  // and toasts instead.
  it('surfaces a failed upload as a toast instead of silently doing nothing', async () => {
    vi.mocked(uploadChatAttachment).mockRejectedValueOnce(new Error('413 Payload Too Large'))
    const onDraftChange = vi.fn()
    const { container } = draw({ onDraftChange })
    const pill = container.querySelector('.pill')!
    const file = new File(['bytes'], 'huge.png', { type: 'image/png' })

    fireEvent.drop(pill, { dataTransfer: { types: ['Files'], files: [file] } })

    await waitFor(() =>
      expect(toastError).toHaveBeenCalledWith(
        'Could not attach that file',
        '413 Payload Too Large',
      ),
    )
    expect(onDraftChange).not.toHaveBeenCalled()
  })

  // A failure that isn't an `Error` instance (e.g. a plain thrown string, or a
  // non-Error rejection from a fetch polyfill) still gets a description, not
  // a crash.
  it('falls back to a generic description for a non-Error rejection', async () => {
    vi.mocked(uploadChatAttachment).mockRejectedValueOnce('boom')
    const { container } = draw()
    const pill = container.querySelector('.pill')!
    const file = new File(['bytes'], 'huge.png', { type: 'image/png' })

    fireEvent.drop(pill, { dataTransfer: { types: ['Files'], files: [file] } })

    await waitFor(() =>
      expect(toastError).toHaveBeenCalledWith(
        'Could not attach that file',
        'Crowbar could not reach the daemon — try again.',
      ),
    )
  })

  // Also (review, minor): the `editorRef.current?.` null-path was previously
  // unexercised. A drop that outlives the component (the pane closes, the
  // chat is switched, mid-upload) must not throw when it tries to insert.
  it('does not throw when the composer unmounts before an in-flight upload resolves', async () => {
    let resolveUpload: ((value: UploadedChatAttachment) => void) | undefined
    vi.mocked(uploadChatAttachment).mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveUpload = resolve
        }),
    )
    const { container, unmount } = draw()
    const pill = container.querySelector('.pill')!
    const file = new File(['bytes'], 'a.png', { type: 'image/png' })

    fireEvent.drop(pill, { dataTransfer: { types: ['Files'], files: [file] } })
    unmount()

    await act(async () => {
      resolveUpload?.({
        ref: 'chats/c1/attachments/x-a.png',
        filename: 'a.png',
        size: 10,
        contentType: 'image/png',
      })
    })
    // Reaching here without throwing IS the assertion — editorRef.current is
    // null post-unmount, and insertUploaded's `?.` must swallow that cleanly.
  })

  // Task 35 review, Finding 1: once `DndPlugin` is registered
  // (chat-composer-plugins.ts) and a real attachment drag source is mounted
  // (a text-attachment pill, here — `useAttachmentDraggable`), `react-dnd`'s
  // `HTML5Backend` attaches CAPTURE-phase listeners on `window` that inspect
  // EVERY native drag entering the document — not just one over the
  // attachment node itself (traced in `HTML5BackendImpl.js`:
  // `handleTopDragEnterCapture`/`handleTopDropCapture` auto-detect any native
  // file drag via `beginDragNativeItem`). Source-reading suggested this is
  // safe (`preventDefault` calls on both sides are idempotent, nothing calls
  // `stopPropagation`, and a bare `DndPlugin` registration — no
  // `onDropFiles` — means `react-dnd`'s own native-drop handling never calls
  // back into app code) but was never actually exercised. This mounts the
  // REAL (unmocked) `<DndProvider>`/`HTML5Backend`/`DndPlugin` stack —
  // proven active by asserting the drag handle actually renders, which is
  // what flips `dnd-core`'s `refCount` above zero and makes `HTML5Backend`
  // attach its listeners at all — and proves the composer's own native
  // file-drop handling over the pill (`handlePillDragOver`/`handlePillDrop`)
  // still uploads and inserts exactly once, not intercepted or duplicated.
  it('still uploads a dropped native file over the pill with a real DndProvider/DndPlugin drag source already active', async () => {
    vi.mocked(uploadChatAttachment).mockResolvedValue({
      ref: 'chats/c1/attachments/y-b.pdf',
      filename: 'b.pdf',
      size: 20,
      contentType: 'application/pdf',
    })
    const onDraftChange = vi.fn()
    const { container } = drawWithDnd({
      onDraftChange,
      seedText: '```text-attachment:AbC123xy\nsome pasted text\n```',
    })

    // Proves a real drag source registered — not just that a <DndProvider>
    // happens to sit somewhere above an otherwise-inert tree.
    expect(screen.getByRole('button', { name: /reorder this attachment/i })).toBeInTheDocument()

    const pill = container.querySelector('.pill')!
    const file = new File(['bytes'], 'b.pdf', { type: 'application/pdf' })

    // A `dragenter` first — same as every real HTML5 drag session (the spec
    // guarantees `dragenter` precedes any `dragover`/`drop` for one drag) —
    // is what lets `HTML5Backend`'s own `handleTopDragEnterCapture` call
    // `beginDragNativeItem` and start tracking the native drag; skipping it
    // (as the OTHER drag-and-drop tests above do, safely, since none of them
    // has `DndPlugin`'s listeners active) left `dnd-core`'s monitor never
    // told a drag was in progress, and its `handleTopDrop` throws its own
    // "Cannot call hover while not dragging" invariant on a bare drop — a
    // real finding from writing this test, not a bug in the fix: it is the
    // artificial test sequence, unreachable through a real OS/browser drag,
    // that trips it.
    fireEvent.dragEnter(pill, { dataTransfer: { types: ['Files'] } })
    fireEvent.dragOver(pill, { dataTransfer: { types: ['Files'] } })
    expect(pill).toHaveClass('drop-target')
    fireEvent.drop(pill, { dataTransfer: { types: ['Files'], files: [file] } })

    expect(pill).not.toHaveClass('drop-target')
    await waitFor(() => expect(uploadChatAttachment).toHaveBeenCalledWith('w1', 'c1', { file }))
    await waitFor(() => {
      const lastCall = onDraftChange.mock.calls.at(-1)?.[0] as string | undefined
      expect(lastCall).toContain('[b.pdf](chats/c1/attachments/y-b.pdf)')
    })
    // Exactly once — react-dnd's own native-item tracking must not cause a
    // second, duplicate upload through some path of its own.
    expect(uploadChatAttachment).toHaveBeenCalledTimes(1)
  })
})
