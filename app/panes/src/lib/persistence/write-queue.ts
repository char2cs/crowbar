export class WriteQueue {
  private tail: Promise<void> = Promise.resolve()

  push(job: () => Promise<unknown>): Promise<void> {
    this.tail = this.tail.then(job).then(
      () => undefined,
      () => undefined,
    )
    return this.tail
  }
}
