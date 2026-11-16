import { describe, it, expect, beforeEach } from 'vitest'
import { hostPorts } from '@/features/hosts/lib/ws-host-ports'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { LOCAL_HOST } from '@/features/hosts/types'

beforeEach(() => {
  useHostConfigStore.setState(useHostConfigStore.getInitialState())
})

describe('hostPorts', () => {
  it('knows the local host and dials it always', () => {
    expect(hostPorts.isLocalHost(LOCAL_HOST.id)).toBe(true)
    expect(hostPorts.isLocalHost('box')).toBe(false)
    expect(hostPorts.mayDial(LOCAL_HOST.id)).toBe(true)
  })

  it('does not dial a remote host nobody paired', () => {
    expect(hostPorts.mayDial('box')).toBe(false)
  })
})
