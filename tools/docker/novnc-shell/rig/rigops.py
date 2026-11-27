#!/usr/bin/env python3
"""Chat-level operations over the daemon API, for `rig agent` and the seed profiles. Stdlib only.

    rigops.py scenarios                              the scripted provider's scenarios
    rigops.py chats                                  every chat: id, project, repo, workspace, title
    rigops.py new [--home] [--project NAME] [--workspace <ws-id>] [--title T] [--parent <chat-id>] [--provider P]
    rigops.py play <chat-id> <scenario> [args...]    sends "#<scenario> args" and waits for the turn
    rigops.py say <chat-id> <text>                   sends a plain prompt (the "reply" scenario)
    rigops.py asks <chat-id>                         the chat's pending asks and their options
    rigops.py answer <chat-id> <option-id> [ask-id]  answers the newest pending ask (or ask-id)
    rigops.py stop <chat-id>
"""
import json
import os
import sys
import time
import uuid

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from rigapi import Api, ApiError, ledger  # noqa: E402

SCENARIOS = os.environ.get("SCRIPTED_SCENARIOS", "/opt/rig/agent/scenarios")
PROVIDER = os.environ.get("RIG_PROVIDER", "scripted")


class Chats:
    def __init__(self, api=None):
        self.api = api or Api()

    # --- where a chat lives ----------------------------------------------------------------

    def all(self):
        """Every chat as (chat, scope prefix) where the prefix is its repo or home route."""
        out = []
        for p in self.api.get("/v0/projects") or []:
            home = f"/v0/projects/{p['id']}/home"
            try:
                for c in self.api.get(home + "/chats") or []:
                    out.append((dict(c, projectName=p["name"], repoName=""), home))
            except ApiError:
                pass
            for r in self.api.get(f"/v0/projects/{p['id']}/repos") or []:
                prefix = f"/v0/projects/{p['id']}/repos/{r['id']}"
                for c in self.api.get(prefix + "/chats") or []:
                    out.append((dict(c, projectName=p["name"], repoName=r["name"]), prefix))
        return out

    def prefix(self, chat_id):
        for c, prefix in self.all():
            if c["id"] == chat_id:
                return prefix
        raise SystemExit(f"rigops: no chat {chat_id}")

    # --- verbs -----------------------------------------------------------------------------

    def create(self, prefix, workspace_id=None, parent_id=None, title=None, provider=PROVIDER, **extra):
        body = {"provider": provider, "id": str(uuid.uuid4())}
        if workspace_id:
            body["workspaceId"] = workspace_id
        if parent_id:
            body["parentId"] = parent_id
        body.update(extra)
        got = self.api.post(prefix + "/chats", body)
        if title:
            self.api.post(f"{prefix}/chats/{got['id']}/rename", {"title": title})
        return got

    def send(self, chat_id, text, prefix=None, wait=True, timeout=60, **extra):
        prefix = prefix or self.prefix(chat_id)
        before = len(ledger(self.api.sock, chat_id))
        body = dict({"text": text, "clientRequestId": str(uuid.uuid4())}, **extra)
        self.api.post(f"{prefix}/chats/{chat_id}/prompts", body)
        if wait:
            return self.settle(chat_id, before, timeout)
        return None

    def settle(self, chat_id, since, timeout=60):
        """Waits until the turn the last prompt opened ends, or stops at an ask; returns the new rows."""
        deadline = time.time() + timeout
        while time.time() < deadline:
            rows = ledger(self.api.sock, chat_id)[since:]
            kinds = [r["kind"] for r in rows]
            settled = [r for r in rows if r["kind"] == "prompt_settled" and not r["body"].get("consumed", True)]
            if "turn_ended" in kinds or "turn_failed" in kinds or settled or self.pending(chat_id, rows):
                return rows
            if any(r["kind"] == "runner_exited" and r["body"].get("reason") not in ("displaced",) for r in rows):
                return rows
            time.sleep(0.2)
        raise TimeoutError(f"rigops: chat {chat_id} did not settle in {timeout}s")

    @staticmethod
    def pending(chat_id, rows):
        resolved = {r["body"]["askId"] for r in rows if r["kind"] == "ask_resolved"}
        return [r["body"] for r in rows if r["kind"] == "ask" and r["body"]["askId"] not in resolved]

    def answer(self, chat_id, option_ids, ask_id=None, reason=None, content=None, prefix=None, wait=True):
        prefix = prefix or self.prefix(chat_id)
        rows = ledger(self.api.sock, chat_id)
        if not ask_id:
            open_asks = self.pending(chat_id, rows)
            if not open_asks:
                raise SystemExit(f"rigops: chat {chat_id} has no pending ask")
            ask_id = open_asks[-1]["askId"]
        body = {"optionIds": option_ids}
        if reason:
            body["reason"] = reason
        if content is not None:
            body["content"] = content
        self.api.post(f"{prefix}/chats/{chat_id}/choices/{ask_id}/answer", body)
        if wait:
            return self.settle(chat_id, len(rows))
        return None

    def stop(self, chat_id, prefix=None):
        prefix = prefix or self.prefix(chat_id)
        self.api.post(f"{prefix}/chats/{chat_id}/stop")


def scenarios():
    for name in sorted(os.listdir(SCENARIOS)):
        if name.endswith(".json"):
            with open(os.path.join(SCENARIOS, name)) as f:
                yield name[:-5], json.load(f).get("description", "")


def summary(rows):
    for r in rows:
        b = r.get("body", {})
        detail = b.get("text") or b.get("name") or b.get("status") or b.get("kind") or b.get("reason") or ""
        print(f"{r['seq']:>5} {r['kind']:<16} {str(detail)[:90]!r}")


def main(argv):
    if not argv:
        print(__doc__.strip())
        return 2
    cmd, args = argv[0], argv[1:]
    if cmd == "scenarios":
        for name, desc in scenarios():
            print(f"#{name:<18} {desc}")
        return 0
    chats = Chats()
    if cmd == "chats":
        for c, _ in chats.all():
            print(f"{c['id']}  {c['projectName']}/{c['repoName'] or '(home)'}  ws {c.get('workspaceId', '')[:8]}  "
                  f"{c.get('type')}  {c.get('title') or '(untitled)'}")
    elif cmd == "new":
        opts = {"--project": "Crowbar Seed", "--workspace": None, "--title": None, "--parent": None, "--provider": PROVIDER}
        home = "--home" in args
        rest = [a for a in args if a != "--home"]
        for i in range(0, len(rest) - 1, 2):
            opts[rest[i]] = rest[i + 1]
        projects = chats.api.get("/v0/projects") or []
        named = [x for x in projects if x["name"] == opts["--project"]]
        with_repos = [x for x in projects if chats.api.get(f"/v0/projects/{x['id']}/repos")]
        project = (named or with_repos or projects or [None])[0]
        if not project:
            raise SystemExit("rigops: no project (rig seed baseline or full)")
        p = project["id"]
        if home:
            prefix, ws = f"/v0/projects/{p}/home", chats.api.get(f"/v0/projects/{p}/home")["id"]
        else:
            repos = chats.api.get(f"/v0/projects/{p}/repos")
            if not repos:
                raise SystemExit(f"rigops: project {project['name']} has no repo (use --home)")
            prefix = f"/v0/projects/{p}/repos/{repos[0]['id']}"
            spaces = chats.api.get(prefix + "/workspaces")
            ws = opts["--workspace"] or next((w["id"] for w in spaces if w.get("branch") == "feature/pricing-rounding"),
                                             next(w["id"] for w in spaces if w.get("isDefault")))
        print(chats.create(prefix, ws, opts["--parent"], opts["--title"], opts["--provider"])["id"])
    elif cmd in ("play", "say"):
        text = f"#{args[1]} {' '.join(args[2:])}".strip() if cmd == "play" else " ".join(args[1:])
        summary(chats.send(args[0], text, timeout=int(os.environ.get("RIG_TIMEOUT", "60"))))
    elif cmd == "asks":
        for a in chats.pending(args[0], ledger(chats.api.sock, args[0])):
            print(a["askId"], a.get("kind"), a.get("title"), [o["id"] for o in a.get("options", [])])
    elif cmd == "answer":
        summary(chats.answer(args[0], args[1].split(","), args[2] if len(args) > 2 else None))
    elif cmd == "stop":
        chats.stop(args[0])
    else:
        print(__doc__.strip())
        return 2
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
