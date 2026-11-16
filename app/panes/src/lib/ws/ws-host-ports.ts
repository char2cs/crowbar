/** What the manager asks of the host layer, which lib/ may not import. */
export interface WsHostPorts {
  isLocalHost(hostId: string): boolean
  /** False for a remote host that is down, needs repair or is not paired: its channels park. */
  mayDial(hostId: string): boolean
  hasLocalDaemon(): boolean
  /** `listener` fires each time `hostId` is up and dialable again; returns the unwatch. */
  onHostReady(hostId: string, listener: VoidFunction): VoidFunction
}
