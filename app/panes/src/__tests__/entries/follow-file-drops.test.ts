import { afterEach, describe, expect, it, vi } from 'vitest'
import { renderHook } from '@testing-library/react'
import { followFileDrops } from '@/entries/follow-file-drops'
import { useTauriFileDrop } from '@/features/file-system/lib/tauri-file-drop'
import type { FileDrop } from '@/sdk/file-drop'

interface ShellWindow {
  crowbar?: { post: (text: string) => void; onmessage: null }
}

function shellPage() {
  let drop: ((drop: FileDrop) => void) | undefined
  const client = {
    onFilesDropped: (listener: (drop: FileDrop) => void) => ((drop = listener), () => {}),
    open: vi.fn(),
  }
  followFileDrops(client)
  return {
    open: client.open,
    dropAt: (x: number, y: number, paths: string[]) => drop?.({ paths, x, y }),
  }
}

function boxAt(left: number, top: number, side: number): HTMLElement {
  const el = document.createElement('div')
  el.getBoundingClientRect = () => DOMRect.fromRect({ x: left, y: top, width: side, height: side })
  return el
}

afterEach(() => {
  delete (window as ShellWindow).crowbar
})

describe('followFileDrops', () => {
  it('opens the files of a drop nothing on the page takes as kept editor tabs, in drop order', () => {
    const page = shellPage()
    page.dropAt(10, 10, ['/home/u/notes.txt', '/home/u/file with spaces.md'])
    expect(page.open.mock.calls).toEqual([
      ['core.editor', {}, { file: '/home/u/notes.txt', mode: 'kept' }],
      ['core.editor', {}, { file: '/home/u/file with spaces.md', mode: 'kept' }],
    ])
  })

  it('gives a drop to the element under it and opens nothing', () => {
    ;(window as ShellWindow).crowbar = { post: () => {}, onmessage: null }
    const onDrop = vi.fn()
    renderHook(() => useTauriFileDrop({ current: boxAt(0, 300, 100) }, onDrop))
    const page = shellPage()
    page.dropAt(50, 350, ['/home/u/notes.txt'])
    expect([onDrop.mock.calls, page.open.mock.calls]).toEqual([[[['/home/u/notes.txt']]], []])
  })
})
