/**
 * ContentSink — trailing-coalesce + de-dupe write buffer for Monaco content.
 *
 * Rapid `push()` calls accumulate the latest value; the actual `write` callback
 * fires only after `delayMs` of silence (trailing debounce). If the pending
 * value equals the last value already written, the write is skipped (de-dupe).
 * Call `flush()` to write immediately (e.g. on blur/save/close), or `dispose()`
 * to drop any pending write without flushing (e.g. on unmount after discard).
 */

export interface ContentSinkOptions {
  /** Called with the latest coalesced value when the trailing timer fires or on flush(). */
  write: (value: string) => void
  /** Debounce delay in milliseconds for the trailing write window. */
  delayMs: number
}

export class ContentSink {
  private readonly write: (value: string) => void
  private readonly delayMs: number

  /** The pending (not-yet-written) value, or undefined when nothing is queued. */
  private pending: string | undefined = undefined
  /** The most recent value that was actually passed to write(). */
  private lastWritten: string | undefined = undefined
  /** Active setTimeout handle, or undefined when no timer is running. */
  private timerId: ReturnType<typeof setTimeout> | undefined = undefined

  constructor({ write, delayMs }: ContentSinkOptions) {
    this.write = write
    this.delayMs = delayMs
  }

  /**
   * Enqueue a new value. Restarts the trailing timer; the previous pending
   * value (if any) is discarded — only the latest value matters.
   */
  push(value: string): void {
    this.pending = value
    this.clearTimer()
    this.timerId = setTimeout(() => {
      this.timerId = undefined
      this.commitPending()
    }, this.delayMs)
  }

  /**
   * Immediately write the pending value (if any and if it differs from the
   * last written value), then cancel the trailing timer.
   */
  flush(): void {
    this.clearTimer()
    this.commitPending()
  }

  /**
   * Cancel any pending write and discard the queued value without writing.
   * Safe to call multiple times.
   */
  dispose(): void {
    this.clearTimer()
    this.pending = undefined
  }

  // ---------------------------------------------------------------------------
  // Private helpers
  // ---------------------------------------------------------------------------

  private clearTimer(): void {
    if (this.timerId !== undefined) {
      clearTimeout(this.timerId)
      this.timerId = undefined
    }
  }

  /** Write pending if present and different from lastWritten, then clear it. */
  private commitPending(): void {
    if (this.pending !== undefined && this.pending !== this.lastWritten) {
      this.lastWritten = this.pending
      this.write(this.pending)
    }
    this.pending = undefined
  }
}
