// web/src/lib/mock/files.ts
import type { FileEntry } from '@/features/file-system/types/app'

export type GitStatus = 'modified' | 'added' | 'deleted' | 'renamed' | 'untracked'

export function getMockFileTree(_rootPath: string): FileEntry[] {
  return [
    {
      name: 'src',
      path: 'src',
      isDir: true,
      children: [
        {
          name: 'payment',
          path: 'src/payment',
          isDir: true,
          children: [
            { name: 'PaymentService.ts', path: 'src/payment/PaymentService.ts', isDir: false, gitStatus: 'modified' },
            { name: 'PaymentError.ts', path: 'src/payment/PaymentError.ts', isDir: false, gitStatus: 'added' },
            { name: 'webhook.ts', path: 'src/payment/webhook.ts', isDir: false },
            { name: 'payment.test.ts', path: 'src/payment/payment.test.ts', isDir: false, gitStatus: 'modified' },
          ],
        },
        {
          name: 'auth',
          path: 'src/auth',
          isDir: true,
          children: [
            { name: 'AuthService.ts', path: 'src/auth/AuthService.ts', isDir: false },
            { name: 'middleware.ts', path: 'src/auth/middleware.ts', isDir: false },
          ],
        },
        {
          name: 'db',
          path: 'src/db',
          isDir: true,
          children: [
            { name: 'schema.ts', path: 'src/db/schema.ts', isDir: false },
            { name: 'migrations.ts', path: 'src/db/migrations.ts', isDir: false },
          ],
        },
        { name: 'index.ts', path: 'src/index.ts', isDir: false },
        { name: 'config.ts', path: 'src/config.ts', isDir: false },
      ],
    },
    { name: 'package.json', path: 'package.json', isDir: false },
    { name: 'tsconfig.json', path: 'tsconfig.json', isDir: false },
    { name: 'README.md', path: 'README.md', isDir: false },
    { name: '.env.example', path: '.env.example', isDir: false, gitStatus: 'untracked' },
  ]
}
