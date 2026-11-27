import { installInputTapeGlobal } from '@/features/terminal/utils/input-tape'

interface TapeDump {
  dropped: number
  entries: {
    kind: string
    label: string
    data?: string
    meta?: Record<string, string | number | boolean>
  }[]
}

type TapeGlobal = (options?: { copy?: boolean; clear?: boolean }) => TapeDump

function tapeGlobal(): TapeGlobal {
  installInputTapeGlobal()
  return (window as unknown as { __crowbarInputTape: TapeGlobal }).__crowbarInputTape
}

export function dumpInputTape(): TapeDump {
  return tapeGlobal()()
}

export function clearInputTape(): void {
  tapeGlobal()({ clear: true })
}
