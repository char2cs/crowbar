import { createRef } from 'react'
import { cleanup, render } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  ExcalidrawCanvas,
  type ExcalidrawCanvasHandle,
} from '@/features/agent/composer/excalidraw-canvas'

// The real `<Excalidraw>` mounts a canvas-rendering tree that jsdom cannot
// host (it also pulls in a raw JSON import Vite's test transform doesn't
// touch when the module is externalized) — mocked here so this file can
// exercise `ExcalidrawCanvas`'s OWN glue (the imperative `save()` handle it
// exposes, and the scene/PNG payload it hands to `onSave`) without depending
// on the library's internals.
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

let latestInitialData: unknown
vi.mock('@excalidraw/excalidraw', () => ({
  Excalidraw: ({
    excalidrawAPI,
    initialData,
  }: {
    excalidrawAPI: (api: unknown) => void
    initialData?: unknown
  }) => {
    excalidrawAPI(fakeApi)
    latestInitialData = initialData
    return <div data-testid="excalidraw-mock" />
  },
  exportToBlob: exportToBlobMock,
}))

vi.mock('@excalidraw/excalidraw/index.css', () => ({}))

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
  latestInitialData = undefined
})

describe('ExcalidrawCanvas', () => {
  it('preloads Excalidraw with initialScene when one is given', () => {
    const scene = { elements: [{ id: 'preloaded' }], appState: { zoom: 2 } }
    render(<ExcalidrawCanvas onSave={vi.fn()} initialScene={scene} />)

    expect(latestInitialData).toMatchObject({ elements: scene.elements, appState: scene.appState })
  })

  it('passes no initialData when there is no initialScene', () => {
    render(<ExcalidrawCanvas onSave={vi.fn()} />)

    expect(latestInitialData).toBeUndefined()
  })

  it('hands onSave a serialized scene and a PNG file built from the exported blob, via the imperative save() handle', async () => {
    const onSave = vi.fn()
    const ref = createRef<ExcalidrawCanvasHandle>()
    render(<ExcalidrawCanvas ref={ref} onSave={onSave} />)

    await ref.current?.save()

    expect(onSave).toHaveBeenCalledTimes(1)
    const [result] = onSave.mock.calls[0]
    const scene = JSON.parse(result.sceneJson)
    expect(scene.elements).toEqual([{ id: 'el1' }])
    expect(result.pngFile).toBeInstanceOf(File)
    expect(result.pngFile.type).toBe('image/png')
    expect(exportToBlobMock).toHaveBeenCalledTimes(1)
  })

  it('save() resolves only once the caller-awaited onSave has settled', async () => {
    let resolveSave!: () => void
    const onSave = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          resolveSave = resolve
        }),
    )
    const ref = createRef<ExcalidrawCanvasHandle>()
    render(<ExcalidrawCanvas ref={ref} onSave={onSave} />)

    let settled = false
    const promise = ref.current!.save().then(() => {
      settled = true
    })

    await Promise.resolve()
    expect(settled).toBe(false)

    resolveSave()
    await promise
    expect(settled).toBe(true)
  })
})
