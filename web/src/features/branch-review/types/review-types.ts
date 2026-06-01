export type MergeStrategy = 'merge' | 'squash' | 'rebase'

export interface ReviewMessage {
  id: string
  author: string | null  // agent name, or null = current user ("You")
  isAgent: boolean
  body: string
  createdAt: string
}

export interface ReviewThread {
  id: string
  filePath: string
  lineNumber: number     // new_line_number of the anchored diff line
  side: 'left' | 'right'
  messages: ReviewMessage[]
  isResolved: boolean
}
