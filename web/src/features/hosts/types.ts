export interface Host {
  id: string
  name: string
  endpoint: string
  /** Display only — what the user confirmed at pairing time. Never sent to
   *  any Tauri command; the keychain-bound fingerprint is what actually
   *  gates dials (see this plan's Amendment). */
  pairedFingerprint: string
  /** The daemon's client id, returned by `pairRemoteHost`. Needed to revoke
   *  server-side (`DELETE /v0/clients/:id`) on removal — see Task 5. */
  clientId: string
}

/** 'needs-repair': a 401 on an otherwise-'up' host mid-session (spec §3) —
 *  distinct from 'down' so the UI doesn't silently retry a dead credential. */
export type HostStatus = 'unknown' | 'probing' | 'up' | 'down' | 'needs-repair'

/** The implicit local daemon — never persisted, never removable. */
export const LOCAL_HOST: Host = {
  id: 'local',
  name: 'Local',
  endpoint: '',
  pairedFingerprint: '',
  clientId: '',
}
