import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ExcalidrawCanvas } from '@/features/agent/composer/excalidraw-canvas'

// The real `<Excalidraw>` mounts a canvas-rendering tree that jsdom cannot
// host (it also pulls in a raw JSON import Vite's test transform doesn't
// touch when the module is externalized) — mocked here so this file can
// exercise `ExcalidrawCanvas`'s OWN glue (the API-null guard, the
// save-in-flight disabling, and the scene/PNG payload it hands to `onSave`)
// without depending on the library's internals.
const fakeApi = {
  getSceneElements: () => [{ id: 'el1' }],
  getAppState: () => ({ zoom: 1 }),
  getFiles: () => ({}),
}

// `vi.hoisted`, not a plain `const`: `vi.mock`'s factory below is hoisted
// above ordinary top-level statements, and references it directly (not from
// inside a closure, unlike `fakeApi` above) — a plain `const` would still be
// in its temporal dead zone when the factory runs.
const exportToBlobMock = vi.hoisted(() =>
  vi.fn(async () => new Blob(['png-bytes'], { type: 'image/png' })),
)

vi.mock('@excalidraw/excalidraw', () => ({
  Excalidraw: ({ excalidrawAPI }: { excalidrawAPI: (api: unknown) => void }) => {
    excalidrawAPI(fakeApi)
    return <div data-testid="excalidraw-mock" />
  },
  exportToBlob: exportToBlobMock,
}))

vi.mock('@excalidraw/excalidraw/index.css', () => ({}))

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe('ExcalidrawCanvas', () => {
  it('calls onCancel when Cancel is clicked', () => {
    const onCancel = vi.fn()
    render(<ExcalidrawCanvas onCancel={onCancel} onSave={vi.fn()} />)

    fireEvent.click(screen.getByText('Cancel'))

    expect(onCancel).toHaveBeenCalledTimes(1)
  })

  it('hands onSave a serialized scene and a PNG file built from the exported blob', async () => {
    const onSave = vi.fn()
    render(<ExcalidrawCanvas onCancel={vi.fn()} onSave={onSave} />)

    fireEvent.click(screen.getByText('Save'))

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1))
    const [result] = onSave.mock.calls[0]
    const scene = JSON.parse(result.sceneJson)
    expect(scene.elements).toEqual([{ id: 'el1' }])
    expect(result.pngFile).toBeInstanceOf(File)
    expect(result.pngFile.type).toBe('image/png')
    expect(exportToBlobMock).toHaveBeenCalledTimes(1)
  })

  it('disables Save while a save is in flight', async () => {
    let resolveBlob!: (blob: Blob) => void
    exportToBlobMock.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveBlob = resolve as (blob: Blob) => void
        }),
    )
    render(<ExcalidrawCanvas onCancel={vi.fn()} onSave={vi.fn()} />)

    fireEvent.click(screen.getByText('Save'))

    await waitFor(() => expect(screen.getByText('Saving…')).toBeInTheDocument())
    expect(screen.getByText('Saving…').closest('button')).toBeDisabled()
    expect(screen.getByText('Cancel').closest('button')).toBeDisabled()

    resolveBlob(new Blob(['x'], { type: 'image/png' }))
    await waitFor(() => expect(screen.getByText('Save')).toBeInTheDocument())
  })

  // TestRegression: `handleSave` used to call `onSave(...)` without an
  // `await` — `finally { setSaving(false) }` ran (re-enabling Save) the
  // instant the PNG was exported, WHILE the real `onSave` (an async upload,
  // in `ExcalidrawModal`) was still in flight. A fast double-click fired it
  // twice before the first resolved: two `nanoid()`s, two uploads, a
  // duplicate fence+image pair inserted.
  it('TestRegression_staysDisabledUntilAnAsyncOnSaveResolves_soADoubleClickOnlyFiresOnce', async () => {
    let resolveSave!: () => void
    const onSave = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          resolveSave = resolve
        }),
    )
    render(<ExcalidrawCanvas onCancel={vi.fn()} onSave={onSave} />)

    const saveButton = screen.getByText('Save').closest('button')!
    fireEvent.click(saveButton)
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1))

    // The upload is still pending — Save must still be disabled, so a second,
    // fast click cannot fire a second onSave (and thus a second nanoid/upload).
    await waitFor(() => expect(screen.getByText('Saving…').closest('button')).toBeDisabled())
    fireEvent.click(screen.getByText('Saving…').closest('button')!)
    expect(onSave).toHaveBeenCalledTimes(1)

    resolveSave()
    await waitFor(() => expect(screen.getByText('Save')).toBeInTheDocument())
    expect(onSave).toHaveBeenCalledTimes(1)
  })
})
