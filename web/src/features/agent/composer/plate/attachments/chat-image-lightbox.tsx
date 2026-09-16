'use client'

import { useState } from 'react'
import {
  Dialog,
  DialogBackdrop,
  DialogPortal,
  DialogPrimitive,
  DialogViewport,
} from '@/components/ui/dialog'
import { CloseIcon } from '@/features/agent/shared/agent-icons'

/** Local open/close state for one image's lightbox — a hook rather than a
 *  bare `useState` call at each use site so the "closed by default" default
 *  and the prop names stay identical everywhere this is wired in. */
export function useChatImageLightbox() {
  const [open, setOpen] = useState(false)
  return { open, onOpenChange: setOpen, show: () => setOpen(true) }
}

interface ChatImageLightboxProps {
  src: string
  alt: string
  open: boolean
  onOpenChange: (open: boolean) => void
}

/**
 * Click-to-expand full-size view of an already-rendered chat image.
 *
 * Built from the same primitives DialogPopup itself composes from
 * (DialogBackdrop + DialogViewport, both exported from dialog.tsx for
 * exactly this), swapping only the bordered max-w-lg card for a bespoke,
 * borderless `DialogPrimitive.Popup` sized to the image instead of a fixed
 * card width — there is no existing lightbox pattern anywhere else in the
 * app to match, so this one follows house dialog conventions as closely as
 * an edge-to-edge image view can. Backdrop click and Escape both dismiss,
 * for free, the same as every other Dialog in the app.
 */
export function ChatImageLightbox({ src, alt, open, onOpenChange }: ChatImageLightboxProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogPortal>
        <DialogBackdrop />
        <DialogViewport className="grid-rows-1 items-center p-4">
          <DialogPrimitive.Popup
            data-slot="chat-image-lightbox"
            className="relative row-start-1 max-h-[88vh] max-w-[88vw] origin-center opacity-[calc(1-var(--nested-dialogs))] outline-none transition-[scale,opacity] duration-200 ease-in-out data-ending-style:scale-98 data-ending-style:opacity-0 data-starting-style:scale-98 data-starting-style:opacity-0"
          >
            <img
              src={src}
              alt={alt}
              className="block max-h-[88vh] max-w-[88vw] rounded-lg object-contain shadow-2xl"
            />
            <DialogPrimitive.Close
              aria-label="Close"
              className="-top-3.5 -right-3.5 absolute flex size-8 items-center justify-center rounded-full border bg-popover text-foreground shadow-md outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <CloseIcon size={14} />
            </DialogPrimitive.Close>
          </DialogPrimitive.Popup>
        </DialogViewport>
      </DialogPortal>
    </Dialog>
  )
}
