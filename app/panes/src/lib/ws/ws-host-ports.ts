export interface WsHostPorts {
  isLocalHost(hostId: string): boolean
  mayDial(hostId: string): boolean
  hasLocalDaemon(): boolean
  onHostReady(hostId: string, listener: VoidFunction): VoidFunction
}
