import type { ViewedFile } from './viewed-file'

/** Files to mark viewed, or to clear the marks of. */
export interface ViewedChange {
  files: readonly ViewedFile[]
  viewed: boolean
}
