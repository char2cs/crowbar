import { useState, type FormEvent } from 'react'
import {
  ArrowDownIcon,
  ArrowLeftIcon,
  ArrowRightIcon,
  ArrowUpIcon,
  PaperPlaneRightIcon,
} from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

const KEYS = [
  { label: 'Esc', name: 'Escape', bytes: '\x1b' },
  { label: 'Tab', name: 'Tab', bytes: '\t' },
] as const

const ARROWS = [
  { Icon: ArrowLeftIcon, name: 'Arrow left', bytes: '\x1b[D' },
  { Icon: ArrowUpIcon, name: 'Arrow up', bytes: '\x1b[A' },
  { Icon: ArrowDownIcon, name: 'Arrow down', bytes: '\x1b[B' },
  { Icon: ArrowRightIcon, name: 'Arrow right', bytes: '\x1b[C' },
] as const

/** ^A..^Z and the ^[ ^\ ^] ^^ ^_ punctuation: the character's code with bit 6 cleared. */
function ctrlChord(char: string): string | null {
  const code = char.toUpperCase().charCodeAt(0)
  return code >= 64 && code <= 95 ? String.fromCharCode(code - 64) : null
}

/** The keys a phone keyboard lacks (Esc, Tab, Ctrl, arrows) and a line field to type into,
 *  for the phone's terminal screen. `onSend` gets the bytes as the PTY should read them. */
export function TerminalKeyBar({ onSend }: { onSend: (bytes: string) => void }) {
  const [line, setLine] = useState('')
  const [ctrl, setCtrl] = useState(false)

  function handleChange(next: string) {
    const chord = ctrl ? ctrlChord(next.slice(-1)) : null
    if (chord === null) {
      setLine(next)
      return
    }
    setCtrl(false)
    setLine('')
    onSend(chord)
  }

  function handleSubmit(event: FormEvent) {
    event.preventDefault()
    onSend(`${line}\r`)
    setLine('')
  }

  return (
    <div className="flex shrink-0 flex-col gap-2 border-t bg-background px-2 pt-2 pb-[calc(env(safe-area-inset-bottom)+0.5rem)]">
      <div className="flex items-center gap-1 overflow-x-auto">
        {KEYS.map(({ label, name, bytes }) => (
          <Button
            key={name}
            variant="outline"
            size="sm"
            aria-label={name}
            onClick={() => onSend(bytes)}
          >
            {label}
          </Button>
        ))}
        <Button
          variant={ctrl ? 'default' : 'outline'}
          size="sm"
          aria-label="Control"
          aria-pressed={ctrl}
          onClick={() => setCtrl((on) => !on)}
        >
          Ctrl
        </Button>
        {ARROWS.map(({ Icon, name, bytes }) => (
          <Button
            key={name}
            variant="outline"
            size="icon-sm"
            aria-label={name}
            onClick={() => onSend(bytes)}
          >
            <Icon />
          </Button>
        ))}
      </div>
      <form className="flex items-center gap-2" onSubmit={handleSubmit}>
        <Input
          aria-label="Terminal input"
          value={line}
          onChange={(e) => handleChange(e.target.value)}
          autoCapitalize="off"
          autoCorrect="off"
          spellCheck={false}
          className="font-mono"
        />
        <Button type="submit" size="icon" aria-label="Send line">
          <PaperPlaneRightIcon />
        </Button>
      </form>
    </div>
  )
}
