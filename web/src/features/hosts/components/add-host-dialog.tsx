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

/**
 * Two-stage add-host flow: (1) name/address -> probe the presented TLS
 * certificate and require the human to explicitly confirm its fingerprint,
 * (2) redeem a pairing code minted by that host's own CLI. Stage 2 is only
 * ever reachable after that confirm click, and always reuses the exact
 * fingerprint shown in stage 1 — never a re-probed or re-derived value.
 */
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
  const [error, setError] = useState<string | null>(null)
  // Minted with the probe, so every code tried after one confirm pairs the same host id.
  const hostId = useRef('')
  const probing = pending === 'probe'
  const pairing = pending === 'pair'
  const codeHintId = useId()
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

  async function handleRedeem(e: React.FormEvent) {
    e.preventDefault()
    if (!codeValid) return
    setError(null)
    onPendingChange('pair')
    try {
      await redeemPairingCode(hostId.current, endpoint.trim(), fingerprint, code, name.trim())
      onPaired(hostId.current)
    } catch (err) {
      setError(pairingFailureText(err, endpoint.trim()))
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
            {error && <p className="text-[12px] text-destructive">{error}</p>}
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
            <p className="text-sm text-muted-foreground">
              Confirm this matches the fingerprint printed by that host's CLI:
            </p>
            <p className="break-all rounded-lg border bg-muted/50 px-3 py-2 font-mono text-[12px]">
              {fingerprint}
            </p>
            {error && <p className="text-[12px] text-destructive">{error}</p>}
          </div>
          <DialogFooter>
            <Button variant="ghost" type="button" onClick={() => setStage('address')}>
              Back
            </Button>
            <Button type="button" autoFocus onClick={handleConfirmFingerprint}>
              Confirm fingerprint
            </Button>
          </DialogFooter>
        </div>
      )}

      {stage === 'code' && (
        <form onSubmit={handleRedeem}>
          <div className="space-y-2 px-6 py-5">
            <Label htmlFor="add-host-code">Pairing code</Label>
            <Input
              id="add-host-code"
              autoFocus
              value={code}
              // Spaces only: a code copied from a terminal often brings some along.
              onChange={(e) => setCode(e.target.value.replace(/\s/g, ''))}
              placeholder="123456"
              inputMode="numeric"
              autoComplete="one-time-code"
              aria-describedby={codeHintId}
              className="font-mono text-[12px]"
              disabled={pairing}
            />
            <p id={codeHintId} className="text-[12px] text-muted-foreground">
              {code === '' || codeValid
                ? 'The 6-digit code crowbar pair printed on the host.'
                : 'A pairing code is exactly 6 digits.'}
            </p>
            {error && <p className="text-[12px] text-destructive">{error}</p>}
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
            <Button type="submit" disabled={!codeValid || pairing}>
              {pairing ? 'Pairing…' : 'Pair'}
            </Button>
          </DialogFooter>
        </form>
      )}
    </>
  )
}
