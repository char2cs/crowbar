/** A host the shell paired with this computer, as the shell's `hosts` frame names it. */
export interface PairedHost {
  id: string
  name: string
  /** `host:port`, as it was paired. */
  endpoint: string
}
