import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useFileMention } from '@/features/agent/hooks/use-file-mention'

const fetchFileTree = vi.fn()

vi.mock('@/features/files/lib/file-tree-api', () => ({
  fetchFileTree: (...args: unknown[]) => fetchFileTree(...args),
}))

const node = (path: string, isDir = false) => ({
  name: path.split('/').pop(),
  path,
  isDir,
  isDirectory: isDir,
  isFile: !isDir,
})

beforeEach(() => {
  fetchFileTree.mockReset()
  fetchFileTree.mockImplementation((_ws: string, dir?: string) =>
    Promise.resolve(
      dir === 'src'
        ? [node('src/lib', true), node('src/main.ts'), node('src/util.ts')]
        : [node('src', true), node('README.md')],
    ),
  )
})

describe('useFileMention', () => {
  it('lists the folder an @ path is in, narrowed by what follows its last slash', async () => {
    const { result } = renderHook(() => useFileMention('ws1'))

    act(() => result.current.noteQuery('src/ma'))

    await waitFor(() =>
      expect(result.current.items.map((item) => item.insertText)).toEqual(['@src/main.ts ']),
    )
    expect(fetchFileTree).toHaveBeenCalledWith('ws1', 'src')
    expect(result.current.highlighted?.insertText).toBe('@src/main.ts ')
  })

  it('opens the workspace root on a bare @ and keeps a folder open to go on typing', async () => {
    const { result } = renderHook(() => useFileMention('ws1'))

    act(() => result.current.noteQuery(''))
    await waitFor(() => expect(result.current.items).toHaveLength(2))
    expect(fetchFileTree).toHaveBeenCalledWith('ws1', undefined)
    expect(result.current.items[0]!.insertText).toBe('@src/')

    act(() => result.current.move(1))
    expect(result.current.highlighted?.insertText).toBe('@README.md ')
  })

  it('closes when the caret leaves the @ path, and on Esc until the next @', async () => {
    const { result } = renderHook(() => useFileMention('ws1'))

    act(() => result.current.noteQuery('src/'))
    await waitFor(() => expect(result.current.open).toBe(true))
    act(() => result.current.close())
    expect(result.current.open).toBe(false)
    act(() => result.current.noteQuery('src/m'))
    expect(result.current.open).toBe(false)

    act(() => result.current.noteQuery(null))
    act(() => result.current.noteQuery(''))
    expect(result.current.open).toBe(true)
  })
})
