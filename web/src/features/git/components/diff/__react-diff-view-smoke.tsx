/**
 * DE-RISK smoke component for react-diff-view v3 under React 19 + Vite.
 * This file is intentionally prefixed with __ to mark it as dev/test scaffolding.
 * It is NOT imported by any production code.
 */
import { Diff, Hunk } from 'react-diff-view'
import type { HunkData } from 'react-diff-view'
import 'react-diff-view/style/index.css'

const SMOKE_HUNKS: HunkData[] = [
  {
    content: '@@ -1,2 +1,2 @@',
    oldStart: 1,
    newStart: 1,
    oldLines: 2,
    newLines: 2,
    changes: [
      {
        type: 'delete',
        content: '-old line',
        lineNumber: 1,
        isDelete: true,
      },
      {
        type: 'insert',
        content: '+new line',
        lineNumber: 1,
        isInsert: true,
      },
      {
        type: 'normal',
        content: ' context line',
        isNormal: true,
        oldLineNumber: 2,
        newLineNumber: 2,
      },
    ],
  },
]

export function ReactDiffViewSmoke() {
  return (
    <Diff viewType="unified" diffType="modify" hunks={SMOKE_HUNKS}>
      {(hunks) => hunks.map((h) => <Hunk key={h.content} hunk={h} />)}
    </Diff>
  )
}
