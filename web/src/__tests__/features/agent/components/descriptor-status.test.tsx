import { render, screen, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { DescriptorStatus } from '@/features/agent/components/descriptor-status'

const { getDescriptorReportsFn } = vi.hoisted(() => ({ getDescriptorReportsFn: vi.fn() }))

vi.mock('@/features/agent/api/agent-api', () => ({
  getDescriptorReports: (...a: unknown[]) => getDescriptorReportsFn(...a),
}))

beforeEach(() => {
  getDescriptorReportsFn.mockReset()
})

describe('DescriptorStatus', () => {
  it('marks a clean descriptor OK and one with an error Blocked, with where and how to fix', async () => {
    getDescriptorReportsFn.mockResolvedValue([
      { id: 'claude', findings: [] },
      {
        id: 'codex',
        source: '/home/me/.crowbar/descriptors/codex.yaml',
        findings: [
          {
            rule: 'session.locate_glob',
            severity: 'error',
            path: 'session.locate.glob[0]',
            line: 12,
            message: 'glob must name the session id',
            hint: 'put {id} in the file name',
          },
        ],
      },
    ])
    render(<DescriptorStatus />)

    const claude = await screen.findByTestId('descriptor-claude')
    expect(within(claude).getByText('OK')).toBeInTheDocument()
    expect(within(claude).getByText('shipped')).toBeInTheDocument()

    const codex = screen.getByTestId('descriptor-codex')
    expect(within(codex).getByText('Blocked')).toBeInTheDocument()
    expect(within(codex).getByText('/home/me/.crowbar/descriptors/codex.yaml')).toBeInTheDocument()
    expect(codex).toHaveTextContent('session.locate.glob[0] line 12: glob must name the session id')
    expect(codex).toHaveTextContent('Fix: put {id} in the file name')
  })

  it('counts warnings without blocking', async () => {
    getDescriptorReportsFn.mockResolvedValue([
      {
        id: 'mine',
        findings: [
          {
            rule: 'session.resume_unverified',
            severity: 'warning',
            path: 'session.resume',
            line: 3,
            message: 'unverified',
          },
        ],
      },
    ])
    render(<DescriptorStatus />)

    expect(await screen.findByText('1 warning')).toBeInTheDocument()
    expect(screen.queryByText('Blocked')).not.toBeInTheDocument()
  })

  it('shows a refused override as falling back to the shipped descriptor, with why', async () => {
    getDescriptorReportsFn.mockResolvedValue([
      {
        id: 'claude',
        source: '/home/me/.crowbar/descriptors/claude.yaml',
        fellBack: true,
        findings: [
          {
            rule: 'hooks.in_config_injection',
            severity: 'error',
            path: 'config_injection[0]',
            line: 40,
            message: 'hooks belong in hooks_injection',
          },
        ],
      },
    ])
    render(<DescriptorStatus />)

    const claude = await screen.findByTestId('descriptor-claude')
    expect(within(claude).getByText('Override refused — using shipped')).toBeInTheDocument()
    expect(within(claude).queryByText('Blocked')).not.toBeInTheDocument()
    expect(claude).toHaveTextContent('config_injection[0] line 40: hooks belong in hooks_injection')
  })

  it('says so when the daemon cannot be reached', async () => {
    getDescriptorReportsFn.mockRejectedValue(new Error('offline'))
    render(<DescriptorStatus />)

    expect(await screen.findByText(/could not check the descriptors/i)).toBeInTheDocument()
  })
})
