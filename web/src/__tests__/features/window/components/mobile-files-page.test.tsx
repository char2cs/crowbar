import { TOUCH_ROW_METRICS } from '@/components/sidebar/lib/row-metrics'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MobileFilesPage } from '@/features/window/components/mobile-files-page'
import type { NativeFiles } from '@/lib/native-chrome'

type Shell = { webkit?: { messageHandlers: { files: { postMessage: (m: unknown) => void } } } }
type Receiver = { __crowbarFiles?: (snapshot: NativeFiles) => void }

const posted = vi.fn()
const modified = { colorClassName: 'text-git-modified', label: 'Modified', statusLetter: 'M' }
const snapshot: NativeFiles = {
  rootName: 'checkout',
  loading: false,
  compactFolders: false,
  activePath: '/r/README.md',
  expanded: ['/r/src'],
  files: [
    {
      name: 'src',
      path: '/r/src',
      isDir: true,
      git: modified,
      children: [{ name: 'pricing.ts', path: '/r/src/pricing.ts', isDir: false, git: modified }],
    },
    { name: 'docs', path: '/r/docs', isDir: true, git: null, children: [] },
    { name: 'README.md', path: '/r/README.md', isDir: false, git: null },
  ],
}
const push = (next: NativeFiles) => act(() => (window as Receiver).__crowbarFiles!(next))
const names = () => screen.queryAllByRole('treeitem').map((el) => el.textContent)

beforeEach(() => {
  posted.mockClear()
  // jsdom lays nothing out; the list virtualises over a phone-sized viewport.
  vi.spyOn(HTMLElement.prototype, 'offsetWidth', 'get').mockReturnValue(402)
  vi.spyOn(HTMLElement.prototype, 'offsetHeight', 'get').mockReturnValue(800)
  ;(window as Shell).webkit = { messageHandlers: { files: { postMessage: posted } } }
})
afterEach(() => {
  vi.restoreAllMocks()
  delete (window as Shell).webkit
})

describe('MobileFilesPage', () => {
  it('asks the shell for the tree once it can draw it', () => {
    render(<MobileFilesPage />)
    expect(posted).toHaveBeenCalledWith({ type: 'filesReady' })
  })

  it('draws the expanded tree with its git marks and the open file selected', () => {
    render(<MobileFilesPage />)
    push(snapshot)
    expect(screen.getByRole('heading', { name: 'checkout' })).toBeVisible()
    expect(names()).toEqual(['srcM', 'pricing.tsM', 'docs', 'README.md'])
    expect(screen.getByRole('treeitem', { name: 'README.md' })).toHaveAttribute(
      'aria-selected',
      'true',
    )
    expect(screen.getAllByLabelText('Modified')).toHaveLength(2)
  })

  it('draws every row at the chat list’s touch height', () => {
    render(<MobileFilesPage />)
    push(snapshot)
    const heights = screen.getAllByRole('treeitem').map((el) => el.style.height)
    expect(new Set(heights)).toEqual(new Set([`${TOUCH_ROW_METRICS.height}px`]))
  })

  it('asks to fold a folder and to open a file', () => {
    render(<MobileFilesPage />)
    push(snapshot)
    fireEvent.click(screen.getByRole('treeitem', { name: 'docs' }))
    expect(posted).toHaveBeenCalledWith({ type: 'toggleDir', path: '/r/docs' })
    fireEvent.click(screen.getByRole('treeitem', { name: /pricing\.ts/ }))
    expect(posted).toHaveBeenCalledWith({ type: 'openFile', path: '/r/src/pricing.ts' })
  })

  it('narrows the tree to what the search names, and says when nothing matches', () => {
    render(<MobileFilesPage />)
    push({ ...snapshot, expanded: [] })
    const search = screen.getByRole('searchbox', { name: 'Search files' })
    fireEvent.change(search, { target: { value: 'pric' } })
    expect(names()).toEqual(['srcM', 'pricing.tsM'])
    fireEvent.change(search, { target: { value: 'zzz' } })
    expect(screen.getByText('No files match “zzz”')).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'Clear search' }))
    expect(names()).toEqual(['srcM', 'docs', 'README.md'])
  })

  // No workspace has no root, which is not the same as a workspace with nothing in it.
  it('says why there are no files when no workspace is selected', () => {
    render(<MobileFilesPage />)
    push({ ...snapshot, rootName: '', files: [], expanded: [] })
    expect(screen.getByText(/open a chat in a workspace/i)).toBeVisible()
    expect(screen.queryByText('No files')).not.toBeInTheDocument()
  })

  it('says it is loading before the first listing lands', () => {
    render(<MobileFilesPage />)
    push({ ...snapshot, files: [], loading: true })
    expect(screen.getByText('Loading files…')).toBeVisible()
  })
})

describe('MobileFilesPage over a long tree', () => {
  it('scrolls the open file into view', async () => {
    const files = Array.from({ length: 200 }, (_, i) => ({
      name: `f${i}.ts`,
      path: `/r/f${i}.ts`,
      isDir: false,
      git: null,
    }))
    // jsdom lays out and scrolls nothing: give the list its content height and catch the scroll.
    vi.spyOn(HTMLElement.prototype, 'scrollHeight', 'get').mockReturnValue(
      files.length * TOUCH_ROW_METRICS.height,
    )
    const scrollTo = vi.fn()
    Object.defineProperty(HTMLElement.prototype, 'scrollTo', {
      value: scrollTo,
      configurable: true,
    })
    render(<MobileFilesPage />)
    push({ ...snapshot, files, expanded: [], activePath: '/r/f190.ts' })
    // The row's bottom edge (191 rows down) at the bottom of the 800px viewport.
    await waitFor(() =>
      expect(scrollTo).toHaveBeenLastCalledWith(
        expect.objectContaining({ top: 191 * TOUCH_ROW_METRICS.height - 800 }),
      ),
    )
    delete (HTMLElement.prototype as Partial<HTMLElement>).scrollTo
  })
})
