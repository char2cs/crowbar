import { LOCAL_HOST } from '@/features/hosts/types'

// `~` occurs in no host id (`[a-z0-9-]+`) nor in a daemon's ids (uuids).
const SEP = '~'

/** A record's key in the store: the raw id on the local daemon, `host~id` on a remote one,
 *  so every persisted local reference is unchanged and two hosts' twins never collide. */
export function entityKey(hostId: string, id: string): string {
  return hostId === LOCAL_HOST.id ? id : `${hostId}${SEP}${id}`
}

/** A scope as the client names it, by {@link entityKey}'s rule; the wire carries it bare. */
export function hostScope(hostId: string, scope: string): string {
  return entityKey(hostId, scope)
}

/** The host and raw id (or bare scope) of a key built by {@link entityKey}. */
export function splitHostKey(key: string): { hostId: string; id: string } {
  const at = key.indexOf(SEP)
  if (at < 0) return { hostId: LOCAL_HOST.id, id: key }
  return { hostId: key.slice(0, at), id: key.slice(at + SEP.length) }
}
