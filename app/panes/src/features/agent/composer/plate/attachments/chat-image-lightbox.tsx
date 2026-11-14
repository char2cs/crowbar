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

/** DialogPopup's own primitives with a borderless popup sized to the image in place
 *  of its fixed-width card. */
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
