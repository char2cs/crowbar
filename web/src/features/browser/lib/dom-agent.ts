import script from './page-agent.js?raw'

export type DomOp = 'snapshot' | 'click' | 'type' | 'press_key' | 'scroll' | 'wait'

/** The async function body that runs one DOM op in the page; it returns the op's result. */
export function domAgentBody(op: DomOp, args: Record<string, unknown>): string {
  return `const run = (${script})();\nreturn run(${JSON.stringify(op)}, ${JSON.stringify(args)});`
}
