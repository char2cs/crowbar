import { describe, it, expect, beforeEach } from 'vitest'
import { useChaosStore } from '@/lib/store/chaos'

beforeEach(() => {
  useChaosStore.setState({
    latency: 0,
    errorRate: 0,
    scenario: 'normal',
    faults: {
      workspaces: 0,
      projects: 0,
      'file-tree': 0,
      'file-content': 0,
      'branch-diff': 0,
      'branch-threads': 0,
      'branch-description': 0,
      'branch-chats': 0,
      'git-commits': 0,
      'git-status': 0,
      'git-branches': 0,
    },
  })
})

describe('useChaosStore', () => {
  it('setScenario updates scenario', () => {
    useChaosStore.getState().setScenario('extreme')
    expect(useChaosStore.getState().scenario).toBe('extreme')
  })

  it('setFault updates a single fault key', () => {
    useChaosStore.getState().setFault('branch-diff', 75)
    expect(useChaosStore.getState().faults['branch-diff']).toBe(75)
    expect(useChaosStore.getState().faults['workspaces']).toBe(0)
  })

  it('resetFaults clears all fault values to 0', () => {
    useChaosStore.getState().setFault('branch-diff', 100)
    useChaosStore.getState().setFault('workspaces', 50)
    useChaosStore.getState().resetFaults()
    const { faults } = useChaosStore.getState()
    expect(Object.values(faults).every((v) => v === 0)).toBe(true)
  })

  it('reset clears latency and errorRate but preserves scenario', () => {
    useChaosStore.getState().setLatency(500)
    useChaosStore.getState().setScenario('empty')
    useChaosStore.getState().reset()
    expect(useChaosStore.getState().latency).toBe(0)
    expect(useChaosStore.getState().scenario).toBe('empty')
  })
})
