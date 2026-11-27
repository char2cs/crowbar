import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogFooter,
  DialogHeader,
  DialogPopup,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import type { PairingOffer } from '@/features/hosts/lib/device-access'

export type Minted =
  { state: 'minting' } | { state: 'failed' } | { state: 'ready'; offer: PairingOffer }

interface AddDeviceDialogProps {
  minted: Minted | null
  onNewCode: () => void
  onClose: () => void
}

const groupCode = (code: string) => `${code.slice(0, 3)} ${code.slice(3)}`

const until = (iso: string) =>
  new Date(iso).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="space-y-2">
      <Label>{label}</Label>
      {children}
    </div>
  )
}

const VALUE_BOX = 'break-all rounded-lg border bg-muted/50 px-3 py-2 font-mono text-[12px]'

function OfferBody({ offer }: { offer: PairingOffer }) {
  return (
    <div className="space-y-5 px-6 py-5">
      <p className="text-sm text-muted-foreground">
        In Crowbar on the device, add a host with the address, check the fingerprint matches, then
        enter the code.
      </p>
      <Field label="Pairing code">
        <div
          aria-label={`Pairing code ${offer.code}`}
          className={`${VALUE_BOX} text-center text-2xl tracking-widest`}
        >
          {groupCode(offer.code)}
        </div>
        <p className="text-[12px] text-muted-foreground">
          Works once, until {until(offer.expiresAt)}.
        </p>
      </Field>
      <Field label={offer.addresses.length > 1 ? 'Addresses' : 'Address'}>
        {offer.addresses.length === 0 ? (
          <p className="text-[12px] text-muted-foreground">
            This computer’s own address and the listening port.
          </p>
        ) : (
          offer.addresses.map((address) => (
            <div key={address} className={VALUE_BOX}>
              {address}
            </div>
          ))
        )}
      </Field>
      <Field label="Fingerprint">
        <div className={VALUE_BOX}>{offer.serverFingerprint}</div>
      </Field>
    </div>
  )
}

export function AddDeviceDialog({ minted, onNewCode, onClose }: AddDeviceDialogProps) {
  return (
    <Dialog open={minted !== null} onOpenChange={(open) => !open && onClose()}>
      <DialogPopup className="max-w-md">
        <DialogHeader>
          <DialogTitle>Add a device</DialogTitle>
        </DialogHeader>
        {minted?.state === 'ready' && <OfferBody offer={minted.offer} />}
        {minted?.state === 'minting' && (
          <p role="status" className="px-6 py-5 text-sm text-muted-foreground">
            Making a code…
          </p>
        )}
        {minted?.state === 'failed' && (
          <p role="alert" className="px-6 py-5 text-sm text-destructive">
            Couldn’t make a code. Check that device connections are on, then try again.
          </p>
        )}
        <DialogFooter>
          <Button
            variant="ghost"
            type="button"
            disabled={minted?.state === 'minting'}
            onClick={onNewCode}
          >
            New code
          </Button>
          <Button type="button" onClick={onClose}>
            Done
          </Button>
        </DialogFooter>
      </DialogPopup>
    </Dialog>
  )
}
