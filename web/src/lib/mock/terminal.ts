// web/src/lib/mock/terminal.ts

export interface TerminalSeed {
  prompt: string
  priorOutput: string[]
}

export function getMockTerminalSeed(): TerminalSeed {
  return {
    prompt: '$ ',
    priorOutput: [
      '\x1b[1;32m~/projects/payment-api\x1b[0m on \x1b[1;35mfeat/payment-flow\x1b[0m',
      '❯ bun test src/payment/',
      '\x1b[32m✓\x1b[0m PaymentService › handleWebhook › validates payload \x1b[90m(12ms)\x1b[0m',
      '\x1b[32m✓\x1b[0m PaymentService › handleWebhook › throws PaymentError on invalid data \x1b[90m(8ms)\x1b[0m',
      '\x1b[32m✓\x1b[0m PaymentError › has typed error code \x1b[90m(3ms)\x1b[0m',
      '',
      '\x1b[32m3 tests passed\x1b[0m in 45ms',
      '',
      '\x1b[1;32m~/projects/payment-api\x1b[0m on \x1b[1;35mfeat/payment-flow\x1b[0m',
    ],
  }
}
