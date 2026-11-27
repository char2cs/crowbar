import { useNewestStop } from '@/features/agent/trays/turn-failure-tray'
import { markerFailure } from '@/features/agent/transcript/lib/marker-failure'

export function useModelHot(chatId: string): boolean {
  const stop = useNewestStop(chatId)
  return stop !== undefined && markerFailure(stop)?.way === 'switch-model'
}
