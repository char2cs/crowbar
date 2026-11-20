/** The shell's viewed map of one workspace: path to signature. */
export interface ReviewViewedFrame {
  workspace: string
  viewed: Record<string, string>
}
