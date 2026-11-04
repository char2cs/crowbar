import { render, screen, fireEvent, waitFor, act } from '@testing-library/react'
import { OobeScreen } from '@/components/oobe/oobe-screen'
import { vi, describe, it, expect } from 'vitest'

// The OOBE is a multi-step gradient flow: presentation → prerequisites →
// add-project. The add-project step's "Choose a folder" CTA opens the import
// modal (subscribe-before-POST). We drive the steps and assert the headline/CTA
// and that the CTA opens the dialog.

vi.mock('@tanstack/react-router', () => ({ useNavigate: () => vi.fn() }))
vi.mock('@/core/entities/active-project', () => ({
  useProjectStore: { getState: () => ({ setActiveProject: vi.fn() }) },
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

describe('OobeScreen', () => {
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
