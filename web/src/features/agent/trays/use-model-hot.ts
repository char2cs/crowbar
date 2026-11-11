import { useNewestStop } from '@/features/agent/trays/turn-failure-tray'
import { markerFailure } from '@/features/agent/transcript/lib/marker-failure'

/** The chat's newest stop is one another model gets past (a limit): the footer's model
 *  chip is lit, as the notice's "Switch model" opens it. */
export function useModelHot(chatId: string): boolean {
  const stop = useNewestStop(chatId)
  return stop !== undefined && markerFailure(stop)?.way === 'switch-model'
}
