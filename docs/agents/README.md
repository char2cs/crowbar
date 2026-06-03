# Implementation Agents — Execution Order

All agents work in the same worktree: `/Users/char2cs/Projects/Rabbyte/crowbar/api`

---

## Execution Waves

Agents within a wave can run **in parallel**. Each wave must complete before the next starts.

```
Wave 1 ─── Agent 01  (scaffold cleanup — fixes module paths all others depend on)

Wave 2 ─┬─ Agent 02  (core utilities)
         └─ Agent 03  (domain entities)

Wave 3 ─┬─ Agent 04  (flow engine)       needs 02 + 03
         ├─ Agent 05  (adapter)           needs 02 + 03
         └─ Agent 06  (app hub)           needs 03

Wave 4 ─┬─ Agent 07  (GORM repos)        needs 03 + 05 + 06
         ├─ Agent 08  (Asynx repos)       needs 03 + 04 + 05
         ├─ Agent 09  (chat WebSocket)    needs 03 + 04
         └─ Agent 10  (PTY terminal)      needs 03

Wave 5 ─┬─ Agent 11  (REST handlers)     needs 07 + 08 + 04
         ├─ Agent 12  (WS broadcaster)   needs 06 + 03
         └─ Agent 13  (MCP server)        needs 07 + 08 + 09 + 04

Wave 6 ─── Agent 14  (wiring)            needs all above

Wave 7 ─── Agent 15  (integration tests) needs 14
```

---

## Files

| Agent | File | What it builds |
|-------|------|----------------|
| 01 | [agent-01-scaffold-cleanup.md](agent-01-scaffold-cleanup.md) | Fix module paths, delete dead files, bump Go version |
| 02 | [agent-02-core-utilities.md](agent-02-core-utilities.md) | `core/metadata`, `core/config`, `core/paths` |
| 03 | [agent-03-domain-entities.md](agent-03-domain-entities.md) | All 7 domain entity types |
| 04 | [agent-04-flow-engine.md](agent-04-flow-engine.md) | YAML parser, validator, evaluator, builtin flow |
| 05 | [agent-05-adapter.md](agent-05-adapter.md) | GORM SQLite store + 4 Asynx event stores |
| 06 | [agent-06-app-hub.md](agent-06-app-hub.md) | Typed `WebSocketHub`, `Subscriber` interface |
| 07 | [agent-07-gorm-repos.md](agent-07-gorm-repos.md) | Project, Repository, ConversationMessage repos |
| 08 | [agent-08-asynx-repos.md](agent-08-asynx-repos.md) | Task, AgentRun, KanbanItem, ReviewThread repos |
| 09 | [agent-09-chat-ws.md](agent-09-chat-ws.md) | `ChatHub`, `ChatHandler`, `AgentRuntime` interface |
| 10 | [agent-10-terminal-pty.md](agent-10-terminal-pty.md) | PTY WebSocket handler |
| 11 | [agent-11-rest-handlers.md](agent-11-rest-handlers.md) | All REST HTTP handlers |
| 12 | [agent-12-ws-broadcaster.md](agent-12-ws-broadcaster.md) | `Broadcaster[T]`, WS fan-out, `dispatch()` helper |
| 13 | [agent-13-mcp-server.md](agent-13-mcp-server.md) | MCP server + all 8 tools + auth |
| 14 | [agent-14-wiring.md](agent-14-wiring.md) | All container files, router, `internal.go` |
| 15 | [agent-15-integration-tests.md](agent-15-integration-tests.md) | Full integration test suite (12 suites + kit) |

---

## Notes

- **Agent 14** is the densest file — it wires every layer together and is the most likely place for import cycle issues. If a cycle appears between `app` and `api/v0/chat`, move the `chat.Hub` interface to `internal/hub/chat/` (neutral package).
- **Agent 15** target is `go build -tags integration ./tests/...` — do not run tests, just compile + vet.
- **ACP SDK** (`github.com/coder/acp-go-sdk`) is not available. `AgentRuntime` is an interface; `AgentStub` in the test kit covers all integration tests.
