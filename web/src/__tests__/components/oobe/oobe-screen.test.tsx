import { render, screen, fireEvent, waitFor, act } from '@testing-library/react'
import { OobeScreen } from '@/components/oobe/oobe-screen'
import { fetchPrerequisite } from '@/lib/api'
import { vi, describe, it, expect, beforeEach } from 'vitest'
import {
  markProjectsSynced,
  projectDTO,
  resetEntities,
  seedProjects,
} from '@/__tests__/__fixtures__/sidebar-entities'

const landedOn = vi.hoisted(() => vi.fn())
vi.mock('@tanstack/react-router', () => ({
  Navigate: (props: unknown) => {
    landedOn(props)
    return null
  },
}))
// git alone, as on a fresh machine: gh and glab are optional and must not hold the first run.
const FRESH = {
  git: { installed: true, version: 'git 2.0' },
  gh: { installed: false, authed: false },
  glab: { installed: false, authed: false },
}
vi.mock('@/lib/api', () => ({
  fetchPrerequisite: vi.fn(async (tool: keyof typeof FRESH) => FRESH[tool]),
}))

// The backdrop is plain CSS: no WebGL canvas, nothing for assistive tech.
describe('OOBE backdrop', () => {
  it('renders a decorative CSS gradient, not a WebGL canvas', () => {
    const { container } = render(<OobeScreen />)
    const backdrop = container.querySelector('.oobe-gradient')
    expect(backdrop).not.toBeNull()
    expect(backdrop).toHaveAttribute('aria-hidden')
    expect(container.querySelector('canvas')).toBeNull()
  })
})

async function advanceToAddProject() {
  fireEvent.click(screen.getByRole('button', { name: /get started/i }))
  await waitFor(() => expect(screen.getByRole('button', { name: /continue/i })).not.toBeDisabled())
  fireEvent.click(screen.getByRole('button', { name: /continue/i }))
}

beforeEach(() => {
  resetEntities()
  markProjectsSynced()
  landedOn.mockReset()
})

describe('OobeScreen', () => {
  // The project list decides, as the app's landing does: a space added from the dialog,
  // the API or another client takes the user to it.
  it('leaves for the first space as soon as the project list holds one', async () => {
    render(<OobeScreen />)
    await advanceToAddProject()
    expect(landedOn).not.toHaveBeenCalled()
    act(() => seedProjects([projectDTO({ id: 'p1', name: 'Space' })]))
    expect(landedOn).toHaveBeenLastCalledWith(
      expect.objectContaining({ to: '/ide/$projectId/home', params: { projectId: 'p1' } }),
    )
  })

  it('renders the presentation headline and CTA', () => {
    render(<OobeScreen />)
    expect(screen.getByText(/the ide where agents do the heavy lifting/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /get started/i })).toBeInTheDocument()
  })

  it('shows each check’s result as soon as the checks answer', async () => {
    render(<OobeScreen />)
    fireEvent.click(screen.getByRole('button', { name: /get started/i }))
    await act(async () => {})

    expect(screen.getByText('git 2.0')).toBeInTheDocument()
    expect(screen.getAllByText(/not installed — optional/i)).toHaveLength(2)
    expect(screen.getByRole('button', { name: /continue/i })).not.toBeDisabled()
  })

  // A slow `gh auth status` must not hold the rows that already answered.
  it('shows git as soon as it answers while gh is still being checked', async () => {
    vi.mocked(fetchPrerequisite).mockImplementation(async (tool) =>
      tool === 'gh' ? new Promise(() => {}) : FRESH[tool],
    )
    render(<OobeScreen />)
    fireEvent.click(screen.getByRole('button', { name: /get started/i }))
    await act(async () => {})

    expect(screen.getByText('git 2.0')).toBeInTheDocument()
    expect(screen.getAllByText('Checking…')).toHaveLength(1)
    expect(screen.getByRole('button', { name: /continue/i })).not.toBeDisabled()
    vi.mocked(fetchPrerequisite).mockImplementation(async (tool) => FRESH[tool])
  })

  it('holds the check while git is missing', async () => {
    vi.mocked(fetchPrerequisite).mockImplementation(async (tool) =>
      tool === 'git' ? { installed: false } : FRESH[tool],
    )
    render(<OobeScreen />)
    fireEvent.click(screen.getByRole('button', { name: /get started/i }))
    await act(async () => {})
    expect(screen.getByText(/git is required/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /continue/i })).toBeDisabled()
    vi.mocked(fetchPrerequisite).mockImplementation(async (tool) => FRESH[tool])
  })

  it('opens the import modal from the add-project step CTA', async () => {
    render(<OobeScreen />)
    await advanceToAddProject()
    fireEvent.click(await screen.findByRole('button', { name: /choose a folder/i }))
    expect(await screen.findByRole('dialog')).toBeInTheDocument()
  })
})
