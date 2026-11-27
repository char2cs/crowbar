#!/usr/bin/env python3
"""Builds the `extras` seed profile on top of the baseline: the few things audit rows ask for that
the baseline lacks, without the full profile's weight. Run by seed/build.sh with the daemon up and
SCRIPTED_SPEED=0. See README.md, "Seed profiles"."""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import files  # noqa: E402
from full import HOME, Repo, add_project, add_repo, api, note, play  # noqa: E402
from rigapi import ApiError  # noqa: E402

README = """# Pricing

How the checkout prices a cart.

| Function | Input | Output | Notes |
| :------- | :---- | -----: | :---- |
| `roundPrice` | cents | cents | rounds half up |
| `applyTax` | cents, rate | cents | added on this branch |
| `cartTotal` | items | cents | 0 for an empty cart |

```ts
import { roundPrice } from './src/pricing'
console.log(roundPrice(1999.5))
```

```mermaid
flowchart LR
  Cart --> cartTotal --> applyTax --> roundPrice
```
"""


def main():
    project = next(p for p in api.get("/v0/projects") if p["name"] == "Crowbar Seed")
    rid = next(r for r in api.get(f"/v0/projects/{project['id']}/repos") if r["name"] == "checkout")["id"]
    seed = Repo(project["id"], rid)
    feature = seed.ws("feature/pricing-rounding")
    wt = feature["localPath"]

    # A README with a table, a fenced block and a mermaid fence; a dotenv pair (the .env ignored).
    files.write(f"{wt}/README.md", README)
    files.write(f"{wt}/.gitignore", ".env\n")
    files.write(f"{wt}/.env.example", "API_TOKEN=\nDEBUG=0\n")
    files.commit(wt, "README table, .env.example")
    files.write(f"{wt}/.env", "API_TOKEN=rig-not-a-secret\nDEBUG=1\n")
    note("  files      feature/pricing-rounding: README.md (table, ts fence, mermaid), .env (ignored), .env.example")

    # A chat whose reply has a table, fenced code and a mermaid diagram, in the feature workspace.
    rich = seed.chat(feature["id"], "Rich markdown: table, code, mermaid")
    play(rich, "#markdown", prefix=seed.prefix)
    note(f"  chat       Rich markdown: table, code, mermaid ({rich})")

    # A read-only workspace: a locked worktree, whose file writes the daemon refuses (409 locked).
    base_chat = seed.ws("main")["owningChatId"]
    locked_chat, locked_ws = seed.fork(base_chat, "ws/read-only", "Read-only workspace")
    api.post(f"{seed.prefix}/chats/{locked_chat}/lock", {"locked": True})
    try:
        api.post(f"/v0/chats/{locked_chat}/files", {"path": "probe.txt", "type": "file"})
        raise SystemExit("extras: the locked workspace accepted a write")
    except ApiError as err:
        if err.status != 409:
            raise
    note(f"  workspace  ws/read-only ({locked_ws['id']}): locked, writes refused 409")

    # A second project with a repo and two chats: the project switcher and carousel need two.
    side_path = f"{HOME}/seed/side-project"
    side_repo_path = f"{side_path}/side-app"
    if not os.path.isdir(f"{side_repo_path}/.git"):
        os.makedirs(side_repo_path)
        files.git(side_repo_path, "init", "-q", "-b", "main")
        files.write(f"{side_repo_path}/README.md", "# Side app\n")
        files.commit(side_repo_path, "Side app")
    side = add_project("Side Project", side_path)
    side_repo = Repo(side, add_repo(side, "side-app", side_repo_path))
    side_ws = api.until("side-app home workspace", lambda: next((w for w in side_repo.workspaces() if w.get("isDefault")), None))
    for title in ("Side chat A", "Side chat B"):
        play(side_repo.chat(side_ws["id"], title), f"Hello from {title}.", prefix=side_repo.prefix)
    note("  projects   Crowbar Seed, Side Project (side-app, 2 chats)")
    with open(os.path.expanduser("~/.rig/seed-extras.txt"), "w") as f:
        f.write(f"rich_chat={rich}\nread_only_ws={locked_ws['id']}\nfeature_ws={feature['id']}\n")


if __name__ == "__main__":
    main()
