import { render, screen, fireEvent, waitFor, act } from '@testing-library/react'
import { OobeScreen } from '@/components/oobe/oobe-screen'
import { vi, beforeEach, afterEach, describe, it, expect } from 'vitest'

// The OOBE is a multi-step gradient flow: presentation → prerequisites →
// add-project. The add-project step's "Choose a folder" CTA opens the import
// modal (subscribe-before-POST). We drive the steps and assert the headline/CTA
// and that the CTA opens the dialog.

// ShaderGradientCanvas uses IntersectionObserver, absent under jsdom — polyfill it.
class IO {
  observe() {}
  unobserve() {}
  disconnect() {}
  takeRecords() {
    return []
  }
}

vi.mock('@tanstack/react-router', () => ({ useNavigate: () => vi.fn() }))
vi.mock('@/lib/store/projects', () => ({
  importProjectAndSync: vi.fn(),
  useProjectStore: { getState: () => ({ setActiveProject: vi.fn() }) },
}))
vi.mock('@/lib/api', () => ({
  fetchPrerequisites: vi.fn().mockResolvedValue({
    git: { installed: true, version: 'git 2.0' },
    gh: { installed: true, authed: true },
    glab: { installed: true, authed: true },
  }),
}))

beforeEach(() => {
  vi.stubGlobal('IntersectionObserver', IO)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

// Advance the presentation → prerequisites → add-project flow up to the
// add-project step (where the "Choose a folder" CTA lives).
async function advanceToAddProject() {
  // presentation → prerequisites
  fireEvent.click(screen.getByRole('button', { name: /get started/i }))
  // prerequisites resolve + reveal (the staggered reveal timers run on real
  // time; waitFor polls until Continue enables), then continue → add-project.
  await waitFor(
    () => expect(screen.getByRole('button', { name: /continue/i })).not.toBeDisabled(),
    { timeout: 3000 },
  )
  fireEvent.click(screen.getByRole('button', { name: /continue/i }))
}

describe('OobeScreen', () => {
  it('renders the presentation headline and CTA', () => {
    render(<OobeScreen />)
    expect(screen.getByText(/the ide where agents do the heavy lifting/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /get started/i })).toBeInTheDocument()
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
