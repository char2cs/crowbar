/** Which host held a workspace or chat when last listed: all a cold start needs to place a pane. */
export interface PlacementRecord {
  key: string
  hostId: string
  kind: 'workspace' | 'chat'
  id: string
}
