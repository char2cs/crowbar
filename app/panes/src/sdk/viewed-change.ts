import type { ViewedFile } from './viewed-file'

export interface ViewedChange {
  files: readonly ViewedFile[]
  viewed: boolean
}
