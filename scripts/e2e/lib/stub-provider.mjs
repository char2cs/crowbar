import { mkdirSync, symlinkSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'

export const STUB_PROVIDER_ID = 'e2estub'

// `cat` stays alive on its PTY, so a chat keeps a live runner while the test
// plays the provider's hooks at the daemon by hand: no login, no network, and
// the same hook path a real CLI uses. The symlink gives the process a name the
// suite can count with `ps` without ever matching the user's own `cat`.
export const STUB_BIN_NAME = 'e2e-stub-provider'

function descriptor(bin) {
  return `id: ${STUB_PROVIDER_ID}
spawn:
  cmd: "${bin}"
  interactive_required: true
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  user_prompt:
    in: user_prompt
    map:
      message: prompt
      subagent_id: agent_id
  turn_stop:
    in: turn_stop
    map:
      session_id: session_id
      message: last_assistant_message
  message_delta:
    in: message_delta
    map:
      session_id: session_id
      turn_id: turn_id
      message_id: message_id
      index: index
      final: final
      text: delta
  reasoning_delta:
    in: reasoning_delta
    map:
      session_id: session_id
      turn_id: turn_id
      message_id: message_id
      index: index
      final: final
      text: delta
  tool_pre:
    in: tool_pre
    map:
      session_id: session_id
      tool_id: tool_use_id
      tool_name: tool_name
      tool_target: tool_input.command
      subagent_id: agent_id
  tool_post:
    in: tool_post
    map:
      session_id: session_id
      tool_id: tool_use_id
      tool_name: tool_name
      subagent_id: agent_id
  permission:
    ask: permission
    timeout_seconds: 120
    map:
      session_id: session_id
      prompt_id: prompt_id
      message: tool_name
      tool_name: tool_name
      tool_target: tool_input.command
      tool_input: tool_input
    reply:
      allow: '{"decision":{"behavior":"allow"}}'
      deny: '{"decision":{"behavior":"deny","message":{reason_json}}}'
injected_prompts:
  - kind: task_notification
    needle: "<task-notification>"
hooks_injection:
  - set_env:
      name: STUB_HOOK
      value: "{crowbar_hook} hook any --segment {segid}"
runtime:
  transport: hooks
  hooks:
    format: json
session:
  resume: { arg: "--resume {id}" }
model:
  available: [sonnet, opus]
  strategy: restart_tui
  apply:
    - pass_arg: { arg: "--model", value: "{model}" }
effort:
  available:
    "*": [low, high]
  strategy: restart_tui
  apply:
    - pass_arg: { arg: "--effort", value: "{effort}" }
presentation:
  prompt_submit:
    strategy: restart_tui
    fresh:
      - pass_arg: { positional: "--" }
      - pass_arg: { positional: "{message}" }
    resume:
      - pass_arg: { positional: "--" }
      - pass_arg: { positional: "{message}" }
    steer:
      skip_prefixes: ["/"]
      frame: "STEERED: {message}"
      reply: '{"decision":"block","reason":{message_json}}'
`
}

/** Writes the stub provider into the isolated home's user-descriptor directory. */
export function installStubProvider(home, runDir) {
  const binDir = join(runDir, 'bin')
  mkdirSync(binDir, { recursive: true })
  const bin = join(binDir, STUB_BIN_NAME)
  symlinkSync('/bin/cat', bin)
  const dir = join(home, 'descriptors')
  mkdirSync(dir, { recursive: true })
  writeFileSync(join(dir, `${STUB_PROVIDER_ID}.yaml`), descriptor(bin))
  return bin
}

/** How many stub-provider processes this run currently has alive, counted from the OS process table. */
export function liveStubProcesses(runDir) {
  const out = Bun.spawnSync(['ps', '-axo', 'command='], {
    stdout: 'pipe',
  }).stdout.toString()
  const needle = join(runDir, 'bin', STUB_BIN_NAME)
  return out.split('\n').filter((line) => line.startsWith(needle)).length
}
