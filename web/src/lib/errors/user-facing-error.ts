/** A client failure worded for the user: report() shows its message as it is. Any other
 *  client Error is a bug whose text means nothing to them, so it stays in the log. */
export class UserFacingError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'UserFacingError'
  }
}
