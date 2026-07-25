# Crowbar

Crowbar is a local-first desktop app for running AI coding agents against real repositories — and reviewing what they produce before it lands.

Import a repo and every branch gets its own managed git worktree. Start a chat, and an agent goes to work in that worktree while you watch it in a real terminal. When it's done, you read the diff, leave comments, and decide. Your working tree is never touched.

It all runs on your machine: a Go daemon, a Tauri shell, and a React frontend.

---

## How it works

**Workspaces.** Every protected branch gets its own locked worktree, provisioned when the repo is imported. Agents only ever work inside a worktree — so two agents can't tread on each other, and neither can touch your checkout.

**Agent chats.** Pick a provider and Crowbar runs its CLI in a PTY inside the workspace. Claude Code and Codex are both supported, and you can switch between them mid-conversation without losing the thread.

**Review.** Every branch has a review: the diff, inline comment threads, and PR status. Human review is the point of the tool, not a step bolted onto the end of it.

**The rest of an IDE, where you need it.** Editor, integrated terminal, file explorer, search, LSP.

---

## Getting started

Requires Go, [Bun](https://bun.sh), and Rust.

```sh
make dev           # daemon + web + desktop, with hot reload
make dev-desktop   # just the Tauri app
make test          # api + web + desktop
make pr-checks     # lint, coverage, build — what CI runs
```

Dev state is isolated by design: every `dev` target roots Crowbar at `<repo>/.crowbar` instead of `~/.crowbar`, so a dev instance never collides with an installed one.

---

## Architecture

```
api/       Go daemon — domain, engines (agent, git, terminal, LSP, search), HTTP + WebSocket API
web/       React frontend
desktop/   Tauri shell
docker/    Self-hosted image: the same daemon, serving a prebuilt web bundle
```

The daemon is the product; the desktop app is a shell around it. That's why `make docker-up` can serve the identical thing at `localhost:3737` with no desktop app involved.

---

## Where it's going

Crowbar's thesis is that a review loop should get shorter the more you use it. The plan is a per-project memory: an agent reads the comments you leave on real reviews, extracts the principles behind them, and feeds those back into future agent runs — so the reviewer starts catching what you catch, in your voice, and you review less over time.

That part isn't built yet. What's above is.

---

## Distribution

Crowbar is distributed through [Quiver](https://github.com/rabbytesoftware/quiver.desktop), a multi-platform package manager by the same author.

---

## Naming

Crowbar follows the Valve-universe naming schema used across the rabbytesoftware ecosystem. The crowbar is Gordon Freeman's tool — simple, brutally effective, works in any environment, and the first thing you reach for. That's the right metaphor for a development tool.

---

## License

Crowbar is dual-licensed: **[AGPL-3.0](LICENSE)** or a **commercial license**.

Use it, modify it, self-host it — freely, under the AGPL. The catch is AGPL § 13: if you
modify Crowbar and let others interact with it over a network, you have to publish your
changes. Running it on your own machine for your own work triggers nothing.

If that doesn't work for you, a commercial license removes the copyleft obligations.
See [LICENSING.md](LICENSING.md).

Third-party components keep their own licenses; see [NOTICE.md](NOTICE.md).
