import { LOCAL_HOST } from '@/features/hosts/types'

const SEP = '~'

export function entityKey(hostId: string, id: string): string {
  return hostId === LOCAL_HOST.id ? id : `${hostId}${SEP}${id}`
}

export function hostScope(hostId: string, scope: string): string {
  return entityKey(hostId, scope)
}

export function splitHostKey(key: string): { hostId: string; id: string } {
  const at = key.indexOf(SEP)
  if (at < 0) return { hostId: LOCAL_HOST.id, id: key }
  return { hostId: key.slice(0, at), id: key.slice(at + SEP.length) }
}
