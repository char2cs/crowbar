import { Fragment, useRef, useState, type ChangeEvent, type ReactElement } from 'react'
import {
  AtIcon,
  CameraIcon,
  ClipboardTextIcon,
  ImagesIcon,
  PaperclipIcon,
  PencilSimpleLineIcon,
  TerminalWindowIcon,
} from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { PlusIcon } from '@/features/agent/shared/agent-icons'
import {
  ATTACH_SHORTCUT_LABEL,
  PASTE_SHORTCUT_LABEL,
  readClipboardImages,
} from '@/features/agent/composer/lib/plus-menu'
import { report, UNREACHED } from '@/lib/errors/report'
import { toast } from '@/features/window/stores/toast-store'
import { NativePlusMenuSheet } from '@/features/agent/composer/native-plus-menu'
import { hasNativeChrome } from '@/lib/native-chrome'
import { IS_MOBILE } from '@/utils/platform'

interface ComposerPlusButtonProps {
  onOpenExcalidraw: () => void
  onOpenAttachFile: () => void
  /** Images from the clipboard, the photo library or the camera: uploaded like a paste. */
  onFiles: (files: File[]) => void
  /** Types the character that opens the file or command picker. */
  onMentionFile: () => void
  onStartCommand: () => void
}

interface PlusItem {
  id: string
  label: string
  icon: ReactElement
  /** The Phosphor name the phone's native sheet draws; the rows without one stay off it. */
  glyph?: string
  hint?: string
  disabled?: boolean
  /** Starts the group after the divider. */
  divider?: boolean
  run: () => void
}

/** Styled neutrally: it opens a choice and sends nothing, so it never borrows
 *  send's primary treatment. A menu on a desktop, the native sheet on the phone. */
export function ComposerPlusButton({
  onOpenExcalidraw,
  onOpenAttachFile,
  onFiles,
  onMentionFile,
  onStartCommand,
}: ComposerPlusButtonProps) {
  const [open, setOpen] = useState(false)
  const photosRef = useRef<HTMLInputElement>(null)
  const cameraRef = useRef<HTMLInputElement>(null)

  const pasteImage = () => {
    readClipboardImages().then(
      (files) => {
        if (files.length > 0) onFiles(files)
        else toast.info('No image on the clipboard')
      },
      (err: unknown) =>
        report(err, { action: 'Could not read the clipboard', fallback: UNREACHED }),
    )
  }

  const picked = (event: ChangeEvent<HTMLInputElement>) => {
    onFiles(Array.from(event.target.files ?? []))
    event.target.value = ''
  }

  const attach: PlusItem = {
    id: 'attach',
    glyph: 'Paperclip',
    label: IS_MOBILE ? 'Attach files' : 'Attach files…',
    icon: <PaperclipIcon size={16} />,
    hint: IS_MOBILE ? undefined : ATTACH_SHORTCUT_LABEL,
    run: onOpenAttachFile,
  }
  const drawing: PlusItem = {
    id: 'drawing',
    glyph: 'PencilSimpleLine',
    label: 'New drawing',
    icon: <PencilSimpleLineIcon size={16} />,
    run: onOpenExcalidraw,
  }
  const items: PlusItem[] = [
    ...(IS_MOBILE
      ? [
          {
            id: 'photos',
            glyph: 'Images',
            label: 'Photo library',
            icon: <ImagesIcon size={16} />,
            run: () => photosRef.current?.click(),
          },
          {
            id: 'camera',
            glyph: 'Camera',
            label: 'Take a photo',
            icon: <CameraIcon size={16} />,
            run: () => cameraRef.current?.click(),
          },
          attach,
        ]
      : [
          attach,
          {
            id: 'paste',
            label: 'Paste image',
            icon: <ClipboardTextIcon size={16} />,
            hint: PASTE_SHORTCUT_LABEL,
            run: pasteImage,
          },
        ]),
    drawing,
    {
      id: 'mention',
      glyph: 'At',
      label: 'Mention a file',
      icon: <AtIcon size={16} />,
      hint: '@',
      divider: true,
      run: onMentionFile,
    },
    {
      id: 'command',
      glyph: 'TerminalWindow',
      label: 'Run a command',
      icon: <TerminalWindowIcon size={16} />,
      hint: '/',
      run: onStartCommand,
    },
  ]

  const nativeItems = items.flatMap((item) =>
    item.glyph
      ? [
          {
            id: item.id,
            title: item.label,
            icon: item.glyph,
            hint: item.hint ?? null,
            divider: item.divider ?? false,
          },
        ]
      : [],
  )

  const trigger = {
    className: 'plusbtn rounded-full',
    'data-open': open || undefined,
    'aria-label': 'Add to this message',
    title: 'Add to this message',
  }
  const glyph = <PlusIcon size={16} />

  return (
    <>
      {hasNativeChrome() ? (
        <>
          <Button variant="ghost" size="icon-xs" {...trigger} onClick={() => setOpen(true)}>
            {glyph}
          </Button>
          <NativePlusMenuSheet
            open={open}
            title="Add to this message"
            items={nativeItems}
            onPick={(id) => {
              setOpen(false)
              items.find((item) => item.id === id)?.run()
            }}
            onClose={() => setOpen(false)}
          />
        </>
      ) : (
        <DropdownMenu open={open} onOpenChange={setOpen}>
          {/* composer-pill.css sizes it to send's diameter (PLUS_DIAMETER in handle-geometry.ts). */}
          <DropdownMenuTrigger render={<Button variant="ghost" size="icon-xs" />} {...trigger}>
            {glyph}
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" side="top" sideOffset={8} className="plus-menu">
            {items.map((item) => (
              <Fragment key={item.id}>
                {item.divider && <DropdownMenuSeparator className="plus-rule" />}
                <DropdownMenuItem className="plus-item" disabled={item.disabled} onClick={item.run}>
                  {item.icon}
                  <span className="plus-label">{item.label}</span>
                  {item.hint && <span className="plus-hint">{item.hint}</span>}
                </DropdownMenuItem>
              </Fragment>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      )}
      {IS_MOBILE && (
        <>
          <input
            ref={photosRef}
            type="file"
            accept="image/*"
            multiple
            hidden
            aria-label="Photo library"
            onChange={picked}
          />
          <input
            ref={cameraRef}
            type="file"
            accept="image/*"
            capture="environment"
            hidden
            aria-label="Camera"
            onChange={picked}
          />
        </>
      )}
    </>
  )
}
