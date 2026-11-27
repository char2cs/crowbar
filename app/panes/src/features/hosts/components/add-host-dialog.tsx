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
import { DEFAULT_PORT } from '@/features/hosts/lib/endpoint'
import {
  CODE_SHAPE_NOTICE,
  usePairingFlow,
  type PairingPending,
} from '@/features/hosts/hooks/use-pairing-flow'

interface AddHostDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onPaired?: (hostId: string) => void
}

const focusOnMount = (element: HTMLElement | null) => element?.focus()

const toAsciiDigits = (value: string) => value.normalize('NFKC').replace(/\D/g, '')

function holdWhileComposing(
  event: React.SyntheticEvent<HTMLDivElement, InputEvent | KeyboardEvent>,
) {
  if (event.nativeEvent.isComposing) event.stopPropagation()
}
const replayCommittedText = (event: React.CompositionEvent<HTMLDivElement>) =>
  event.target.dispatchEvent(new Event('input', { bubbles: true }))

export function AddHostDialog({ open, onOpenChange, onPaired }: AddHostDialogProps) {
  const [pending, setPending] = useState<PairingPending>(null)

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
        <AddHostForm
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

interface AddHostFormProps {
  pending: PairingPending
  onPendingChange: (pending: PairingPending) => void
  onCancel: () => void
  onPaired: (hostId: string) => void
}

function AddHostForm({ pending, onPendingChange, onCancel, onPaired }: AddHostFormProps) {
  const flow = usePairingFlow({ onPaired, pending, onPendingChange })
  const { stage, name, address, fingerprint, code, turnedAway, error } = flow
  const probing = pending === 'probe'
  const pairing = pending === 'pair'
  const codeHintId = useId()
  const addressHintId = useId()
  const fieldRef = useRef<HTMLDivElement>(null)
  const pairRef = useRef<HTMLButtonElement>(null)
  const promptId = useId()
  const fingerprintId = useId()
  const shownCode = code.length > 6 ? '' : code
  const codeValid = /^\d{6}$/.test(code)

  function handleProbe(e: React.FormEvent) {
    e.preventDefault()
    void flow.submitAddress()
  }

  const codeSlots = () => Array.from(fieldRef.current?.querySelectorAll('input') ?? [])

  function refuseOverrun(event: React.SyntheticEvent<HTMLDivElement>, text: string) {
    if (pairing) return
    const slots = codeSlots()
    const at = slots.indexOf(event.target as HTMLInputElement)
    const digits = toAsciiDigits(text)
    if (at + digits.length <= slots.length) return
    event.preventDefault()
    slots[0].focus()
    flow.refuseCode(code.slice(0, at) + digits + code.slice(at + digits.length))
  }

  function refocusCode() {
    const active = document.activeElement
    if (active === pairRef.current || active === document.body) codeSlots()[0]?.focus()
  }

  async function handleRedeem(e: React.FormEvent) {
    e.preventDefault()
    const outcome = await flow.submitCode()
    if (outcome === 'invalid') codeSlots()[Math.min(shownCode.length, 5)]?.focus()
    if (outcome === 'failed') refocusCode()
  }

  return (
    <>
      {stage === 'address' && (
        <form onSubmit={handleProbe}>
          <div className="space-y-5 px-6 py-5">
            <p className="text-sm text-muted-foreground">
              On the host, open Settings → Devices and tap Add a device, or run crowbar pair: either
              shows the address, fingerprint and code you’ll need.
            </p>
            <div className="space-y-2">
              <Label htmlFor="add-host-name">Name</Label>
              <Input
                id="add-host-name"
                value={name}
                onChange={(e) => flow.setName(e.target.value)}
                placeholder="homelab"
                maxLength={64}
                disabled={probing}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="add-host-address">Address</Label>
              <Input
                id="add-host-address"
                value={address}
                onChange={(e) => flow.setAddress(e.target.value)}
                placeholder="homelab.example.com or 1.2.3.4"
                aria-describedby={addressHintId}
                autoCapitalize="off"
                autoCorrect="off"
                spellCheck={false}
                className="font-mono text-[12px]"
                disabled={probing}
              />
              <p id={addressHintId} className="text-[12px] text-muted-foreground">
                Uses port {DEFAULT_PORT} unless you add one, like :4000.
              </p>
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
            <Button type="submit" disabled={!name.trim() || !address.trim() || probing}>
              {probing ? 'Connecting…' : 'Continue'}
            </Button>
          </DialogFooter>
        </form>
      )}

      {stage === 'fingerprint' && (
        <div>
          <div className="space-y-3 px-6 py-5">
            <p id={promptId} className="text-sm text-muted-foreground">
              Confirm this matches the fingerprint shown on that host:
            </p>
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
            <Button variant="ghost" type="button" onClick={flow.back}>
              Back
            </Button>
            <Button type="button" onClick={flow.confirmFingerprint}>
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
              onValueChange={flow.setCode}
              validationType="none"
              inputMode="numeric"
              normalizeValue={toAsciiDigits}
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
                The 6-digit code shown on the host.
              </p>
              <p role="status" className="text-destructive">
                {turnedAway && CODE_SHAPE_NOTICE}
              </p>
            </div>
            {error && (
              <p role="alert" className="text-[12px] text-destructive">
                {error}
              </p>
            )}
          </div>
          <DialogFooter>
            <Button variant="ghost" type="button" onClick={flow.back} disabled={pairing}>
              Back
            </Button>
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
