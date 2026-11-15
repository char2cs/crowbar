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

/** The tape as the console sees it: through the window global the terminal mount installs. */
export function dumpInputTape(): TapeDump {
  return tapeGlobal()()
}

export function clearInputTape(): void {
  tapeGlobal()({ clear: true })
}
