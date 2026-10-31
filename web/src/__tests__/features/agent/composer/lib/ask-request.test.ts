import { describe, expect, it } from 'vitest'
import { foldCode, foldedCue, requestView } from '@/features/agent/composer/lib/ask-request'

const folded = (command: string) =>
  foldCode(requestView({ toolName: 'Bash', toolInput: { command } }).code)

describe('foldedCue', () => {
  it('counts the characters cut from long lines as well as the lines left out', () => {
    const command = Array.from({ length: 13 }, () => 'y'.repeat(300)).join('\n')
    expect(foldedCue(folded(command))).toBe('1 more line and 720 more characters not shown')
  })

  it('says nothing when the card leaves nothing out', () => {
    expect(foldedCue(folded('ls'))).toBe('')
  })
})

describe('foldCode', () => {
  it('never cuts a line between the halves of a surrogate pair', () => {
    const line = `${'a'.repeat(239)}😀tail`
    const [shown] = folded(line).lines
    expect(shown.text).toBe('a'.repeat(239))
    expect(shown.cut).toBe(true)
  })
})
