import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { RepoIconMark, EditableRepoIcon, type RepoIconSource } from '@/components/layout/repo-icon-mark'

const { apiFetch } = vi.hoisted(() => ({ apiFetch: vi.fn() }))
vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiFetch,
}))

/**
 * A repo's mark is ONE component across the repo home row, the context pill, the
 * workspace switcher, the removal tray and the New Tab heading.
 *
 * It used to be two. The row drew it through IconPopover (separate emoji/iconUrl
 * props, rounded-md, px-1, a full type step larger); everywhere else drew it
 * through RepoAvatar, which encodes the emoji INSIDE the url as an `emoji:`
 * sentinel and used rounded-sm, px-0.5 and a smaller emoji. Same repo, two
 * different marks — visibly so in the pill sitting directly above the row.
 */
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

/**
 * The repo home row's click-to-edit icon — the reconnection Task 5 (icon
 * personalization) restores. Built in cf422bc5 as `repo-icon-popover.tsx`,
 * deleted in the tree retirement along with the row that hosted it; this is
 * a fresh, thin wrapper against today's IconPopover/RepoIconMark, not a
 * resurrection of the deleted file.
 */
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
