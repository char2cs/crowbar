import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { ProjectIconMark, EditableProjectIcon } from '@/components/layout/project-icon-mark'
import { assetURL } from '@/lib/api'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { idle, success } from '@/lib/loadable'
import { useProjectDataStore } from '@/lib/store/projects'
import type { Project } from '@/lib/types'

const { apiFetch } = vi.hoisted(() => ({ apiFetch: vi.fn() }))
vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiFetch,
}))

/**
 * The mark a project shows, wherever it is shown. It exists because the context
 * pill used to draw its own <Library>, so a project with an icon rendered that
 * icon in the sidebar row and the default glyph in the pill above it.
 */
describe('ProjectIconMark', () => {
  const project = { id: 'p1', name: 'Crowbar' }

  it('draws the Library default when the project has no icon', () => {
    const { container } = render(<ProjectIconMark project={project} size="lg" />)

    expect(container.querySelector('svg')).not.toBeNull()
    expect(container.querySelector('img')).toBeNull()
  })

  it('prefers an emoji over an uploaded image', () => {
    // The daemon clears a stored image when an emoji is set and vice versa, so
    // the two can never both be live; this order mirrors that, it does not
    // invent one.
    const { container } = render(
      <ProjectIconMark
        project={{ ...project, avatarEmoji: '🚀', avatarUrl: '/v0/projects/p1/icon' }}
        size="lg"
      />,
    )

    expect(container.textContent).toBe('🚀')
    expect(container.querySelector('img')).toBeNull()
    expect(container.querySelector('svg')).toBeNull()
  })

  it('resolves the image URL for the browser', () => {
    const { container } = render(
      <ProjectIconMark project={{ ...project, avatarUrl: '/v0/projects/p1/icon' }} size="lg" />,
    )

    expect(container.querySelector('img')?.getAttribute('src')).toBe(
      assetURL('/v0/projects/p1/icon'),
    )
  })

  it('scales the mark by size, so one component serves row and pill', () => {
    const box = (size: 'md' | 'lg') => {
      const { container } = render(<ProjectIconMark project={project} size={size} />)
      return container.firstElementChild!.className
    }

    expect(box('lg')).toContain('size-5')
    expect(box('md')).toContain('size-4')
  })

  describe('cache-busting', () => {
    const url = '/v0/projects/p1/icon?v=3'

    it('leaves the daemon-versioned URL alone for read-only surfaces', () => {
      const { container } = render(
        <ProjectIconMark project={{ ...project, avatarUrl: url }} size="md" />,
      )

      expect(container.querySelector('img')?.getAttribute('src')).toBe(assetURL(url))
    })

    it('stacks its own version for a surface that can change the icon', () => {
      // The daemon's `?v=` only moves once the updated ProjectDTO has come back
      // over the WS stream; this one moves as soon as the upload resolves, which
      // is what stops a re-upload showing the previous bytes.
      const { container } = render(
        <ProjectIconMark project={{ ...project, avatarUrl: url }} size="lg" version={7} />,
      )

      expect(container.querySelector('img')?.getAttribute('src')).toBe(`${assetURL(url)}&v=7`)
    })
  })
})

/**
 * The space header's click-to-edit icon — mirrors EditableRepoIcon's own
 * test suite (repo-icon-mark.test.tsx). Built in cf422bc5 as
 * `project-icon-popover.tsx`, deleted in the tree retirement along with the
 * row that hosted it (project-home-row.tsx); this is a fresh, thin wrapper
 * against today's IconPopover/ProjectIconMark, not a resurrection.
 */
describe('EditableProjectIcon', () => {
  const project = { id: 'p1', name: 'harbour' }

  beforeEach(() => {
    vi.clearAllMocks()
    apiFetch.mockResolvedValue(undefined)
  })

  it('draws the same mark ProjectIconMark draws, read-only surfaces included', () => {
    const { container } = render(<EditableProjectIcon project={project} size="lg" />)
    expect(container.querySelector('svg')).not.toBeNull()
  })

  it('clicking the mark opens the picker, without folding the header it sits in', async () => {
    const user = userEvent.setup()
    const onHeaderClick = vi.fn()
    render(
      <div onClick={onHeaderClick}>
        <EditableProjectIcon project={project} size="lg" />
      </div>,
    )
    await user.click(screen.getByRole('button', { name: /edit harbour icon/i }))
    expect(await screen.findByText('Icon')).toBeInTheDocument()
    expect(onHeaderClick).not.toHaveBeenCalled()
  })

  it('has no GitHub owner-avatar action — a project has no origin remote', async () => {
    const user = userEvent.setup()
    render(<EditableProjectIcon project={project} size="lg" />)
    await user.click(screen.getByRole('button', { name: /edit harbour icon/i }))
    await screen.findByText('Icon')
    expect(screen.queryByRole('button', { name: /github/i })).not.toBeInTheDocument()
  })

  it('setting an emoji PUTs it to this project’s own REST base and persists it', async () => {
    const user = userEvent.setup()
    render(<EditableProjectIcon project={project} size="lg" />)
    await user.click(screen.getByRole('button', { name: /edit harbour icon/i }))
    await user.click(await screen.findByRole('button', { name: /emoji/i }))
    await user.type(screen.getByPlaceholderText('Type an emoji…'), '🛰️')
    await user.keyboard('{Enter}')
    expect(apiFetch).toHaveBeenCalledWith('/v0/projects/p1/icon/emoji', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ emoji: '🛰️' }),
    })
  })

  it('resolves a stored image URL for the browser, same as the read-only mark', () => {
    // The visible TRIGGER mark (EditableProjectIcon's `trigger` render-prop
    // draws ProjectIconMark directly), not the popover's own internal Avatar
    // preview — that one is base-ui's load-gated Image, which jsdom never
    // fires the load event for, so it never mounts an <img> here at all.
    // The trigger always carries IconPopover's own cache-bust version (an
    // editing surface, per ProjectIconMark's own doc), starting at 0.
    const { container } = render(
      <EditableProjectIcon project={{ ...project, avatarUrl: '/v0/projects/p1/icon' }} size="lg" />,
    )
    expect(container.querySelector('img')?.getAttribute('src')).toBe(
      `${assetURL('/v0/projects/p1/icon')}?v=0`,
    )
  })
})

// The icon URL is built during render, from the project's own host: an unplaceable project
// draws the default mark, and a remote project's image is never fetched from the local daemon.
describe('a project on a remote host', () => {
  const project = { id: 'p1', name: 'harbour', avatarUrl: '/v0/projects/p1/icon?v=1' }
  const row = (hostId: string): Project => ({
    id: 'p1',
    name: 'harbour',
    path: '/work/p1',
    lastActivity: new Date(0),
    hostId,
  })
  const pairHost = (id: string) =>
    useHostConfigStore.getState().addHost({
      id,
      name: id,
      endpoint: `${id}.example:3737`,
      pairedFingerprint: 'SHA256:aaaa',
      clientId: `client-${id}`,
    })

  beforeEach(() => {
    vi.clearAllMocks()
    apiFetch.mockResolvedValue(undefined)
    ;(window as { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__ = {}
    useHostConfigStore.setState({ hosts: [], remoteEverPaired: false })
    useProjectDataStore.setState({ data: idle() })
    pairHost('host-2')
  })

  afterEach(() => {
    delete (window as { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__
    useHostConfigStore.setState({ hosts: [], remoteEverPaired: false })
    useProjectDataStore.setState({ data: idle() })
  })

  it('loads the image from its own host', () => {
    useProjectDataStore.setState({ data: success([row('host-2')]) })
    const { container } = render(<ProjectIconMark project={project} size="lg" />)
    expect(container.querySelector('img')?.getAttribute('src')).toBe(
      'crowbar://host-2/v0/projects/p1/icon?v=1',
    )
  })

  it.each([
    ['the project list has not loaded', () => undefined],
    [
      'its host is not configured',
      () => useProjectDataStore.setState({ data: success([row('gone')]) }),
    ],
  ])('draws the default mark, with no image, while %s', (_why, arrange) => {
    arrange()
    const { container } = render(<ProjectIconMark project={project} size="lg" />)
    expect(container.querySelector('svg')).not.toBeNull()
    expect(container.querySelector('img')).toBeNull()
  })

  it('swaps the default mark for the image once the project’s row arrives', () => {
    const { container } = render(<ProjectIconMark project={project} size="lg" />)
    expect(container.querySelector('img')).toBeNull()
    act(() => useProjectDataStore.setState({ data: success([row('host-2')]) }))
    expect(container.querySelector('img')?.getAttribute('src')).toBe(
      'crowbar://host-2/v0/projects/p1/icon?v=1',
    )
  })

  it('draws no image for a source that names no project to place it on', () => {
    const { container } = render(
      <ProjectIconMark project={{ name: 'x', avatarUrl: '/v0/projects/p1/icon' }} size="lg" />,
    )
    expect(container.querySelector('img')).toBeNull()
  })

  it('edits against its own host: an emoji is PUT to the qualified base', async () => {
    useProjectDataStore.setState({ data: success([row('host-2')]) })
    const user = userEvent.setup()
    render(<EditableProjectIcon project={project} size="lg" />)
    await user.click(screen.getByRole('button', { name: /edit harbour icon/i }))
    await user.click(await screen.findByRole('button', { name: /emoji/i }))
    await user.type(screen.getByPlaceholderText('Type an emoji…'), '🛰️')
    await user.keyboard('{Enter}')
    expect(apiFetch).toHaveBeenCalledWith('crowbar://host-2/v0/projects/p1/icon/emoji', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ emoji: '🛰️' }),
    })
  })

  it('offers no editing until the host is known, and never throws getting there', () => {
    const { container } = render(<EditableProjectIcon project={project} size="lg" />)
    expect(screen.queryByRole('button', { name: /edit harbour icon/i })).toBeNull()
    expect(container.querySelector('img')).toBeNull()
    act(() => useProjectDataStore.setState({ data: success([row('host-2')]) }))
    expect(screen.getByRole('button', { name: /edit harbour icon/i })).toBeInTheDocument()
  })
})
