import type { ChannelContext } from './channel-context'

function removeListener(ctx: ChannelContext, endpoint: string, listener: VoidFunction): void {
  const listeners = ctx.dropListeners.get(endpoint)
  listeners?.delete(listener)
  if (listeners?.size === 0) ctx.dropListeners.delete(endpoint)
}

/** Hears each drop of `endpoint`'s open socket; returns the unsubscribe. */
export function onDrop(
  ctx: ChannelContext,
  endpoint: string,
  listener: VoidFunction,
): VoidFunction {
  const listeners = ctx.dropListeners.get(endpoint) ?? new Set()
  ctx.dropListeners.set(endpoint, listeners.add(listener))
  return () => removeListener(ctx, endpoint, listener)
}

/** Tells every drop listener of `endpoint`. */
export function notifyDrop(ctx: ChannelContext, endpoint: string): void {
  for (const listener of [...(ctx.dropListeners.get(endpoint) ?? [])]) listener()
}
