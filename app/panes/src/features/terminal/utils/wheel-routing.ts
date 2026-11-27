export function shouldScrollScrollback(
  event: Pick<WheelEvent, 'shiftKey'>,
  mouseTrackingMode: string | undefined,
  bufferType: 'normal' | 'alternate',
): boolean {
  if (!mouseTrackingMode || mouseTrackingMode === 'none') return false
  return event.shiftKey && bufferType !== 'alternate'
}
