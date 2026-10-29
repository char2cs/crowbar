import { useEffect, useState } from 'react'

/**
 * The launch model a live runner last reported, kept after `live` drops so the
 * chip does not blank between runners; another chat's or provider's never carries over.
 */
export function useConfirmedModel(key: string, live: boolean, launchModel: string) {
  const [confirmed, setConfirmed] = useState({ key, model: '' })
  // react-doctor-disable-next-line no-derived-state-effect -- it must outlive `launchModel` going absent
  useEffect(() => {
    // react-doctor-disable-next-line no-derived-state -- see above
    if (live && launchModel) setConfirmed({ key, model: launchModel })
  }, [key, live, launchModel])
  if (live) return launchModel
  return confirmed.key === key ? confirmed.model : ''
}
