import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { ProjectIconMark, EditableProjectIcon } from '@/components/layout/project-icon-mark'
import { assetURL } from '@/lib/api'
import {
  loadProjects,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'

const { apiFetch } = vi.hoisted(() => ({ apiFetch: vi.fn() }))
vi.mock('@/lib/api/fetch', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/fetch')>()),
  apiFetch,
}))

describe('ProjectIconMark', () => {
  const project = { id: 'p1', name: 'Crowbar' }

  it('draws the Library default when the project has no icon', () => {
    const { container } = render(<ProjectIconMark project={project} size="lg" />)

    expect(container.querySelector('svg')).not.toBeNull()
    expect(container.querySelector('img')).toBeNull()
  })

  it('prefers an emoji over an uploaded image', () => {
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
      const { container } = render(
        <ProjectIconMark project={{ ...project, avatarUrl: url }} size="lg" version={7} />,
      )

      expect(container.querySelector('img')?.getAttribute('src')).toBe(`${assetURL(url)}&v=7`)
    })
  })
})

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
    const user = userEvent.setup({ delay: null })
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
    const user = userEvent.setup({ delay: null })
    render(<EditableProjectIcon project={project} size="lg" />)
    await user.click(screen.getByRole('button', { name: /edit harbour icon/i }))
    await screen.findByText('Icon')
    expect(screen.queryByRole('button', { name: /github/i })).not.toBeInTheDocument()
  })

  it('setting an emoji PUTs it to this project’s own REST base and persists it', async () => {
    const user = userEvent.setup({ delay: null })
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
    const { container } = render(
      <EditableProjectIcon project={{ ...project, avatarUrl: '/v0/projects/p1/icon' }} size="lg" />,
    )
    expect(container.querySelector('img')?.getAttribute('src')).toBe(
      `${assetURL('/v0/projects/p1/icon')}?v=0`,
    )
  })
})

describe('a project on a remote host', () => {
  const project = { id: 'p1', name: 'harbour', avatarUrl: '/v0/projects/p1/icon?v=1' }
  const row = (hostId: string) => projectOn('p1', hostId)

  beforeEach(() => {
    vi.clearAllMocks()
    apiFetch.mockResolvedValue(undefined)
    ;(window as { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__ = {}
    resetHostWorld()
    pairRemoteHost('host-2')
  })

  afterEach(() => {
    delete (window as { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__
    resetHostWorld()
  })

  it('loads the image from its own host', () => {
    loadProjects(row('host-2'))
    const { container } = render(<ProjectIconMark project={project} size="lg" />)
    expect(container.querySelector('img')?.getAttribute('src')).toBe(
      'crowbar://host-2/v0/projects/p1/icon?v=1',
    )
  })

  it.each([
    ['the project list has not loaded', () => undefined],
    ['its host is not configured', () => loadProjects(row('gone'))],
  ])('draws the default mark, with no image, while %s', (_why, arrange) => {
    arrange()
    const { container } = render(<ProjectIconMark project={project} size="lg" />)
    expect(container.querySelector('svg')).not.toBeNull()
    expect(container.querySelector('img')).toBeNull()
  })

  it('swaps the default mark for the image once the project’s row arrives', () => {
    const { container } = render(<ProjectIconMark project={project} size="lg" />)
    expect(container.querySelector('img')).toBeNull()
    act(() => loadProjects(row('host-2')))
    expect(container.querySelector('img')?.getAttribute('src')).toBe(
      'crowbar://host-2/v0/projects/p1/icon?v=1',
    )
  })

  it('edits against its own host: an emoji is PUT to the qualified base', async () => {
    loadProjects(row('host-2'))
    const user = userEvent.setup({ delay: null })
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
    act(() => loadProjects(row('host-2')))
    expect(screen.getByRole('button', { name: /edit harbour icon/i })).toBeInTheDocument()
  })
})
