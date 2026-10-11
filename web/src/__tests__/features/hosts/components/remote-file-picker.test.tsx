import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { StrictMode } from 'react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiFetch: vi.fn(),
}))
// Counts every row drawn, to see which rows a key press draws again.
const drawn = vi.hoisted(() => ({ rows: 0 }))
vi.mock('@/components/ui/tree-row', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/components/ui/tree-row')>()
  return {
    ...actual,
    TreeRow: (props: Parameters<typeof actual.TreeRow>[0]) => {
      drawn.rows++
      return actual.TreeRow(props)
    },
  }
})

import { ApiError, apiFetch } from '@/lib/api'
import { proxyMarkerError } from '@/lib/api/errors'
import { RemoteFilePicker } from '@/features/hosts/components/remote-file-picker'
import type { BrowseEntry } from '@/features/hosts/lib/browse'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { getOrCreateHostRuntime } from '@/features/hosts/stores/host-runtime-registry'
import type { Host, HostStatus } from '@/features/hosts/types'

function host(id: string, name: string): Host {
  return { id, name, endpoint: `${id}:3737`, pairedFingerprint: 'fp', clientId: 'c' }
}

const dir = (path: string): BrowseEntry => ({ name: path.split('/').pop()!, path, isDir: true })
const file = (path: string): BrowseEntry => ({ ...dir(path), isDir: false })

// What each host's daemon answers per path; a path with no answer stays pending until
// `answer` settles it, so a test can order responses against what the user does.
type Reply = BrowseEntry[] | Error
let served: Record<string, Record<string, Reply>> = {}
const requests: { hostId: string; path: string }[] = []
const held = new Map<string, Array<(reply: Reply) => void>>()

function serve(hostId: string, replies: Record<string, Reply>) {
  served[hostId] = { ...served[hostId], ...replies }
}

async function answer(hostId: string, path: string, reply: Reply) {
  const waiting = held.get(`${hostId}:${path}`)?.shift()
  if (!waiting) throw new Error(`no request held for ${hostId}:${path}`)
  await act(async () => waiting(reply))
}

const requested = (path: string, hostId = 'h1') =>
  requests.filter((r) => r.hostId === hostId && r.path === path).length

function setStatus(hostId: string, status: HostStatus) {
  act(() => getOrCreateHostRuntime(hostId).getState().setStatus(status))
}

beforeEach(() => {
  served = {}
  requests.length = 0
  held.clear()
  useHostConfigStore.setState({
    hosts: [host('h1', 'homelab-prox'), host('h2', 'office-vm')],
  })
  getOrCreateHostRuntime('h1').getState().setStatus('up')
  getOrCreateHostRuntime('h2').getState().setStatus('up')
  vi.mocked(apiFetch).mockReset()
  vi.mocked(apiFetch).mockImplementation((url, _init, _retry, hostId) => {
    const path = new URL(url, 'http://x').searchParams.get('path') ?? ''
    requests.push({ hostId: hostId ?? '(none)', path })
    const reply = served[hostId ?? '']?.[path]
    const settle = (r: Reply) => (r instanceof Error ? Promise.reject(r) : Promise.resolve(r))
    if (reply) return settle(reply) as Promise<never>
    return new Promise((resolve, reject) => {
      const key = `${hostId}:${path}`
      held.set(key, [
        ...(held.get(key) ?? []),
        (r) => (r instanceof Error ? reject(r) : resolve(r as never)),
      ])
    })
  })
})

// I1: whatever happens in a test, no request ever leaves for a host it was not opened on.
afterEach(() => {
  expect(requests.filter((r) => r.hostId !== 'h1' && r.hostId !== 'h2')).toEqual([])
})

function renderPicker(hostId = 'h1') {
  const onSelect = vi.fn()
  const onOpenChange = vi.fn()
  const view = render(
    <RemoteFilePicker hostId={hostId} open onOpenChange={onOpenChange} onSelect={onSelect} />,
  )
  return { onSelect, onOpenChange, ...view }
}

const row = (name: string) => screen.getByRole('treeitem', { name })
const findRow = (name: string) => screen.findByRole('treeitem', { name })
const selectButton = () => screen.getByRole('button', { name: /^select/i })
const chevron = (name: string) => row(name).querySelector('[data-slot="tree-item-chevron"]')!

const HOME = { '/': [dir('/home'), dir('/srv'), file('/notes.txt')] }

describe('RemoteFilePicker — browsing', () => {
  it("lists the host's filesystem from its root, folders before files, on that host only", async () => {
    serve('h1', { '/': [file('/notes.txt'), dir('/srv'), dir('/home')] })
    renderPicker()

    expect(screen.getByText("Browsing homelab-prox's filesystem")).toBeInTheDocument()
    await findRow('srv')
    const names = screen.getAllByRole('treeitem').map((item) => item.textContent)
    expect(names).toEqual(['srv', 'home', 'notes.txt'])
    expect(requests).toEqual([{ hostId: 'h1', path: '/' }])
  })

  it('says it is loading while the first listing is on its way', async () => {
    renderPicker()

    expect(screen.getByText('Loading folders…')).toBeInTheDocument()
    await answer('h1', '/', [dir('/home')])
    expect(await findRow('home')).toBeInTheDocument()
  })

  it('lists a folder only when it is opened, and reopens it from what was listed', async () => {
    serve('h1', { ...HOME, '/home': [dir('/home/mateo')] })
    renderPicker()
    fireEvent.click(await findRow('home'))

    expect(await findRow('mateo')).toBeInTheDocument()
    expect(requested('/home')).toBe(1)
    expect(requested('/srv')).toBe(0)

    fireEvent.click(chevron('home'))
    expect(screen.queryByRole('treeitem', { name: 'mateo' })).not.toBeInTheDocument()
    fireEvent.click(chevron('home'))
    expect(row('mateo')).toBeInTheDocument()
    expect(requested('/home')).toBe(1)
  })

  it('opening a folder many times while it is still listing asks once', async () => {
    serve('h1', HOME)
    renderPicker()
    await findRow('home')

    fireEvent.click(chevron('home'))
    fireEvent.click(chevron('home'))
    fireEvent.click(chevron('home'))
    fireEvent.click(row('home'))
    await answer('h1', '/home', [dir('/home/mateo')])

    expect(requested('/home')).toBe(1)
    expect(await findRow('mateo')).toBeInTheDocument()
  })

  it('the chevron opens a folder without choosing it', async () => {
    serve('h1', { ...HOME, '/srv': [] })
    renderPicker()
    await findRow('srv')
    fireEvent.click(chevron('srv'))

    expect(await screen.findByText('Empty folder')).toBeInTheDocument()
    expect(selectButton()).toBeDisabled()
    expect(selectButton()).toHaveAccessibleName('Select')
  })

  it('files are shown but cannot be chosen', async () => {
    serve('h1', HOME)
    const { onSelect } = renderPicker()
    fireEvent.click(await findRow('notes.txt'))

    expect(row('notes.txt')).toHaveAttribute('aria-disabled', 'true')
    expect(selectButton()).toBeDisabled()
    expect(onSelect).not.toHaveBeenCalled()
  })

  it('choosing a folder and confirming answers its path and closes', async () => {
    serve('h1', { ...HOME, '/home': [dir('/home/mateo')], '/home/mateo': [dir('/home/mateo/p')] })
    const { onSelect, onOpenChange } = renderPicker()
    fireEvent.click(await findRow('home'))
    fireEvent.click(await findRow('mateo'))

    expect(row('mateo')).toHaveAttribute('aria-selected', 'true')
    fireEvent.click(selectButton())

    expect(onSelect).toHaveBeenCalledWith('/home/mateo')
    expect(onOpenChange).toHaveBeenCalledWith(false)
    await findRow('p')
  })

  // Layout jsdom does not run: the name is cut on screen, so the button carries the whole path.
  it('a long folder name still names the whole folder on its Select button', async () => {
    const long = 'a-folder-with-a-very-long-name-'.repeat(10)
    serve('h1', { '/': [dir(`/${long}`)] })
    renderPicker()
    fireEvent.click(await findRow(long))

    expect(selectButton()).toHaveAccessibleName(`Select "${long}"`)
    expect(selectButton()).toHaveAttribute('title', `/${long}`)
  })

  it('says the root is empty rather than showing nothing', async () => {
    serve('h1', { '/': [] })
    renderPicker()
    expect(await screen.findByText('This folder is empty.')).toBeInTheDocument()
  })

  it('a folder it may not read says why on that folder, and its siblings still work', async () => {
    const denied = new ApiError('cannot read directory: open /root: permission denied', 400)
    serve('h1', { '/': [dir('/root'), dir('/srv')], '/root': denied, '/srv': [dir('/srv/git')] })
    const { onSelect } = renderPicker()
    fireEvent.click(await findRow('root'))

    expect(
      await screen.findByText("Can't open: cannot read directory: open /root: permission denied"),
    ).toBeInTheDocument()
    fireEvent.click(row('srv'))
    fireEvent.click(await findRow('git'))
    fireEvent.click(selectButton())
    expect(onSelect).toHaveBeenCalledWith('/srv/git')
  })

  it('a folder that vanished says so on that folder', async () => {
    const gone = new ApiError('cannot read path: stat /x: no such file or directory', 400)
    serve('h1', { '/': [dir('/x')], '/x': gone })
    renderPicker()
    fireEvent.click(await findRow('x'))

    expect(await screen.findByText(/^Can't open: cannot read path: .*no such file/)).toBeVisible()
  })

  it('a symlinked folder is listed through its own path, one request per opening, even in a loop', async () => {
    serve('h1', {
      '/': [dir('/loop')],
      '/loop': [dir('/loop/self')],
      '/loop/self': [dir('/loop/self/self')],
    })
    renderPicker()
    fireEvent.click(await findRow('loop'))
    fireEvent.click(await findRow('self'))
    await waitFor(() => expect(screen.getAllByRole('treeitem', { name: 'self' })).toHaveLength(2))

    expect(requests.map((r) => r.path)).toEqual(['/', '/loop', '/loop/self'])
  })

  // The daemon joins each child onto its directory; a listing that broke that must not loop.
  it("only a folder's own children nest under it, so no listing can loop", async () => {
    serve('h1', { '/': [dir('/a')], '/a': [dir('/a'), dir('/elsewhere'), dir('/a/b')] })
    renderPicker()
    fireEvent.click(await findRow('a'))

    expect(await findRow('b')).toBeInTheDocument()
    expect(screen.getAllByRole('treeitem', { name: 'a' })).toHaveLength(1)
    expect(screen.queryByRole('treeitem', { name: 'elsewhere' })).not.toBeInTheDocument()
  })
})

describe('RemoteFilePicker — keyboard', () => {
  it('arrows move, open and close; Enter chooses', async () => {
    serve('h1', { ...HOME, '/home': [dir('/home/mateo')] })
    renderPicker()
    await findRow('home')
    const tree = screen.getByRole('tree', { name: 'Folders on homelab-prox' })
    const active = () => document.getElementById(tree.getAttribute('aria-activedescendant')!)

    fireEvent.focus(tree)
    expect(active()).toBe(row('home'))
    fireEvent.keyDown(tree, { key: 'ArrowRight' })
    await findRow('mateo')
    fireEvent.keyDown(tree, { key: 'ArrowRight' })
    expect(active()).toBe(row('mateo'))
    fireEvent.keyDown(tree, { key: 'ArrowLeft' })
    expect(active()).toBe(row('home'))
    fireEvent.keyDown(tree, { key: 'ArrowLeft' })
    expect(screen.queryByRole('treeitem', { name: 'mateo' })).not.toBeInTheDocument()
    fireEvent.keyDown(tree, { key: 'End' })
    expect(active()).toBe(row('notes.txt'))
    fireEvent.keyDown(tree, { key: 'ArrowUp' })
    fireEvent.keyDown(tree, { key: 'Enter' })

    expect(selectButton()).toHaveAccessibleName('Select "srv"')
  })

  // Focus and the active row must agree, or assistive tech reads one row while keys move another.
  it('a click leaves focus on the tree, and the arrows move its active row from there', async () => {
    serve('h1', HOME)
    const user = userEvent.setup()
    renderPicker()
    const tree = await screen.findByRole('tree', { name: 'Folders on homelab-prox' })

    await user.click(row('notes.txt'))
    expect(tree).toHaveFocus()
    expect(tree).toHaveAttribute('aria-activedescendant', row('notes.txt').id)
    await user.keyboard('{ArrowUp}')

    expect(tree).toHaveFocus()
    expect(tree).toHaveAttribute('aria-activedescendant', row('srv').id)
  })

  // A big folder must stay quick to walk: a key press draws again only the rows it changed.
  it('an arrow key draws again only the row it left and the row it reached', async () => {
    serve('h1', { '/': Array.from({ length: 40 }, (_, i) => dir(`/d${i}`)) })
    renderPicker()
    const tree = await screen.findByRole('tree')
    fireEvent.focus(tree)

    drawn.rows = 0
    fireEvent.keyDown(tree, { key: 'ArrowDown' })

    expect(tree).toHaveAttribute('aria-activedescendant', row('d1').id)
    expect(drawn.rows).toBe(2)
  })

  it('opens with focus in the folder list, never on Cancel, and hands it to the tree once listed', async () => {
    renderPicker()

    const list = screen.getByRole('group', { name: 'Folder list' })
    await waitFor(() => expect(list).toHaveFocus())
    expect(list).toHaveTextContent('Loading folders…')
    await answer('h1', '/', [dir('/home')])

    expect(await screen.findByRole('tree')).toHaveFocus()
  })

  it('hands focus to the tree even when the listing answers before the dialog settles', async () => {
    serve('h1', HOME)
    renderPicker()

    const tree = await screen.findByRole('tree')
    await waitFor(() => expect(tree).toHaveFocus())
  })
})

describe('RemoteFilePicker — the host', () => {
  it('a host unreachable at open says so, marks it down, and lists once it is up again', async () => {
    renderPicker()
    await answer('h1', '/', proxyMarkerError('host-unreachable', 'h1')!)

    expect(await screen.findByRole('alert')).toHaveTextContent(
      "Couldn't list folders on homelab-prox: the remote host is unreachable",
    )
    expect(screen.getByRole('status')).toHaveTextContent('homelab-prox is not answering')
    expect(screen.getByRole('button', { name: 'Retry' })).toBeDisabled()

    serve('h1', HOME)
    setStatus('h1', 'up')
    expect(await findRow('home')).toBeInTheDocument()
  })

  it('a failure that is not the host can be retried', async () => {
    renderPicker()
    await answer('h1', '/', new ApiError('internal error', 500, undefined, undefined, 'h1'))
    serve('h1', HOME)

    fireEvent.click(await screen.findByRole('button', { name: 'Retry' }))

    expect(await findRow('home')).toBeInTheDocument()
    expect(requested('/')).toBe(2)
  })

  it('a host that goes down while open stops choosing and asking, then recovers', async () => {
    serve('h1', HOME)
    renderPicker()
    fireEvent.click(await findRow('srv'))
    await answer('h1', '/srv', [dir('/srv/git')])
    expect(selectButton()).toBeEnabled()

    setStatus('h1', 'down')
    expect(screen.getByRole('status')).toHaveTextContent('homelab-prox is not answering')
    expect(screen.getByRole('img', { name: 'Offline' })).toHaveAttribute('data-host-state', 'down')
    expect(screen.getByText('homelab-prox')).toHaveAttribute('data-host-tone', 'offline')
    expect(selectButton()).toBeDisabled()
    fireEvent.click(row('home'))
    expect(screen.getByText('Waiting for homelab-prox to answer')).toBeInTheDocument()
    expect(requested('/home')).toBe(0)

    serve('h1', { '/home': [dir('/home/mateo')] })
    setStatus('h1', 'up')
    expect(await findRow('mateo')).toBeInTheDocument()
    expect(selectButton()).toBeEnabled()
  })

  it('a folder that failed under a collapsed one is listed again when the host is up', async () => {
    serve('h1', { ...HOME, '/home': [dir('/home/mateo')] })
    renderPicker()
    fireEvent.click(await findRow('home'))
    fireEvent.click(await findRow('mateo'))
    await answer('h1', '/home/mateo', proxyMarkerError('host-unreachable', 'h1')!)
    fireEvent.click(chevron('home'))

    serve('h1', { '/home/mateo': [dir('/home/mateo/code')] })
    setStatus('h1', 'up')
    await waitFor(() => expect(requested('/home/mateo')).toBe(2))
    fireEvent.click(chevron('home'))

    expect(await findRow('code')).toBeInTheDocument()
  })

  it('a host that needs repair while open says so and cannot be chosen from', async () => {
    serve('h1', HOME)
    renderPicker()
    fireEvent.click(await findRow('srv'))

    setStatus('h1', 'needs-repair')

    expect(
      screen.getByText('homelab-prox refuses this app until it is paired again.'),
    ).toBeVisible()
    expect(selectButton()).toBeDisabled()
  })

  it('a host removed while open is no longer browsed — not as itself, not as Local', async () => {
    serve('h1', HOME)
    renderPicker()
    await findRow('home')

    act(() => useHostConfigStore.setState({ hosts: [host('h2', 'office-vm')] }))

    expect(screen.queryByRole('tree')).not.toBeInTheDocument()
    expect(screen.getByText(/no longer paired/)).toBeInTheDocument()
    expect(selectButton()).toBeDisabled()
    expect(requests).toEqual([{ hostId: 'h1', path: '/' }])
  })

  it('a host that is not configured is never asked anything', () => {
    renderPicker('ghost')

    expect(screen.getByText(/no longer paired/)).toBeInTheDocument()
    expect(requests).toEqual([])
  })
})

describe('RemoteFilePicker — late answers', () => {
  it('an answer that arrives after the picker closed is dropped, and says nothing of the host', async () => {
    const { rerender, onOpenChange, onSelect } = renderPicker()
    rerender(
      <RemoteFilePicker hostId="h1" open={false} onOpenChange={onOpenChange} onSelect={onSelect} />,
    )
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())

    await answer('h1', '/', proxyMarkerError('host-unreachable', 'h1')!)

    expect(getOrCreateHostRuntime('h1').getState().status).toBe('up')
    rerender(<RemoteFilePicker hostId="h1" open onOpenChange={onOpenChange} onSelect={onSelect} />)
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(screen.getByText('Loading folders…')).toBeInTheDocument()
    await answer('h1', '/', [dir('/home')])
    expect(await findRow('home')).toBeInTheDocument()
  })

  it('what was listed on one host is never shown, or answered, for another', async () => {
    serve('h1', { '/': [dir('/only-on-h1')] })
    const { rerender, onOpenChange, onSelect } = renderPicker('h1')
    fireEvent.click(await findRow('only-on-h1'))

    rerender(<RemoteFilePicker hostId="h2" open onOpenChange={onOpenChange} onSelect={onSelect} />)
    expect(screen.queryByRole('treeitem', { name: 'only-on-h1' })).not.toBeInTheDocument()
    expect(selectButton()).toHaveAccessibleName('Select')
    await answer('h1', '/only-on-h1', [dir('/only-on-h1/late')])
    await answer('h2', '/', [dir('/only-on-h2')])

    expect(await findRow('only-on-h2')).toBeInTheDocument()
    expect(screen.queryByRole('treeitem', { name: 'late' })).not.toBeInTheDocument()
    expect(
      within(screen.getByRole('dialog')).getByText("Browsing office-vm's filesystem"),
    ).toBeVisible()
  })

  it('a superseded answer never overwrites the one that replaced it', async () => {
    render(
      <StrictMode>
        <RemoteFilePicker hostId="h1" open onOpenChange={vi.fn()} onSelect={vi.fn()} />
      </StrictMode>,
    )
    expect(requested('/')).toBe(2)

    const [first, second] = held.get('h1:/')!
    await act(async () => second([dir('/fresh')]))
    await act(async () => first([dir('/stale')]))

    expect(await findRow('fresh')).toBeInTheDocument()
    expect(screen.queryByRole('treeitem', { name: 'stale' })).not.toBeInTheDocument()
  })

  it('opening a folder that failed asks again, and shows the new answer', async () => {
    serve('h1', HOME)
    renderPicker()
    fireEvent.click(await findRow('home'))
    await answer('h1', '/home', new ApiError('busy', 500))
    fireEvent.click(row('home'))
    await answer('h1', '/home', [dir('/home/mateo')])

    expect(await findRow('mateo')).toBeInTheDocument()
    expect(requested('/home')).toBe(2)
  })
})
