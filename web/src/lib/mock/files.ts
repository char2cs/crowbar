// web/src/lib/mock/files.ts
import type { FileEntry } from '@/features/file-system/types/app'

export type GitStatus = 'modified' | 'added' | 'deleted' | 'renamed' | 'untracked'

/** Public shape used by tests and consumers. */
export type FileNode = FileEntry

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

/** Realistic mock content for each file in the mock tree */
const MOCK_CONTENT: Record<string, string> = {
  'src/payment/PaymentService.ts': `import Stripe from 'stripe'
import { PaymentError } from './PaymentError'

const stripe = new Stripe(process.env.STRIPE_SECRET_KEY!, { apiVersion: '2023-10-16' })

export interface CreatePaymentIntentParams {
  amount: number
  currency: string
  customerId?: string
  metadata?: Record<string, string>
}

export async function createPaymentIntent(params: CreatePaymentIntentParams) {
  try {
    const intent = await stripe.paymentIntents.create({
      amount: params.amount,
      currency: params.currency,
      customer: params.customerId,
      metadata: params.metadata ?? {},
      automatic_payment_methods: { enabled: true },
    })
    return { clientSecret: intent.client_secret, id: intent.id }
  } catch (err) {
    throw new PaymentError(\`Failed to create payment intent: \${(err as Error).message}\`)
  }
}

export async function confirmPayment(paymentIntentId: string) {
  const intent = await stripe.paymentIntents.retrieve(paymentIntentId)
  return intent.status === 'succeeded'
}`,

  'src/payment/PaymentError.ts': `export class PaymentError extends Error {
  constructor(
    message: string,
    public readonly code?: string,
    public readonly stripeCode?: string,
  ) {
    super(message)
    this.name = 'PaymentError'
  }
}`,

  'src/payment/webhook.ts': `import type { Request, Response } from 'express'
import Stripe from 'stripe'

const stripe = new Stripe(process.env.STRIPE_SECRET_KEY!, { apiVersion: '2023-10-16' })
const webhookSecret = process.env.STRIPE_WEBHOOK_SECRET!

export async function handleWebhook(req: Request, res: Response) {
  const sig = req.headers['stripe-signature'] as string

  let event: Stripe.Event
  try {
    event = stripe.webhooks.constructEvent(req.body, sig, webhookSecret)
  } catch (err) {
    return res.status(400).send(\`Webhook Error: \${(err as Error).message}\`)
  }

  switch (event.type) {
    case 'payment_intent.succeeded':
      await handlePaymentSucceeded(event.data.object as Stripe.PaymentIntent)
      break
    case 'payment_intent.payment_failed':
      await handlePaymentFailed(event.data.object as Stripe.PaymentIntent)
      break
  }

  res.json({ received: true })
}

async function handlePaymentSucceeded(intent: Stripe.PaymentIntent) {
  console.log('Payment succeeded:', intent.id)
}

async function handlePaymentFailed(intent: Stripe.PaymentIntent) {
  console.error('Payment failed:', intent.id, intent.last_payment_error?.message)
}`,

  'src/payment/payment.test.ts': `import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createPaymentIntent, confirmPayment } from './PaymentService'
import { PaymentError } from './PaymentError'

vi.mock('stripe')

describe('PaymentService', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('creates a payment intent with the correct params', async () => {
    const result = await createPaymentIntent({
      amount: 2000,
      currency: 'usd',
      metadata: { orderId: 'ord_123' },
    })
    expect(result.clientSecret).toBeDefined()
    expect(result.id).toBeDefined()
  })

  it('throws PaymentError when Stripe fails', async () => {
    await expect(createPaymentIntent({ amount: -1, currency: 'usd' }))
      .rejects
      .toThrow(PaymentError)
  })
})`,

  'src/auth/AuthService.ts': `import jwt from 'jsonwebtoken'
import { db } from '../db/schema'

const JWT_SECRET = process.env.JWT_SECRET!
const JWT_EXPIRES_IN = '7d'

export interface TokenPayload {
  userId: string
  email: string
  role: 'user' | 'admin'
}

export function signToken(payload: TokenPayload): string {
  return jwt.sign(payload, JWT_SECRET, { expiresIn: JWT_EXPIRES_IN })
}

export function verifyToken(token: string): TokenPayload {
  return jwt.verify(token, JWT_SECRET) as TokenPayload
}

export async function authenticateUser(email: string, password: string) {
  const user = await db.users.findByEmail(email)
  if (!user) throw new Error('Invalid credentials')

  const valid = await user.comparePassword(password)
  if (!valid) throw new Error('Invalid credentials')

  return signToken({ userId: user.id, email: user.email, role: user.role })
}`,

  'src/auth/middleware.ts': `import type { Request, Response, NextFunction } from 'express'
import { verifyToken } from './AuthService'

export function requireAuth(req: Request, res: Response, next: NextFunction) {
  const authHeader = req.headers.authorization
  if (!authHeader?.startsWith('Bearer ')) {
    return res.status(401).json({ error: 'Missing or invalid Authorization header' })
  }

  const token = authHeader.slice(7)
  try {
    const payload = verifyToken(token)
    req.user = payload
    next()
  } catch {
    res.status(401).json({ error: 'Invalid or expired token' })
  }
}`,

  'src/db/schema.ts': `import { pgTable, uuid, text, varchar, timestamp, pgEnum } from 'drizzle-orm/pg-core'

export const roleEnum = pgEnum('role', ['user', 'admin'])

export const users = pgTable('users', {
  id:        uuid('id').primaryKey().defaultRandom(),
  email:     varchar('email', { length: 255 }).notNull().unique(),
  password:  text('password').notNull(),
  role:      roleEnum('role').default('user').notNull(),
  createdAt: timestamp('created_at').defaultNow().notNull(),
  updatedAt: timestamp('updated_at').defaultNow().notNull(),
})

export const sessions = pgTable('sessions', {
  id:        uuid('id').primaryKey().defaultRandom(),
  userId:    uuid('user_id').references(() => users.id).notNull(),
  token:     text('token').notNull().unique(),
  expiresAt: timestamp('expires_at').notNull(),
  createdAt: timestamp('created_at').defaultNow().notNull(),
})`,

  'src/db/migrations.ts': `import { drizzle } from 'drizzle-orm/postgres-js'
import { migrate } from 'drizzle-orm/postgres-js/migrator'
import postgres from 'postgres'

const connectionString = process.env.DATABASE_URL!
const sql = postgres(connectionString, { max: 1 })
const db = drizzle(sql)

async function runMigrations() {
  console.log('Running migrations...')
  await migrate(db, { migrationsFolder: './drizzle' })
  console.log('Migrations complete')
  await sql.end()
}

runMigrations().catch((err) => {
  console.error('Migration failed:', err)
  process.exit(1)
})`,

  'src/index.ts': `import express from 'express'
import cors from 'cors'
import helmet from 'helmet'
import { requireAuth } from './auth/middleware'
import { handleWebhook } from './payment/webhook'

const app = express()
const PORT = process.env.PORT ?? 3000

app.use(helmet())
app.use(cors({ origin: process.env.ALLOWED_ORIGINS?.split(',') }))
app.use(express.json())

// Public routes
app.post('/auth/login', async (req, res) => {
  const { email, password } = req.body
  try {
    const token = await import('./auth/AuthService').then(m => m.authenticateUser(email, password))
    res.json({ token })
  } catch (err) {
    res.status(401).json({ error: (err as Error).message })
  }
})

// Stripe webhook (raw body required)
app.post('/webhooks/stripe', express.raw({ type: 'application/json' }), handleWebhook)

// Protected routes
app.use('/api', requireAuth)
app.get('/api/me', (req, res) => res.json({ user: req.user }))

app.listen(PORT, () => console.log(\`Server running on port \${PORT}\`))`,

  'src/config.ts': `export const config = {
  database: {
    url: process.env.DATABASE_URL ?? 'postgresql://localhost:5432/crowbar_dev',
    maxConnections: parseInt(process.env.DB_MAX_CONNECTIONS ?? '10'),
  },
  jwt: {
    secret: process.env.JWT_SECRET ?? 'dev-secret-change-in-production',
    expiresIn: process.env.JWT_EXPIRES_IN ?? '7d',
  },
  stripe: {
    secretKey: process.env.STRIPE_SECRET_KEY ?? '',
    webhookSecret: process.env.STRIPE_WEBHOOK_SECRET ?? '',
  },
  server: {
    port: parseInt(process.env.PORT ?? '3000'),
    corsOrigins: process.env.ALLOWED_ORIGINS?.split(',') ?? ['http://localhost:5173'],
  },
} as const`,

  'package.json': `{
  "name": "crowbar-api",
  "version": "0.1.0",
  "type": "module",
  "scripts": {
    "dev": "tsx watch src/index.ts",
    "build": "tsc",
    "start": "node dist/index.js",
    "test": "vitest",
    "db:migrate": "tsx src/db/migrations.ts",
    "db:generate": "drizzle-kit generate"
  },
  "dependencies": {
    "cors": "^2.8.5",
    "drizzle-orm": "^0.29.3",
    "express": "^4.18.2",
    "helmet": "^7.1.0",
    "jsonwebtoken": "^9.0.2",
    "postgres": "^3.4.3",
    "stripe": "^14.12.0"
  },
  "devDependencies": {
    "@types/express": "^4.17.21",
    "@types/jsonwebtoken": "^9.0.5",
    "@types/node": "^20.11.5",
    "drizzle-kit": "^0.20.14",
    "tsx": "^4.7.0",
    "typescript": "^5.3.3",
    "vitest": "^1.2.2"
  }
}`,

  'tsconfig.json': `{
  "compilerOptions": {
    "target": "ES2022",
    "module": "NodeNext",
    "moduleResolution": "NodeNext",
    "lib": ["ES2022"],
    "outDir": "dist",
    "rootDir": "src",
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "forceConsistentCasingInFileNames": true,
    "resolveJsonModule": true,
    "declaration": true,
    "declarationMap": true,
    "sourceMap": true
  },
  "include": ["src/**/*"],
  "exclude": ["node_modules", "dist"]
}`,

  'README.md': `# Crowbar API

Backend service for the Crowbar agentic IDE platform.

## Setup

\`\`\`bash
npm install
cp .env.example .env
# Edit .env with your credentials
npm run db:migrate
npm run dev
\`\`\`

## Environment Variables

| Variable | Description |
|---|---|
| DATABASE_URL | PostgreSQL connection string |
| JWT_SECRET | Secret for signing JWTs |
| STRIPE_SECRET_KEY | Stripe API key |
| STRIPE_WEBHOOK_SECRET | Stripe webhook signing secret |
| PORT | Server port (default: 3000) |`,

  '.env.example': `DATABASE_URL=postgresql://localhost:5432/crowbar_dev
JWT_SECRET=change-me-in-production
JWT_EXPIRES_IN=7d
STRIPE_SECRET_KEY=sk_test_...
STRIPE_WEBHOOK_SECRET=whsec_...
PORT=3000
ALLOWED_ORIGINS=http://localhost:5173`,
}

const DEFAULT_CONTENT = `// File content not available in mock mode`

export function getMockFileContent(path: string): string {
  // Strip leading repo prefix (e.g. '/repos/crowbar/')
  const normalizedPath = path.replace(/^\/repos\/[^/]+\//, '')
  return MOCK_CONTENT[normalizedPath] ?? MOCK_CONTENT[path] ?? DEFAULT_CONTENT
}
