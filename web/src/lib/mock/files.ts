// web/src/lib/mock/files.ts

export type GitStatus = 'modified' | 'added' | 'deleted' | 'renamed' | 'untracked'

export interface FileNode {
  name: string
  path: string
  isDirectory: boolean
  gitStatus?: GitStatus
  children?: FileNode[]
}

export function getMockFileTree(_rootPath: string): FileNode[] {
  return [
    {
      name: 'src',
      path: 'src',
      isDirectory: true,
      children: [
        {
          name: 'payment',
          path: 'src/payment',
          isDirectory: true,
          children: [
            { name: 'PaymentService.ts', path: 'src/payment/PaymentService.ts', isDirectory: false, gitStatus: 'modified' },
            { name: 'PaymentError.ts', path: 'src/payment/PaymentError.ts', isDirectory: false, gitStatus: 'added' },
            { name: 'webhook.ts', path: 'src/payment/webhook.ts', isDirectory: false },
            { name: 'payment.test.ts', path: 'src/payment/payment.test.ts', isDirectory: false, gitStatus: 'modified' },
          ],
        },
        {
          name: 'auth',
          path: 'src/auth',
          isDirectory: true,
          children: [
            { name: 'AuthService.ts', path: 'src/auth/AuthService.ts', isDirectory: false },
            { name: 'middleware.ts', path: 'src/auth/middleware.ts', isDirectory: false },
          ],
        },
        {
          name: 'db',
          path: 'src/db',
          isDirectory: true,
          children: [
            { name: 'schema.ts', path: 'src/db/schema.ts', isDirectory: false },
            { name: 'migrations.ts', path: 'src/db/migrations.ts', isDirectory: false },
          ],
        },
        { name: 'index.ts', path: 'src/index.ts', isDirectory: false },
        { name: 'config.ts', path: 'src/config.ts', isDirectory: false },
      ],
    },
    { name: 'package.json', path: 'package.json', isDirectory: false },
    { name: 'tsconfig.json', path: 'tsconfig.json', isDirectory: false },
    { name: 'README.md', path: 'README.md', isDirectory: false },
    { name: '.env.example', path: '.env.example', isDirectory: false, gitStatus: 'untracked' },
  ]
}
