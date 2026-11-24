/** Runs writes one after another, so a delete is never overtaken by the save it follows. A job's
 *  failure is the queue's loss: a cache write that fails never fails its caller or the next job. */
export class WriteQueue {
  private tail: Promise<void> = Promise.resolve()

  /** Runs `job` once every job pushed before it settles; resolves when it settles, either way. */
  push(job: () => Promise<unknown>): Promise<void> {
    this.tail = this.tail.then(job).then(
      () => undefined,
      () => undefined,
    )
    return this.tail
  }
}
