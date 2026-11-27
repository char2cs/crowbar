export interface PlacementRecord {
  key: string
  hostId: string
  kind: 'workspace' | 'chat'
  id: string
}
