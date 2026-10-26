import { useEffect, useEffectEvent, useState } from 'react'
import { CrowbarWordmark } from '@/components/ui/crowbar-wordmark'
import { OOBE_TAGLINE } from '@/components/oobe/oobe-frame'
import {
  CODE_SHAPE_NOTICE,
  usePairingFlow,
  type PairingPending,
} from '@/features/hosts/hooks/use-pairing-flow'
import { onNativeChromeIntent, setNativeChrome, type NativeChromeIntent } from '@/lib/native-chrome'
import { useThemeVersion } from '@/features/editor/theme/use-theme-version'
import { useSettingsStore } from '@/features/settings/store'
import { readNativePalette } from '@/lib/native-palette'

const WELCOME_LABEL = 'Let’s build together'

interface AddHostScreenProps {
  onPaired: (hostId: string) => void
  onCancel?: () => void
}

/** The phone app's way in: the brand on its green, with a native glass button that opens a
 *  native sheet for pairing with a remote daemon, the only kind the phone has. The brand rises
 *  to the top while the sheet is up. Opened from the app, it starts on the sheet. */
export function AddHostScreen({ onPaired, onCancel }: AddHostScreenProps) {
  const [connecting, setConnecting] = useState(onCancel !== undefined)
  // Each opening of the sheet is its own pairing attempt.
  const [attempt, setAttempt] = useState(0)

  useEffect(() => () => setNativeChrome({}), [])

  function handleDismiss() {
    if (onCancel) return onCancel()
    setConnecting(false)
    setAttempt((n) => n + 1)
  }

  return (
    <main className="relative h-dvh overflow-hidden bg-brand-ground">
      {/* Larger while it is the whole screen; it settles at its top place and size for the sheet. */}
      <div
        data-state={connecting ? 'open' : 'closed'}
        className="group absolute inset-x-0 top-1/2 flex -translate-y-[calc(50%+3rem)] flex-col items-center gap-4 px-6 text-center transition-[top,translate] duration-500 ease-out data-[state=open]:top-[calc(env(safe-area-inset-top)+2.5rem)] data-[state=open]:translate-y-0 motion-reduce:transition-none"
      >
        <CrowbarWordmark
          role="img"
          aria-label="Crowbar"
          className="w-44 -translate-y-3 scale-150 text-white transition-[translate,scale] duration-500 ease-out group-data-[state=open]:translate-y-0 group-data-[state=open]:scale-100 motion-reduce:transition-none"
        />
        <p className="max-w-xs translate-y-2 scale-115 text-base text-white/80 transition-[translate,scale] duration-500 ease-out group-data-[state=open]:translate-y-0 group-data-[state=open]:scale-100 motion-reduce:transition-none">
          {OOBE_TAGLINE}
        </p>
      </div>
      <NativeConnect
        key={attempt}
        open={connecting}
        onStart={() => setConnecting(true)}
        onDismiss={handleDismiss}
        onPaired={(hostId) => {
          setConnecting(false)
          onPaired(hostId)
        }}
      />
    </main>
  )
}

interface NativeConnectProps {
  open: boolean
  onStart: () => void
  onDismiss: () => void
  onPaired: (hostId: string) => void
}

/** One pairing attempt, drawn by the native chrome: it renders nothing of its own. */
function NativeConnect({ open, onStart, onDismiss, onPaired }: NativeConnectProps) {
  const [pending, setPending] = useState<PairingPending>(null)
  const flow = usePairingFlow({ onPaired, pending, onPendingChange: setPending })
  const { stage, name, address, code, fingerprint } = flow
  const error = flow.error ?? (flow.turnedAway ? CODE_SHAPE_NOTICE : null)
  const busy = pending !== null
  // The accent is read from CSS, so a light/dark flip must hand it over again.
  const themeVersion = useThemeVersion()
  const appearance = useSettingsStore((s) => s.settings.themeMode)

  useEffect(() => {
    setNativeChrome({
      accent: getComputedStyle(document.documentElement).getPropertyValue('--brand').trim(),
      welcome: { visible: !open, label: WELCOME_LABEL },
      connect: { open, stage, name, address, code, fingerprint, busy, error },
      appearance,
      palette: readNativePalette(),
    })
  }, [open, stage, name, address, code, fingerprint, busy, error, themeVersion, appearance])

  const handleIntent = useEffectEvent((intent: NativeChromeIntent) => {
    switch (intent.type) {
      case 'start':
        return onStart()
      case 'dismiss':
        return onDismiss()
      case 'back':
        return flow.back()
      case 'submitAddress':
        return void flow.submitAddress({ name: intent.name, address: intent.address })
      case 'confirmFingerprint':
        return flow.confirmFingerprint()
      case 'submitCode':
        return void flow.submitCode(intent.code)
    }
  })
  useEffect(() => onNativeChromeIntent(handleIntent), [])

  return null
}
