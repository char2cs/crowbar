/** Hears a workspace's viewed map (path to signature). */
export interface ViewedListener {
  (viewed: Record<string, string>): void
}
