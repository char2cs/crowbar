import script from './page-agent.js?raw'
import pointerScript from './pointer-agent.js?raw'

export type DomOp = 'snapshot' | 'click' | 'type' | 'press_key' | 'scroll' | 'wait'
export type PointerOp = 'pick' | 'cancelPick' | 'resolve'

const POINTER_OPS: ReadonlySet<string> = new Set<PointerOp>(['pick', 'cancelPick', 'resolve'])

/** The async function body that runs one DOM op in the page; it returns the op's result. */
export function domAgentBody(op: DomOp | PointerOp, args: Record<string, unknown>): string {
  const call = `return run(${JSON.stringify(op)}, ${JSON.stringify(args)});`
  if (POINTER_OPS.has(op)) {
    return `const base = (${script})();\nconst run = (${pointerScript})(base);\n${call}`
  }
  return `const run = (${script})();\n${call}`
}
