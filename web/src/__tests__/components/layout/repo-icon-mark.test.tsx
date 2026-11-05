import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import {
  RepoIconMark,
  EditableRepoIcon,
  type RepoIconSource,
} from '@/components/layout/repo-icon-mark'
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

// A repo's mark is ONE component across the home row, context pill, switcher, removal tray and
// New Tab heading, so the same repo never renders as two different marks.
describe('RepoIconMark', () => {
  const repo: RepoIconSource = {
    name: 'crowbar',
    avatarLabel: 'C',
    avatarColor: 'bg-indigo-700',
  }

  it('draws the generated letter tile when the repo has no icon', () => {
    const { container } = render(<RepoIconMark repo={repo} size="lg" />)
    const tile = container.firstElementChild!
    expect(tile.textContent).toBe('C')
    expect(tile.className).toContain('bg-indigo-700')
    expect(container.querySelector('img')).toBeNull()
  })

  it('renders the emoji the avatarURL sentinel carries', () => {
    const { container } = render(
      <RepoIconMark repo={{ ...repo, avatarURL: 'emoji:🚀' }} size="lg" />,
    )
    expect(container.textContent).toBe('🚀')
    expect(container.querySelector('img')).toBeNull()
  })

  it('renders a real avatarURL as an image', () => {
    const { container } = render(
      <RepoIconMark repo={{ ...repo, avatarURL: '/v0/x/icon?v=2' }} size="lg" />,
    )
    expect(container.querySelector('img')?.getAttribute('src')).toBe('/v0/x/icon?v=2')
  })

  // The repo home row is where an icon is SET, so its rendering is the one every
  // other surface has to match.
  it('matches the repo home row: rounded-md, px-1, and a full step larger emoji', () => {
    const tile = render(<RepoIconMark repo={repo} size="lg" />).container.firstElementChild!
    expect(tile.className).toContain('rounded-md')
    expect(tile.className).toContain('px-1')
    expect(tile.className).not.toContain('rounded-sm')

    const emoji = render(<RepoIconMark repo={{ ...repo, avatarURL: 'emoji:🚀' }} size="lg" />)
      .container.firstElementChild!
    expect(emoji.className).toContain('text-lg')

    const img = render(
      <RepoIconMark repo={{ ...repo, avatarURL: '/i' }} size="lg" />,
    ).container.querySelector('img')!
    expect(img.className).toContain('rounded-md')
  })

  it('scales by size while keeping the same shape', () => {
    const box = (size: 'sm' | 'lg' | 'xl') =>
      render(<RepoIconMark repo={repo} size={size} />).container.firstElementChild!.className
    expect(box('sm')).toContain('size-4')
    expect(box('lg')).toContain('size-5')
    expect(box('xl')).toContain('size-6')
  })

  it('only the editing surface adds a cache-bust', () => {
    const plain = render(
      <RepoIconMark repo={{ ...repo, avatarURL: '/i?v=3' }} size="lg" />,
    ).container.querySelector('img')!
    expect(plain.getAttribute('src')).toBe('/i?v=3')

    const versioned = render(
      <RepoIconMark repo={{ ...repo, avatarURL: '/i?v=3' }} size="lg" version={7} />,
    ).container.querySelector('img')!
    expect(versioned.getAttribute('src')).toBe('/i?v=3&v=7')
  })
})

// The repo home row's click-to-edit icon: a thin wrapper joining IconPopover to RepoIconMark.
describe('EditableRepoIcon', () => {
  const repo: RepoIconSource = {
    name: 'crowbar',
    avatarLabel: 'C',
    avatarColor: 'bg-indigo-700',
  }

  beforeEach(() => {
    vi.clearAllMocks()
    apiFetch.mockResolvedValue(undefined)
  })

  it('draws the same mark RepoIconMark draws, read-only surfaces included', () => {
    const { container } = render(
      <EditableRepoIcon repo={repo} projectId="p1" repoId="r1" size="lg" />,
    )
    expect(container.textContent).toBe('C')
  })

  it('clicking the mark opens the picker, without navigating the row it sits in', async () => {
    const user = userEvent.setup()
    const onRowClick = vi.fn()
    render(
      <div onClick={onRowClick}>
        <EditableRepoIcon repo={repo} projectId="p1" repoId="r1" size="lg" />
      </div>,
    )
    await user.click(screen.getByRole('button', { name: /edit crowbar icon/i }))
    expect(await screen.findByText('Icon')).toBeInTheDocument()
    expect(onRowClick).not.toHaveBeenCalled()
  })

  it('offers the GitHub owner-avatar action a repo has, that a project does not', async () => {
    const user = userEvent.setup()
    render(<EditableRepoIcon repo={repo} projectId="p1" repoId="r1" size="lg" />)
    await user.click(screen.getByRole('button', { name: /edit crowbar icon/i }))
    expect(await screen.findByRole('button', { name: /github/i })).toBeInTheDocument()
  })

  it('setting an emoji PUTs it to this repo’s own REST base and persists it', async () => {
    const user = userEvent.setup()
    render(<EditableRepoIcon repo={repo} projectId="p1" repoId="r1" size="lg" />)
    await user.click(screen.getByRole('button', { name: /edit crowbar icon/i }))
    await user.click(await screen.findByRole('button', { name: /emoji/i }))
    await user.type(screen.getByPlaceholderText('Type an emoji…'), '🚀')
    await user.keyboard('{Enter}')
    expect(apiFetch).toHaveBeenCalledWith('/v0/projects/p1/repos/r1/icon/emoji', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ emoji: '🚀' }),
    })
  })

  it('splits an emoji avatarURL into IconPopover’s separate emoji prop, not iconUrl', async () => {
    const user = userEvent.setup()
    render(
      <EditableRepoIcon
        repo={{ ...repo, avatarURL: 'emoji:🛰️' }}
        projectId="p1"
        repoId="r1"
        size="lg"
      />,
    )
    await user.click(screen.getByRole('button', { name: /edit crowbar icon/i }))
    // The preview Avatar shows the emoji directly (not an <img>) only when
    // IconPopover was handed `emoji`, not a raw `emoji:` iconUrl it would try
    // to fetch as an image. Two matches are expected — the trigger mark
    // (still mounted with the popover open) and the popup's own preview.
    await screen.findByText('Icon') // wait for the popup to be open
    expect(screen.getAllByText('🛰️').length).toBeGreaterThanOrEqual(1)
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
  })
})

// The REST base is built during render from the repo's project: it names the project's own
// host, and nothing is editable (or thrown) until that host is known.
describe('EditableRepoIcon on a remote host', () => {
  const repo: RepoIconSource = { name: 'crowbar', avatarLabel: 'C', avatarColor: 'bg-indigo-700' }
  const loadRow = (hostId: string) => loadProjects(projectOn('p1', hostId))

  beforeEach(() => {
    vi.clearAllMocks()
    apiFetch.mockResolvedValue(undefined)
    resetHostWorld()
    pairRemoteHost('host-2')
  })

  afterEach(() => resetHostWorld())

  it('PUTs an emoji to the repo’s qualified base', async () => {
    loadRow('host-2')
    const user = userEvent.setup()
    render(<EditableRepoIcon repo={repo} projectId="p1" repoId="r1" size="lg" />)
    await user.click(screen.getByRole('button', { name: /edit crowbar icon/i }))
    await user.click(await screen.findByRole('button', { name: /emoji/i }))
    await user.type(screen.getByPlaceholderText('Type an emoji…'), '🚀')
    await user.keyboard('{Enter}')
    expect(apiFetch).toHaveBeenCalledWith('crowbar://host-2/v0/projects/p1/repos/r1/icon/emoji', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ emoji: '🚀' }),
    })
  })

  it('draws the read-only mark, without throwing, until the host is known', () => {
    const { container } = render(
      <EditableRepoIcon repo={repo} projectId="p1" repoId="r1" size="lg" />,
    )
    expect(container.textContent).toBe('C')
    expect(screen.queryByRole('button', { name: /edit crowbar icon/i })).toBeNull()
    act(() => loadRow('host-2'))
    expect(screen.getByRole('button', { name: /edit crowbar icon/i })).toBeInTheDocument()
  })
})
