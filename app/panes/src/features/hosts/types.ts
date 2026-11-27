export interface Host {
  id: string
  name: string
  endpoint: string
  pairedFingerprint: string
  clientId: string
}

export type HostStatus = 'unknown' | 'probing' | 'up' | 'down' | 'needs-repair'

export type HostDotState = HostStatus | 'no-live-stream' | 'not-available'

export const LOCAL_HOST: Host = {
  id: 'local',
  name: 'Local',
  endpoint: '',
  pairedFingerprint: '',
  clientId: '',
}
