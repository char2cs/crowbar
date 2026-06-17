import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { OobeScreen } from '@/components/oobe/oobe-screen'
import { vi } from 'vitest'

vi.mock('@tanstack/react-router', () => ({ useNavigate: () => vi.fn() }))
vi.mock('@/lib/store/projects', () => ({ importProjectAndSync: vi.fn() }))

describe('OobeScreen', () => {
  it('renders headline and CTA', () => {
    render(<OobeScreen />)
    expect(screen.getByText('Open a project folder')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /choose folder/i })).toBeInTheDocument()
  })

  it('opens import modal when CTA clicked', async () => {
    render(<OobeScreen />)
    await userEvent.click(screen.getByRole('button', { name: /choose folder/i }))
    expect(screen.getByRole('dialog')).toBeInTheDocument()
  })
})
