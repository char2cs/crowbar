import { useId, useRef, useState } from 'react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogFooter,
  DialogHeader,
  DialogPopup,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { OTPField, OTPFieldInput, OTPFieldSeparator } from '@/components/ui/otp-field'
import { probeRemoteFingerprint } from '@/lib/crowbar-bridge'
import {
  alreadyPairedReason,
  pairingFailureText,
  redeemPairingCode,
} from '@/features/hosts/lib/pairing'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'

type Stage = 'address' | 'fingerprint' | 'code'

/** The host's answer the dialog is waiting on; it cannot be dismissed meanwhile. */
type Pending = 'probe' | 'pair' | null

interface AddHostDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** The host just paired, once it is saved; the dialog closes either way. */
  onPaired?: (hostId: string) => void
}

// Lowercase letters, digits and hyphens only, per register_remote_host's
// accepted hostId shape — a random suffix keeps two hosts named the same
// from colliding.
function makeHostId(name: string): string {
  const slug = name
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
  return `${slug || 'host'}-${crypto.randomUUID().slice(0, 8)}`
}

const focusOnMount = (element: HTMLElement | null) => element?.focus()

// Full-width digits (an input method types them) read as the digits they are; the rest is dropped.
const toAsciiDigits = (value: string) => value.normalize('NFKC').replace(/\D/g, '')

// Base UI rewrites a slot and moves focus on every keystroke and input, which aborts an input
// method's composition: the field sees none until compositionend replays the committed text.
function holdWhileComposing(
  event: React.SyntheticEvent<HTMLDivElement, InputEvent | KeyboardEvent>,
) {
  if (event.nativeEvent.isComposing) event.stopPropagation()
}
const replayCommittedText = (event: React.CompositionEvent<HTMLDivElement>) =>
  event.target.dispatchEvent(new Event('input', { bubbles: true }))

/** Adds a host: probe its certificate, have the user confirm the fingerprint, then redeem a code
 *  from its `crowbar pair`. The code stage is reached only by that confirm, and pins exactly the
 *  fingerprint shown — never a re-probed one. */
export function AddHostDialog({ open, onOpenChange, onPaired }: AddHostDialogProps) {
  const [pending, setPending] = useState<Pending>(null)

  function handleOpenChange(next: boolean) {
    if (!next && pending) return
    onOpenChange(next)
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogPopup className="max-w-md">
        <DialogHeader>
          <DialogTitle>Add remote host</DialogTitle>
        </DialogHeader>
        {/* Lives as long as the popup, which unmounts once closed, and callers key the dialog per
            opening (one during the close too): a confirmed fingerprint never reaches the next. */}
        <AddHostAttempt
          pending={pending}
          onPendingChange={setPending}
          onCancel={() => handleOpenChange(false)}
          onPaired={(hostId) => {
            onOpenChange(false)
            onPaired?.(hostId)
          }}
        />
      </DialogPopup>
    </Dialog>
  )
}

interface AddHostAttemptProps {
  pending: Pending
  onPendingChange: (pending: Pending) => void
  onCancel: () => void
  onPaired: (hostId: string) => void
}

function AddHostAttempt({ pending, onPendingChange, onCancel, onPaired }: AddHostAttemptProps) {
  const [stage, setStage] = useState<Stage>('address')
  const [name, setName] = useState('')
  const [endpoint, setEndpoint] = useState('')
  const [fingerprint, setFingerprint] = useState('')
  const [code, setCode] = useState('')
  // The code was turned away for not being exactly 6 digits; the next change to it ends that.
  const [turnedAway, setTurnedAway] = useState(false)
  const [error, setError] = useState<string | null>(null)
  // Minted with the probe, so every code tried after one confirm pairs the same host id.
  const hostId = useRef('')
  const probing = pending === 'probe'
  const pairing = pending === 'pair'
  const codeHintId = useId()
  const fieldRef = useRef<HTMLDivElement>(null)
  const pairRef = useRef<HTMLButtonElement>(null)
  const promptId = useId()
  const fingerprintId = useId()
  // An entry past the sixth slot is kept (to be refused whole) but never shown.
  const shownCode = code.length > 6 ? '' : code
  const codeValid = /^\d{6}$/.test(code)

  async function handleProbe(e: React.FormEvent) {
    e.preventDefault()
    const trimmedName = name.trim()
    const trimmedEndpoint = endpoint.trim()
    if (!trimmedName || !trimmedEndpoint) return
    setError(null)
    onPendingChange('probe')
    try {
      const fp = await probeRemoteFingerprint(trimmedEndpoint)
      const hosts = useHostConfigStore.getState().hosts
      const paired = alreadyPairedReason(hosts, trimmedEndpoint, fp)
      if (paired) {
        setError(paired)
        return
      }
      hostId.current = makeHostId(trimmedName)
      setFingerprint(fp)
      setStage('fingerprint')
    } catch (err) {
      setError(pairingFailureText(err, trimmedEndpoint))
    } finally {
      onPendingChange(null)
    }
  }

  function handleConfirmFingerprint() {
    setError(null)
    setStage('code')
  }

  // A failure about the old code is stale the moment the code changes.
  function handleCodeChange(next: string) {
    setCode(next)
    setError(null)
    setTurnedAway(false)
  }

  const codeSlots = () => Array.from(fieldRef.current?.querySelectorAll('input') ?? [])

  // Base UI cuts an entry that runs past the sixth slot, leaving a wrong code that looks whole.
  // Kept whole instead, it is no code: the slots empty and the correction says why.
  function refuseOverrun(event: React.SyntheticEvent<HTMLDivElement>, text: string) {
    if (pairing) return
    const slots = codeSlots()
    const at = slots.indexOf(event.target as HTMLInputElement)
    const digits = toAsciiDigits(text)
    if (at + digits.length <= slots.length) return
    event.preventDefault()
    slots[0].focus()
    setError(null)
    setTurnedAway(true)
    setCode(code.slice(0, at) + digits + code.slice(at + digits.length))
  }

  // A pressed Pair button, or a click that focused nothing, is where focus is when a code fails:
  // the user retypes from the first slot instead of from the page behind the dialog.
  function refocusCode() {
    const active = document.activeElement
    if (active === pairRef.current || active === document.body) codeSlots()[0]?.focus()
  }

  async function handleRedeem(e: React.FormEvent) {
    e.preventDefault()
    if (pairing) return
    if (!codeValid) {
      setTurnedAway(true)
      codeSlots()[Math.min(shownCode.length, 5)]?.focus()
      return
    }
    setError(null)
    onPendingChange('pair')
    try {
      await redeemPairingCode(hostId.current, endpoint.trim(), fingerprint, code, name.trim())
      onPaired(hostId.current)
    } catch (err) {
      setError(pairingFailureText(err, endpoint.trim()))
      refocusCode()
    } finally {
      onPendingChange(null)
    }
  }

  return (
    <>
      {stage === 'address' && (
        <form onSubmit={handleProbe}>
          <div className="space-y-5 px-6 py-5">
            <p className="text-sm text-muted-foreground">
              Run crowbar pair on the host first: it shows the fingerprint and the code you’ll need.
            </p>
            <div className="space-y-2">
              <Label htmlFor="add-host-name">Name</Label>
              <Input
                id="add-host-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="homelab"
                maxLength={64}
                disabled={probing}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="add-host-address">Address</Label>
              <Input
                id="add-host-address"
                value={endpoint}
                onChange={(e) => setEndpoint(e.target.value)}
                placeholder="1.2.3.4:3737"
                className="font-mono text-[12px]"
                disabled={probing}
              />
            </div>
            {error && (
              <p role="alert" className="text-[12px] text-destructive">
                {error}
              </p>
            )}
          </div>
          <DialogFooter>
            <Button variant="ghost" type="button" disabled={probing} onClick={onCancel}>
              Cancel
            </Button>
            <Button type="submit" disabled={!name.trim() || !endpoint.trim() || probing}>
              {probing ? 'Connecting…' : 'Continue'}
            </Button>
          </DialogFooter>
        </form>
      )}

      {stage === 'fingerprint' && (
        <div>
          <div className="space-y-3 px-6 py-5">
            <p id={promptId} className="text-sm text-muted-foreground">
              Confirm this matches the fingerprint printed by that host’s CLI:
            </p>
            {/* Focused on arrival instead of Confirm: a stray or repeated Enter accepts nothing. */}
            <div
              ref={focusOnMount}
              id={fingerprintId}
              role="group"
              tabIndex={-1}
              aria-labelledby={`${promptId} ${fingerprintId}`}
              className="break-all rounded-lg border bg-muted/50 px-3 py-2 font-mono text-[12px]"
            >
              {fingerprint}
            </div>
            {error && (
              <p role="alert" className="text-[12px] text-destructive">
                {error}
              </p>
            )}
          </div>
          <DialogFooter>
            <Button variant="ghost" type="button" onClick={() => setStage('address')}>
              Back
            </Button>
            <Button type="button" onClick={handleConfirmFingerprint}>
              Confirm fingerprint
            </Button>
          </DialogFooter>
        </div>
      )}

      {stage === 'code' && (
        <form onSubmit={handleRedeem}>
          <div className="space-y-2 px-6 py-5">
            <Label htmlFor="add-host-code">Pairing code</Label>
            <OTPField
              ref={fieldRef}
              id="add-host-code"
              length={6}
              value={shownCode}
              onValueChange={handleCodeChange}
              validationType="none"
              inputMode="numeric"
              normalizeValue={toAsciiDigits}
              // Not `disabled`: that would drop the focus the user is typing in.
              readOnly={pairing}
              onPasteCapture={(e) => refuseOverrun(e, e.clipboardData.getData('text/plain'))}
              onChangeCapture={(e) => refuseOverrun(e, (e.target as HTMLInputElement).value)}
              onInputCapture={holdWhileComposing}
              onKeyDownCapture={holdWhileComposing}
              onCompositionEnd={replayCommittedText}
            >
              <OTPFieldInput autoFocus aria-describedby={codeHintId} />
              <OTPFieldInput aria-label="Character 2 of 6" aria-describedby={codeHintId} />
              <OTPFieldInput aria-label="Character 3 of 6" aria-describedby={codeHintId} />
              <OTPFieldSeparator />
              <OTPFieldInput aria-label="Character 4 of 6" aria-describedby={codeHintId} />
              <OTPFieldInput aria-label="Character 5 of 6" aria-describedby={codeHintId} />
              <OTPFieldInput aria-label="Character 6 of 6" aria-describedby={codeHintId} />
            </OTPField>
            <div className="text-[12px]">
              <p id={codeHintId} className="text-muted-foreground">
                The 6-digit code crowbar pair printed on the host.
              </p>
              {/* Live from the first render, outside aria-describedby: a region inserted with its
                  text may go unannounced, and a described-by change would be read twice. */}
              <p role="status" className="text-destructive">
                {turnedAway && 'A pairing code is exactly 6 digits.'}
              </p>
            </div>
            {error && (
              <p role="alert" className="text-[12px] text-destructive">
                {error}
              </p>
            )}
          </div>
          <DialogFooter>
            <Button
              variant="ghost"
              type="button"
              onClick={() => setStage('fingerprint')}
              disabled={pairing}
            >
              Back
            </Button>
            {/* aria-disabled, not disabled: it keeps focus while a code is out, and a press before
                the code is whole is answered instead of ignored. */}
            <Button
              ref={pairRef}
              type="submit"
              aria-disabled={!codeValid || pairing || undefined}
              aria-busy={pairing || undefined}
              className="aria-disabled:opacity-64"
            >
              {pairing ? 'Pairing…' : 'Pair'}
            </Button>
          </DialogFooter>
        </form>
      )}
    </>
  )
}
