import type { MultiFileDiff } from '@/features/git/types/git-diff-types'

export function getMockBranchDiff(_wsId: string): MultiFileDiff {
  return {
    commitHash: 'mock-branch-diff',
    commitMessage: 'Mock branch changes',
    files: [
      {
        file_path: 'src/api/routes.ts',
        is_new: false,
        is_deleted: false,
        is_renamed: false,
        additions: 42,
        deletions: 8,
        lines: [
          { line_type: 'header', content: '@@ -1,8 +1,42 @@' },
          { line_type: 'context', content: 'import { Router } from "express"', old_line_number: 1, new_line_number: 1 },
          { line_type: 'removed', content: 'const router = Router()', old_line_number: 2 },
          { line_type: 'added', content: 'const router = Router({ strict: true })', new_line_number: 2 },
          { line_type: 'added', content: '', new_line_number: 3 },
          { line_type: 'added', content: 'router.use(rateLimit({ windowMs: 60_000, max: 100 }))', new_line_number: 4 },
          { line_type: 'context', content: '', old_line_number: 3, new_line_number: 5 },
          { line_type: 'context', content: 'export default router', old_line_number: 4, new_line_number: 6 },
        ],
      },
      {
        file_path: 'src/middleware/auth.ts',
        is_new: true,
        is_deleted: false,
        is_renamed: false,
        additions: 28,
        deletions: 0,
        lines: [
          { line_type: 'header', content: '@@ -0,0 +1,28 @@' },
          { line_type: 'added', content: 'import { verifyToken } from "../lib/jwt"', new_line_number: 1 },
          { line_type: 'added', content: '', new_line_number: 2 },
          { line_type: 'added', content: 'export function authMiddleware(req, res, next) {', new_line_number: 3 },
          { line_type: 'added', content: '  const token = req.headers.authorization?.split(" ")[1]', new_line_number: 4 },
          { line_type: 'added', content: '  if (!token) return res.status(401).json({ error: "Unauthorized" })', new_line_number: 5 },
          { line_type: 'added', content: '  req.user = verifyToken(token)', new_line_number: 6 },
          { line_type: 'added', content: '  next()', new_line_number: 7 },
          { line_type: 'added', content: '}', new_line_number: 8 },
        ],
      },
    ],
    totalFiles: 2,
    totalAdditions: 70,
    totalDeletions: 8,
  }
}
