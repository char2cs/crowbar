import { act, createRef } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi, type MockInstance } from 'vitest'
import { askItem } from '@/__tests__/__fixtures__/ledger-rows'
import * as uploadApi from '@/features/agent/api/upload-chat-attachment'
import type { UploadedChatAttachment } from '@/features/agent/api/upload-chat-attachment'
import { DndScope } from '@/features/agent/chat/dnd-scope'
import { AgentComposer } from '@/features/agent/composer/agent-composer'
import { saveExcalidrawDesign } from '@/features/agent/composer/lib/excalidraw-design-persistence'
import {
  requestExcalidrawEdit,
  useExcalidrawEditRequests,
} from '@/features/agent/composer/lib/excalidraw-edit-requests'
import type { ParsedExcalidrawScene } from '@/features/agent/composer/plate/attachments/excalidraw-scene'
import type { ChatMarkdownEditorHandle } from '@/features/agent/composer/plate/chat-markdown-editor'
import * as tauriDrop from '@/features/file-system/lib/tauri-file-drop'
import * as reports from '@/__tests__/__fixtures__/reports'

// The real canvas mounts `@excalidraw/excalidraw`, which jsdom cannot host; its
// `onSave` is captured so a test can save the way the canvas's button would.
let latestExcalidrawOnSave: ((result: { sceneJson: string; pngFile: File }) => void) | null = null
let latestExcalidrawInitialScene: ParsedExcalidrawScene | undefined
vi.mock('@/features/agent/composer/excalidraw-canvas', () => ({
  ExcalidrawCanvas: ({
    onSave,
    initialScene,
  }: {
    onSave: (result: { sceneJson: string; pngFile: File }) => void
    initialScene?: ParsedExcalidrawScene
  }) => {
    latestExcalidrawOnSave = onSave
    latestExcalidrawInitialScene = initialScene
    return <div data-testid="excalidraw-canvas-mock" />
  },
}))

type AgentComposerProps = Parameters<typeof AgentComposer>[0]

let upload: MockInstance<typeof uploadApi.uploadChatAttachment>
let tauriHook: MockInstance<typeof tauriDrop.useTauriFileDrop>
let toastError: ReturnType<typeof reports.watchToasts>

beforeEach(() => {
  localStorage.clear()
  latestExcalidrawInitialScene = undefined
  upload = vi.spyOn(uploadApi, 'uploadChatAttachment')
  // A no-op under jsdom (`isTauri()` is false); its `onDrop` is invoked directly.
  tauriHook = vi.spyOn(tauriDrop, 'useTauriFileDrop').mockImplementation(() => {})
  toastError = reports.watchToasts()
})

afterEach(() => vi.restoreAllMocks())

function uploaded(ref: string, filename: string, contentType: string): UploadedChatAttachment {
  return { ref, filename, size: 10, contentType }
}

function draw(overrides: Partial<AgentComposerProps> = {}, { dnd = false } = {}) {
  const editorRef = createRef<ChatMarkdownEditorHandle>()
  const props: AgentComposerProps = {
    wsId: 'w1',
    chatId: 'c1',
    asks: [],
    providerLabel: 'Claude',
    live: true,
    working: false,
    compacting: false,
    sending: false,
    submitUnavailable: false,
    canStop: false,
    placement: 'dock',
    editorRef,
    slashOpen: false,
    onShapeChange: vi.fn(),
    onKeyDown: vi.fn(),
    onSend: vi.fn(),
    onStop: vi.fn(),
    onOpenTerminal: vi.fn(),
    ...overrides,
  }
  const tree = (more: Partial<AgentComposerProps>) =>
    dnd ? (
      <DndScope>
        <AgentComposer {...props} {...more} />
      </DndScope>
    ) : (
      <AgentComposer {...props} {...more} />
    )
  const result = render(tree({}))
  return {
    ...result,
    pill: () => result.container.querySelector('.pill')!,
    markdown: () => editorRef.current?.getMarkdown() ?? '',
    load: (text: string) => act(async () => editorRef.current?.setMarkdown(text)),
    rerenderWith: (more: Partial<AgentComposerProps>) => result.rerender(tree(more)),
  }
}

async function openPlus(item: RegExp) {
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: /add to this message/i }))
  await user.click(await screen.findByRole('menuitem', { name: item }))
  return user
}

describe('AgentComposer', () => {
  it('passes sending through to the handle as an input', () => {
    const { container } = draw({ sending: true })

    expect(container.querySelector('[data-flicker-spinner]')).toBeInTheDocument()
  })

  it('enables send from the editor shape, and sends on a click', async () => {
    const onSend = vi.fn()
    const { load } = draw({ onSend })
    expect(screen.getByRole('button', { name: 'Send prompt' })).toBeDisabled()

    await load('hi')
    fireEvent.click(screen.getByRole('button', { name: 'Send prompt' }))

    expect(onSend).toHaveBeenCalledTimes(1)
    await load('')
    expect(screen.getByRole('button', { name: 'Send prompt' })).toBeDisabled()
  })

  it('opens the real excalidraw takeover from the plus button and restores the bar on close', async () => {
    const { load } = draw()
    await load('hi')

    const user = await openPlus(/new drawing/i)
    expect(await screen.findByTestId('excalidraw-takeover')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Close' }))

    await waitFor(() => expect(screen.queryByTestId('excalidraw-takeover')).toBeNull())
    expect(screen.getByRole('button', { name: 'Send prompt' })).toBeEnabled()
  })

  // Inline under `.dock` (absolutely positioned) the takeover was sized to the dock.
  it('portals the takeover into takeoverContainer when one is given, not inline under the composer', async () => {
    const container = document.createElement('div')
    document.body.appendChild(container)
    const { container: composerRoot } = draw({ takeoverContainer: container })

    await openPlus(/new drawing/i)
    await waitFor(() =>
      expect(container.querySelector('[data-testid="excalidraw-takeover"]')).not.toBeNull(),
    )
    expect(composerRoot.querySelector('[data-testid="excalidraw-takeover"]')).toBeNull()
    document.body.removeChild(container)
  })

  it('preloads the takeover with this chat’s previously saved local design', async () => {
    saveExcalidrawDesign('w1', 'c1', '{"elements":[{"id":"saved"}],"appState":{}}')
    draw()

    await openPlus(/new drawing/i)
    await screen.findByTestId('excalidraw-canvas-mock')
    expect(latestExcalidrawInitialScene).toEqual({ elements: [{ id: 'saved' }], appState: {} })
  })

  it('opens blank when there is no previously saved local design', async () => {
    draw()

    await openPlus(/new drawing/i)
    await screen.findByTestId('excalidraw-canvas-mock')
    expect(latestExcalidrawInitialScene).toBeUndefined()
  })

  // A transcript diagram's Edit reaches the composer only through this store.
  it('opens the takeover preloaded when the store requests an excalidraw edit for this chat, and clears the request', async () => {
    const scene: ParsedExcalidrawScene = { elements: [{ id: 'from-agent' }], appState: {} }
    draw()

    act(() => requestExcalidrawEdit('c1', scene))

    await screen.findByTestId('excalidraw-takeover')
    expect(latestExcalidrawInitialScene).toEqual(scene)
    expect(useExcalidrawEditRequests.getState().requests['c1']).toBeUndefined()
  })

  it('ignores an edit request queued for a different chat', () => {
    draw()

    act(() => requestExcalidrawEdit('c2', { elements: [], appState: {} }))

    expect(screen.queryByTestId('excalidraw-takeover')).not.toBeInTheDocument()
    expect(useExcalidrawEditRequests.getState().requests['c2']).toBeDefined()
  })

  it('uploads via the attach-file modal and inserts the markdown into the field', async () => {
    upload.mockResolvedValueOnce(
      uploaded('chats/c1/attachments/x-notes.txt', 'notes.txt', 'text/plain'),
    )
    const { markdown } = draw()

    await openPlus(/attach files/i)
    fireEvent.change(screen.getByLabelText('Choose files'), {
      target: { files: [new File(['hi'], 'notes.txt', { type: 'text/plain' })] },
    })
    await screen.findByText(/Uploaded/)
    fireEvent.click(screen.getByRole('button', { name: /^Attach 1 file/ }))

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    await waitFor(() =>
      expect(markdown()).toContain('[notes.txt](chats/c1/attachments/x-notes.txt)'),
    )
  })

  // Under the real DndScope the inserted fence resolves to a draggable preview.
  it('uploads via the excalidraw takeover and inserts both markdown blocks into the field', async () => {
    upload.mockResolvedValueOnce(
      uploaded('chats/c1/attachments/x-diagram.png', 'x-diagram.png', 'image/png'),
    )
    const { markdown } = draw({}, { dnd: true })

    await openPlus(/new drawing/i)
    await screen.findByTestId('excalidraw-canvas-mock')
    const pngFile = new File(['x'], 'diagram.png', { type: 'image/png' })
    await act(async () => {
      await latestExcalidrawOnSave?.({ sceneJson: '{"elements":[]}', pngFile })
    })

    await waitFor(() => expect(screen.queryByTestId('excalidraw-takeover')).toBeNull())
    expect(markdown()).toContain('excalidraw:')
    expect(markdown()).toContain('![diagram](chats/c1/attachments/x-diagram.png)')
  })

  it('renders every pending choice instead of the field', () => {
    draw({ asks: [askItem({ id: 'k1' }), askItem({ id: 'k2', seq: 2, toolName: 'Write' })] })

    expect(screen.getAllByTestId('agent-choice-prompt')).toHaveLength(2)
    expect(screen.queryByRole('textbox')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Send prompt' })).toBeNull()
  })

  it('keeps what was typed while an ask holds the bar, and gives the box focus back', async () => {
    const { load, markdown, rerenderWith } = draw()
    await load('half a thought')

    rerenderWith({ asks: [askItem({ turnId: 't1' })] })
    expect(screen.queryByRole('textbox')).toBeNull()
    act(() => screen.getAllByRole('button')[0]?.focus())
    rerenderWith({ asks: [] })

    expect(screen.getByRole('textbox', { name: 'Message the agent' })).toHaveFocus()
    expect(markdown()).toBe('half a thought')
  })

  it('leaves focus where the person moved it when an ask goes', async () => {
    const { load, rerenderWith } = draw()
    await load('half a thought')
    rerenderWith({ asks: [askItem({ turnId: 't1' })] })
    const elsewhere = document.body.appendChild(document.createElement('input'))
    act(() => elsewhere.focus())

    rerenderWith({ asks: [] })

    expect(elsewhere).toHaveFocus()
    elsewhere.remove()
  })

  it('keeps the field mounted while compacting, queuing behind the busy provider', () => {
    draw({ compacting: true })

    expect(screen.getByRole('textbox', { name: 'Message the agent' })).toBeInTheDocument()
  })

  it('marks the pill multiline once the field grows past two lines', () => {
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
      height: 60,
      width: 0,
    } as DOMRect)
    const { pill } = draw()

    expect(pill()).toHaveClass('multi')
  })

  it('places the same editor in the document with its own controls and banner', () => {
    draw({ placement: 'document', controls: <span data-testid="chips" /> })

    expect(screen.getByRole('textbox', { name: 'Describe the change' })).toBeInTheDocument()
    expect(screen.getByTestId('agent-empty-document')).toContainElement(screen.getByTestId('chips'))
    expect(screen.getByRole('button', { name: /add to this message/i })).toBeInTheDocument()
  })
})

describe('AgentComposer drag-and-drop', () => {
  const drop = (target: Element, files: File[]) =>
    fireEvent.drop(target, { dataTransfer: { types: ['Files'], files } })

  it('shows a drag-over state while a file is dragged over the pill', () => {
    const { pill } = draw()

    fireEvent.dragOver(pill(), { dataTransfer: { types: ['Files'] } })
    expect(pill()).toHaveClass('drop-target')

    fireEvent.dragLeave(pill())
    expect(pill()).not.toHaveClass('drop-target')
  })

  it('ignores a drag and a drop that carry no files', () => {
    const { pill } = draw()

    fireEvent.dragOver(pill(), { dataTransfer: { types: ['text/plain'] } })
    fireEvent.drop(pill(), { dataTransfer: { types: ['text/plain'], files: [] } })

    expect(pill()).not.toHaveClass('drop-target')
    expect(upload).not.toHaveBeenCalled()
  })

  // A leave onto the pill's own child is not a leave: the border would flicker.
  it('keeps the drag-over state when drag-leave lands on a still-contained child', () => {
    const { pill } = draw()
    fireEvent.dragOver(pill(), { dataTransfer: { types: ['Files'] } })

    // jsdom's DragEvent drops an init `relatedTarget`; React reads this one.
    const leave = new Event('dragleave', { bubbles: true, cancelable: false })
    Object.defineProperty(leave, 'relatedTarget', { value: pill(), configurable: true })
    fireEvent(pill(), leave)

    expect(pill()).toHaveClass('drop-target')
  })

  it('uploads a dropped image and inserts image markdown, clearing the drag-over state', async () => {
    upload.mockResolvedValue(uploaded('chats/c1/attachments/x-a.png', 'a.png', 'image/png'))
    const { pill, markdown } = draw({}, { dnd: true })
    const file = new File(['bytes'], 'a.png', { type: 'image/png' })

    fireEvent.dragOver(pill(), { dataTransfer: { types: ['Files'] } })
    drop(pill(), [file])

    expect(pill()).not.toHaveClass('drop-target')
    await waitFor(() => expect(upload).toHaveBeenCalledWith('w1', 'c1', { file }))
    await waitFor(() => expect(markdown()).toContain('![a.png](chats/c1/attachments/x-a.png)'))
  })

  it('uploads a file dropped over the document placement', async () => {
    upload.mockResolvedValue(uploaded('chats/c1/attachments/y-b.pdf', 'b.pdf', 'application/pdf'))
    const { markdown } = draw({ placement: 'document' })
    const file = new File(['bytes'], 'b.pdf', { type: 'application/pdf' })

    drop(screen.getByTestId('agent-empty-document'), [file])

    await waitFor(() => expect(markdown()).toContain('[b.pdf](chats/c1/attachments/y-b.pdf)'))
  })

  it('resolves a small, well-formed dropped CSV inline as a markdown table, without uploading', async () => {
    const { pill, markdown } = draw()

    drop(pill(), [new File(['name,age\nAda,36\n'], 'people.csv', { type: 'text/csv' })])

    await waitFor(() => expect(markdown()).toContain('| name | age |'))
    expect(upload).not.toHaveBeenCalled()
  })

  // On the real editable, slate-react's own drop would also insert the raw CSV
  // text unless the editor's drop guard claims the event.
  it('drops a CSV onto the real editable node without duplicating its raw text anywhere in the document', async () => {
    const { container, markdown } = draw()
    const editable = container.querySelector('[data-slate-editor]')!
    const csvText = 'name,age\nAda,36\n'
    const file = new File([csvText], 'people.csv', { type: 'text/csv' })
    const dataTransfer = {
      types: ['Files', 'text/plain'],
      files: [file],
      items: [],
      getData: (type: string) => (type === 'text/plain' ? csvText : ''),
    }
    // jsdom implements no caretRangeFromPoint, which slate-react's drop needs.
    document.caretRangeFromPoint = () => {
      const range = document.createRange()
      range.setStart(editable, 0)
      range.collapse(true)
      return range
    }

    const dropEvent = new Event('drop', { bubbles: true, cancelable: true })
    Object.defineProperty(dropEvent, 'dataTransfer', { value: dataTransfer, configurable: true })
    Object.defineProperty(dropEvent, 'clientX', { value: 1, configurable: true })
    Object.defineProperty(dropEvent, 'clientY', { value: 1, configurable: true })
    fireEvent(editable, dropEvent)

    await waitFor(() => expect(markdown()).toContain('| name | age |'))
    expect(upload).not.toHaveBeenCalled()
    const cells = container.querySelectorAll('td, th')
    expect(Array.from(cells).map((cell) => cell.textContent)).toEqual(['name', 'age', 'Ada', '36'])
    expect(markdown()).not.toContain('name,age')
    expect(markdown().match(/\bAda\b/g)).toHaveLength(1)
  })

  it('falls through to uploading a CSV that is too malformed to inline', async () => {
    upload.mockResolvedValue(
      uploaded('chats/c1/attachments/x-ragged.csv', 'ragged.csv', 'text/csv'),
    )
    const { pill, markdown } = draw()
    const file = new File(['a,b\n1\n'], 'ragged.csv', { type: 'text/csv' })

    drop(pill(), [file])

    await waitFor(() => expect(upload).toHaveBeenCalledWith('w1', 'c1', { file }))
    await waitFor(() =>
      expect(markdown()).toContain('[ragged.csv](chats/c1/attachments/x-ragged.csv)'),
    )
  })

  it('uploads a dropped non-image file and inserts link markdown', async () => {
    upload.mockResolvedValue(uploaded('chats/c1/attachments/y-b.pdf', 'b.pdf', 'application/pdf'))
    const { pill, markdown } = draw()

    drop(pill(), [new File(['bytes'], 'b.pdf', { type: 'application/pdf' })])

    await waitFor(() => expect(markdown()).toContain('[b.pdf](chats/c1/attachments/y-b.pdf)'))
    expect(markdown()).not.toContain('![b.pdf]')
  })

  // A native OS drop reaches the app only through the Tauri hook's callback.
  it('uploads each Tauri-reported host path and clears the drag-over state', async () => {
    upload.mockResolvedValue(uploaded('chats/c1/attachments/z-c.txt', 'c.txt', 'text/plain'))
    const { pill, markdown } = draw()
    fireEvent.dragOver(pill(), { dataTransfer: { types: ['Files'] } })

    const onDrop = tauriHook.mock.calls.at(-1)?.[1]
    await act(async () => onDrop?.(['/Users/me/c.txt']))

    expect(pill()).not.toHaveClass('drop-target')
    expect(upload).toHaveBeenCalledWith('w1', 'c1', { path: '/Users/me/c.txt' })
    await waitFor(() => expect(markdown()).toContain('[c.txt](chats/c1/attachments/z-c.txt)'))
  })

  // A fresh handler per render would re-subscribe Tauri's drop channel over IPC.
  it('passes the same onDrop identity to useTauriFileDrop across a re-render', () => {
    const { rerenderWith } = draw()
    const first = tauriHook.mock.calls.at(-1)?.[1]

    rerenderWith({ working: true })

    expect(tauriHook.mock.calls.at(-1)?.[1]).toBe(first)
  })

  it('surfaces a failed upload as a toast and leaves nothing of it in the box', async () => {
    upload.mockRejectedValueOnce(reports.refusal('413 Payload Too Large'))
    const { pill, markdown } = draw({}, { dnd: true })

    drop(pill(), [new File(['bytes'], 'huge.png', { type: 'image/png' })])

    await waitFor(() =>
      reports.expectReported(toastError, 'Could not attach that file', '413 Payload Too Large'),
    )
    expect(markdown()).not.toContain('blob:')
    expect(markdown()).not.toContain('huge.png')
  })

  it('an upload refused while the daemon migrates shows the migrating state', async () => {
    upload.mockRejectedValueOnce(reports.migratingRefusal())
    const { pill } = draw()

    drop(pill(), [new File(['bytes'], 'huge.png', { type: 'image/png' })])

    await waitFor(() => reports.expectMigratingShown(toastError))
  })

  // An upload that outlives the pane must not throw when it tries to insert.
  it('does not throw when the composer unmounts before an in-flight upload resolves', async () => {
    let resolveUpload: ((value: UploadedChatAttachment) => void) | undefined
    upload.mockImplementation(() => new Promise((resolve) => (resolveUpload = resolve)))
    const { pill, unmount } = draw()

    drop(pill(), [new File(['bytes'], 'a.png', { type: 'image/png' })])
    unmount()

    await act(async () =>
      resolveUpload?.(uploaded('chats/c1/attachments/x-a.png', 'a.png', 'image/png')),
    )
    expect(toastError).not.toHaveBeenCalled()
  })

  // Attachment reordering (dnd-kit) must not swallow a native file drop.
  it('still uploads a dropped native file over the pill with a real attachment drag source mounted', async () => {
    upload.mockResolvedValue(uploaded('chats/c1/attachments/y-b.pdf', 'b.pdf', 'application/pdf'))
    const { pill, markdown, load } = draw({}, { dnd: true })
    await load('```text-attachment:AbC123xy\nsome pasted text\n```')
    expect(screen.getByRole('button', { name: /reorder this attachment/i })).toBeInTheDocument()

    fireEvent.dragOver(pill(), { dataTransfer: { types: ['Files'] } })
    drop(pill(), [new File(['bytes'], 'b.pdf', { type: 'application/pdf' })])

    await waitFor(() => expect(markdown()).toContain('[b.pdf](chats/c1/attachments/y-b.pdf)'))
    expect(upload).toHaveBeenCalledTimes(1)
  })

  // A CLI parked on its own prompt holds the prompt sent to it: the bar must still stop it.
  it('lets a prompt held behind a terminal wait be stopped from the signpost', () => {
    const onStop = vi.fn()
    draw({ terminalWait: { kind: 'workspace_trust' }, sending: true, canStop: true, onStop })
    fireEvent.click(screen.getByRole('button', { name: 'Stop this turn' }))
    expect(onStop).toHaveBeenCalled()
  })
})
