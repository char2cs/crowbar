import { render, screen, fireEvent, waitFor, act } from '@testing-library/react'
import { OobeScreen } from '@/components/oobe/oobe-screen'
import { vi, describe, it, expect, beforeEach } from 'vitest'
import {
  markProjectsSynced,
  projectDTO,
  resetEntities,
  seedProjects,
} from '@/__tests__/__fixtures__/sidebar-entities'

// The OOBE is a multi-step gradient flow: presentation → prerequisites →
// add-project. The add-project step's "Choose a folder" CTA opens the import
// modal (subscribe-before-POST). We drive the steps and assert the headline/CTA
// and that the CTA opens the dialog.

const landedOn = vi.hoisted(() => vi.fn())
vi.mock('@tanstack/react-router', () => ({
  Navigate: (props: unknown) => {
    landedOn(props)
    return null
  },
}))
vi.mock('@/lib/api', () => ({
  fetchPrerequisites: vi.fn().mockResolvedValue({
    git: { installed: true, version: 'git 2.0' },
    gh: { installed: true, authed: true },
    glab: { installed: true, authed: true },
  }),
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

// Advance the presentation → prerequisites → add-project flow up to the
// add-project step (where the "Choose a folder" CTA lives).
async function advanceToAddProject() {
  // presentation → prerequisites
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
    expect(screen.getAllByText('Authenticated')).toHaveLength(2)
    expect(screen.getByRole('button', { name: /continue/i })).not.toBeDisabled()
  })

  it('opens the import modal from the add-project step CTA', async () => {
    render(<OobeScreen />)
    await advanceToAddProject()
    await waitFor(() =>
      expect(screen.getByRole('button', { name: /choose a folder/i })).toBeInTheDocument(),
    )
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /choose a folder/i }))
    })
    expect(screen.getByRole('dialog')).toBeInTheDocument()
  })
})
