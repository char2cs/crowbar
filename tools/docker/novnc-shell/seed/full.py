#!/usr/bin/env python3
"""Builds the `full` seed profile on top of the baseline, through the daemon's HTTP API, git and
fixture files only. Run by seed/build.sh with the daemon up and SCRIPTED_SPEED=0. See README.md."""
import json
import os
import subprocess
import sys
import time
import uuid

sys.path.insert(0, "/opt/rig/bin")
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import files  # noqa: E402
from rigapi import Api, ApiError, hold, ledger  # noqa: E402
from rigops import Chats  # noqa: E402

HOME = os.environ["CROWBAR_HOME"]
FIXTURES = os.path.expanduser("~/fixtures")
api = Api()
chats = Chats(api)
report = []


def note(line):
    report.append(line)
    print(line, flush=True)


def wait_list(what, path, pick, timeout=60):
    return api.until(what, lambda: next((x for x in (api.get(path) or []) if pick(x)), None), timeout)


class Repo:
    def __init__(self, project_id, repo_id):
        self.pid, self.rid = project_id, repo_id
        self.prefix = f"/v0/projects/{project_id}/repos/{repo_id}"

    def workspaces(self):
        return api.get(self.prefix + "/workspaces")

    def ws(self, branch, default=False):
        return next(w for w in self.workspaces() if w.get("branch") == branch and bool(w.get("isDefault")) == default)

    def fork(self, parent_chat, branch=None, title=None):
        body = {"provider": "scripted", "parentId": parent_chat, "ownWorktree": True}
        if branch:
            body["branch"] = branch
        got = api.post(self.prefix + "/chats", body)
        if title:
            api.post(f"{self.prefix}/chats/{got['id']}/rename", {"title": title})
        ws = api.until(f"worktree for {branch}", lambda: next(
            (w for w in self.workspaces() if w["id"] == got["workspaceId"] and w.get("localPath")), None))
        return got["id"], ws

    def chat(self, workspace_id, title=None, parent=None, provider="scripted"):
        return chats.create(self.prefix, workspace_id, parent, title, provider)["id"]

    def folder(self, name, parent=None):
        body = {"name": name}
        if parent:
            body["parentId"] = parent
        return api.post(self.prefix + "/chats/folders", body)["folder"]["id"]

    def place(self, chat_id, parent_id):
        api.patch(f"{self.prefix}/chats/{chat_id}/placement", {"parentId": parent_id})


def add_project(name, path):
    os.makedirs(path, exist_ok=True)
    api.post("/v0/projects", {"name": name, "path": path})
    return wait_list(f"project {name}", "/v0/projects", lambda p: p["name"] == name)["id"]


def add_repo(project_id, name, path):
    api.post(f"/v0/projects/{project_id}/repos", {"name": name, "path": path})
    return wait_list(f"repo {name}", f"/v0/projects/{project_id}/repos", lambda r: r["name"] == name)["id"]


def play(chat_id, *scenarios, prefix=None):
    for s in scenarios:
        rows = chats.send(chat_id, s if s.startswith(("#", "/")) or " " in s else f"#{s}", prefix=prefix, timeout=120)
        if not any(r["kind"] in ("turn_ended", "turn_failed", "ask", "runner_exited", "prompt_settled") for r in rows):
            raise RuntimeError(f"{s} on {chat_id} did not settle: {[r['kind'] for r in rows]}")


def answer(chat_id, option, prefix, reason=None, content=None):
    chats.answer(chat_id, [option], reason=reason, content=content, prefix=prefix)


def providers():
    """The rig's two scripted providers first, then the shipped ones."""
    ids = [p["id"] for p in api.get("/v0/settings/chat/providers")]
    order = ["scripted", "scripted-lite"] + [i for i in ids if i not in ("scripted", "scripted-lite")]
    api.put("/v0/settings/chat/providers", {"providers": [{"id": i, "disabled": False, "mcpDisabled": False} for i in order]})
    note(f"  providers  {', '.join(order)}")


def main():
    providers()
    project = next(p for p in api.get("/v0/projects") if p["name"] == "Crowbar Seed")
    rid = next(r for r in api.get(f"/v0/projects/{project['id']}/repos") if r["name"] == "checkout")["id"]
    seed = Repo(project["id"], rid)
    checkout = f"{HOME}/seed/checkout"
    base_chat = seed.ws("main")["owningChatId"]
    feature = seed.ws("feature/pricing-rounding")

    # --- a remote with protected, imported and new branches -------------------------------------
    # "gitlab" in the remote's path is what makes the daemon ask the forge CLI (forge/glab) for PRs.
    remote = f"{FIXTURES}/remote/gitlab-fixture/checkout.git"
    if not os.path.isdir(remote):
        files.git(FIXTURES, "clone", "-q", "--bare", checkout, remote)
        files.git(checkout, "remote", "add", "origin", remote)
        files.git(checkout, "push", "-q", "origin", "main", "feature/pricing-rounding")
        tmp = f"{FIXTURES}/remote/work"
        files.git(FIXTURES, "clone", "-q", remote, tmp)
        for branch in ("remote-only/new-feature", "release/1.0"):
            files.git(tmp, "checkout", "-q", "-b", branch, "origin/main")
            files.write(f"{tmp}/{branch.replace('/', '-')}.md", f"# {branch}\n")
            files.commit(tmp, f"Work on {branch}")
            files.git(tmp, "push", "-q", "origin", branch)
        subprocess.run(["rm", "-rf", tmp], check=True)
        files.git(checkout, "fetch", "-q", "origin")
    note("  remote     origin with main, feature/pricing-rounding, remote-only/new-feature, release/1.0")

    # --- main grows the files the review branch changes, then the review branch -----------------
    files.review_base(checkout)
    files.git(checkout, "push", "-q", "origin", "main")  # a fork starts from origin/main
    review_chat, review_ws = seed.fork(base_chat, "feature/review-everything", "Review everything")
    files.checkout_kinds(review_ws["localPath"], f"{HOME}/state")
    files.commit(review_ws["localPath"], "Checkout fixture: every kind of file")
    files.review_branch(review_ws["localPath"])
    note(f"  workspace  feature/review-everything ({review_ws['id']}): file kinds + branch diff shapes")

    # --- uncommitted changes of every kind in the seeded feature workspace ----------------------
    files.uncommitted(feature["localPath"])
    note("  changes    feature/pricing-rounding: modified, staged, deleted, renamed, untracked")

    # --- workspaces in every state the API reaches --------------------------------------------
    locked_chat, locked_ws = seed.fork(base_chat, "ws/locked", "Locked workspace")
    api.post(f"{seed.prefix}/chats/{locked_chat}/lock", {"locked": True})
    generated_chat, generated_ws = seed.fork(base_chat, None, "Generated branch name")
    big_chat, big_ws = seed.fork(base_chat, "feature/over-999-lines", "Over 999 changed lines")
    files.many_lines(big_ws["localPath"])
    broken_chat, broken_ws = seed.fork(base_chat, "ws/git-status-fails", "Git status fails")
    files.write(f"{broken_ws['localPath']}/.git", "gitdir: /nonexistent/rig/broken-worktree\n")
    conflict_chat, conflict_ws = seed.fork(base_chat, "feature/conflicts", "Conflicts with main")
    files.write(f"{conflict_ws['localPath']}/README.md", "# Conflicting title on the branch\n")
    files.commit(conflict_ws["localPath"], "Retitle the README on the branch")
    merged_chat, merged_ws = seed.fork(base_chat, "feature/already-merged", "Already merged")
    files.write(f"{merged_ws['localPath']}/MERGED.md", "merged into main\n")
    files.commit(merged_ws["localPath"], "A change main already has")
    files.git(checkout, "-c", "user.name=Crowbar Seed", "-c", "user.email=seed@crowbar.local",
              "merge", "-q", "--no-ff", "-m", "Merge feature/already-merged", "feature/already-merged")
    files.write(f"{checkout}/README.md", "# Conflicting title on main\n")
    files.commit(checkout, "Retitle the README on main")
    files.git(checkout, "push", "-q", "origin", "main")
    resolved_chat, resolved_ws = seed.fork(base_chat, "feature/threads-resolved", "Threads resolved")
    files.write(f"{resolved_ws['localPath']}/src/pricing.ts", "export const roundPrice = (c: number) => Math.round(c)\n")
    files.commit(resolved_ws["localPath"], "Simplify roundPrice")
    files.git(checkout, "branch", "topic/no-parent")
    try:
        files.git(checkout, "push", "-q", "origin", "topic/no-parent")
        orphan = api.post(seed.prefix + "/chats", {"provider": "scripted", "import": {"branch": "topic/no-parent"}})
        api.post(f"{seed.prefix}/chats/{orphan['id']}/rename", {"title": "Adopted local branch"})
    except ApiError as err:
        note(f"  skipped    adopt topic/no-parent: {err}")
    # A batch import cannot check out a branch another worktree holds: it leaves a placeholder.
    files.git(checkout, "branch", "held/foreign")
    files.git(checkout, "push", "-q", "origin", "held/foreign")
    files.git(checkout, "worktree", "add", "-q", f"{FIXTURES}/foreign-worktree", "held/foreign")
    try:
        api.post(seed.prefix + "/chats/import-batch",
                 {"branches": [{"name": "held/foreign", "chatId": str(uuid.uuid4()), "lock": False}]})
        api.until("held/foreign placeholder", lambda: any(w.get("branch") == "held/foreign" for w in seed.workspaces()))
    except (ApiError, TimeoutError) as err:
        note(f"  skipped    import held/foreign: {err}")
    cascade_chat, cascade_ws = seed.fork(base_chat, "feature/delete-cascades", "Delete cascades")
    for i in (1, 2, 3):
        seed.chat(cascade_ws["id"], f"Sibling chat {i}")
    note("  workspaces locked, generated chat-*, >999 lines, git status fails, conflicts, merged, "
         "threads resolved, adopted (no parent), held by a foreign worktree (placeholder), delete cascades")

    # --- pull-request states, read from the forge fixture while a client holds forge: scopes ---
    mrs, pr_ws = [], []
    for iid, (branch, state) in enumerate([("feature/pr-open", "opened"), ("feature/pr-merged", "merged"),
                                           ("feature/pr-closed", "closed")], start=101):
        _, ws = seed.fork(base_chat, branch, f"PR {state}")
        files.write(f"{ws['localPath']}/{branch.split('/')[1]}.md", f"# {branch}\n")
        files.commit(ws["localPath"], f"Work for {branch}")
        files.git(ws["localPath"], "push", "-q", "origin", branch)
        mrs.append({"iid": iid, "state": state, "web_url": f"http://127.0.0.1:8088/mr/{iid}", "title": f"Fixture MR: {branch}",
                    "source_branch": branch, "target_branch": "main", "sha": files.git(ws["localPath"], "rev-parse", "HEAD")})
        pr_ws.append(ws["id"])
    files.write(f"{FIXTURES}/forge/merge-requests.json", json.dumps({"protected": ["main", "release/1.0"], "merge_requests": mrs}, indent=1))
    hold(api.sock, [f"forge:{w}" for w in pr_ws], 8)
    states = {w["branch"]: w.get("status") for w in seed.workspaces() if w["id"] in pr_ws}
    note(f"  forge      merge requests via forge/glab: {states}")

    # --- review threads -------------------------------------------------------------------------
    identity = api.get(f"/v0/chats/{review_chat}/identity") or {}
    me = identity.get("displayName") or identity.get("login") or "Crowbar Seed"
    threads = f"{seed.prefix}/workspaces/{review_ws['id']}/threads"

    def thread(path, line, body, author=me, agent=False):
        return api.post(threads, {"filePath": path, "line": line, "startLine": line, "endLine": line, "side": "right",
                                  "author": author, "isAgent": agent, "body": body})
    mine = thread("src/long-module.ts", 11, "Why was this comment changed? (thread by the current identity)")
    replied = thread("src/modern-name.ts", 1, "The rename should keep the old export as an alias.", author="Reviewer Two")
    api.post(f"{threads}/{replied['id']}/replies", {"author": me, "isAgent": False, "body": "Added the alias in the next commit."})
    thread("dist/bundle.min.js", 1, "Should a minified bundle be committed at all?", author="Lint Bot", agent=True)
    outdated = thread("notes/needles.txt", 3, "This line changes in the next commit, so this thread goes outdated.")
    lines = open(f"{review_ws['localPath']}/notes/needles.txt").read().splitlines(keepends=True)
    lines[2] = "needle 2: moved on\n"
    files.write(f"{review_ws['localPath']}/notes/needles.txt", "".join(lines))
    files.commit(review_ws["localPath"], "Change the line an open thread is on")
    rthreads = f"{seed.prefix}/workspaces/{resolved_ws['id']}/threads"
    done = api.post(rthreads, {"filePath": "src/pricing.ts", "line": 1, "startLine": 1, "endLine": 1, "side": "right",
                               "author": me, "isAgent": False, "body": "Resolved finding."})
    api.patch(f"{rthreads}/{done['id']}", {"isResolved": True})
    agent_chat = seed.chat(review_ws["id"], "Agent review")
    play(agent_chat, "#mcp-review", prefix=seed.prefix)
    note(f"  threads    by {me}, with a reply, agent-authored (isAgent), outdated, resolved, posted over MCP (chatId)"
         f" [{mine['id'][:8]} {outdated['id'][:8]}]")

    # --- chat ledgers -------------------------------------------------------------------------
    fws = feature["id"]
    tools = seed.chat(fws, "Ledger: tools of every kind and status")
    play(tools, "#tools", "#read-run", "#edits", "#multi", prefix=seed.prefix)
    asks = seed.chat(fws, "Ledger: asks resolved every way")
    for scen, option, extra in [("#ask-execute", "allow", {}), ("#ask-edit", "suggestion-0", {}),
                                ("#ask-mcp", "deny", {"reason": "Not on the docs server, please."}),
                                ("#question", "questions", {}), ("#question-multi", "questions", {}),
                                ("#questions-three", "questions", {}),
                                ("#elicit", "accept", {"content": {"title": "Rig page", "priority": 2, "audience": "internal", "notify": True}})]:
        play(asks, scen, prefix=seed.prefix)
        pending = Chats.pending(asks, ledger(api.sock, asks))
        if not pending:
            note(f"  skipped    {scen}: no ask opened")
            continue
        ask = pending[-1]
        if option == "questions":
            # One pick per question; a multi-select question gets two.
            picks = [o["id"] for q in ask.get("questions", []) for o in q["options"][:2 if q.get("multi") else 1]]
        else:
            ids = [o["id"] for o in ask.get("options", [])]
            picks = [option if option in ids or not ids else ids[0]]
        chats.answer(asks, picks, reason=extra.get("reason"), content=extra.get("content"), prefix=seed.prefix)
    play(asks, "#ask-unknown", prefix=seed.prefix)  # left pending: the restart settles it unanswered
    markers = seed.chat(fws, "Ledger: markers")
    play(markers, "#fail-auth", "#fail-limit", "#fail-overloaded", "#fail-other", "#fail-nokind", "#compact", prefix=seed.prefix)
    api.patch(f"{seed.prefix}/chats/{markers}/selection", {"model": "scripted-small", "effort": "low"})
    play(markers, "After a model and effort change.", prefix=seed.prefix)
    try:
        api.post(f"{seed.prefix}/chats/{markers}/switch", {"provider": "scripted-lite"})
        play(markers, "After a provider switch.", prefix=seed.prefix)
        api.post(f"{seed.prefix}/chats/{markers}/switch", {"provider": "scripted"})
    except ApiError as err:
        note(f"  skipped    provider switch: {err}")
    play(markers, "#lose-session", "After the session was lost.", "#exit", prefix=seed.prefix)
    chats.send(markers, "#working", prefix=seed.prefix, wait=False)
    api.until("a running turn", lambda: any(r["kind"] == "turn_started" for r in ledger(api.sock, markers)[-3:]), 30)
    chats.stop(markers, prefix=seed.prefix)
    notices = seed.chat(fws, "Ledger: notices and telemetry")
    play(notices, "#warning", "#telemetry-40", "#telemetry-90", "#notification", prefix=seed.prefix)
    agents = seed.chat(fws, "Ledger: subagents, shells, plan")
    play(agents, "#subagents", "#shells", "#plan", prefix=seed.prefix)
    rich = seed.chat(fws, "Ledger: rich content")
    play(rich, "#markdown", "#mermaid-open", "#long-reply", prefix=seed.prefix)
    play(rich, "A long user message that goes on well past sixty characters so the bubble wraps and truncates in lists.",
         "A multi-line message:\nline two\nline three", "A **markdown** user message with `code` and a [link](http://127.0.0.1:8088/).",
         "/review fast", prefix=seed.prefix)
    long_chat = seed.chat(fws, "Long chat: 120 turns")
    for i in range(1, 121):
        play(long_chat, f"Turn {i}: tell me something.", prefix=seed.prefix)
    seed.chat(fws)  # an untitled chat, never prompted
    nested = seed.chat(fws, "Nested thread", parent=rich)
    play(nested, "A reply inside a nested thread.", prefix=seed.prefix)
    plain = seed.chat(seed.ws("main", default=True)["id"], "Plain repo thread (no worktree)")
    play(plain, "Hello from the repo folder itself.", prefix=seed.prefix)
    on_main = seed.chat(seed.ws("main")["id"], "Chat on main (Make a branch)") if seed.ws("main").get("localPath") else None
    note("  chats      tools, asks (allowed/grant/denied/answered/form/pending), markers (failed x5, compaction, "
         "model/effort/provider change, session lost, runner exited, interrupted), notices + telemetry, "
         "subagents/shells/plan, rich content, 120-turn chat, untitled, nested thread, plain repo thread"
         + (", chat on main" if on_main else ""))

    # --- sidebar: folders and enough rows to overflow ------------------------------------------
    experiments = seed.folder("Experiments")
    archive = seed.folder("Archive", experiments)
    for i in range(1, 25):
        cid = seed.chat(fws, f"Row {i:02d}: overflow filler")
        if i <= 4:
            seed.place(cid, experiments)
        elif i <= 7:
            seed.place(cid, archive)
    note("  sidebar    folders Experiments/Archive in the repo, 24 filler chats")

    # --- a second repo, a second project with chats, a project with no repos --------------------
    second = f"{HOME}/seed/second-repo"
    if not os.path.isdir(f"{second}/.git"):
        os.makedirs(second)
        files.git(second, "init", "-q", "-b", "develop")
        files.write(f"{second}/README.md", "# Second repo\n")
        files.commit(second, "Second repo")
    add_repo(project["id"], "second-repo", second)
    home_prefix = f"/v0/projects/{project['id']}/home"
    home_ws = api.get(home_prefix)["id"]
    home_chat = chats.create(home_prefix, home_ws, title="Project home chat")["id"]
    play(home_chat, "Hello from the project home.", prefix=home_prefix)
    notes_folder = api.post(home_prefix + "/chats/folders", {"name": "Notes"})["folder"]["id"]
    for i in (1, 2):
        cid = chats.create(home_prefix, home_ws, title=f"Home note {i}")["id"]
        api.patch(f"{home_prefix}/chats/{cid}/placement", {"parentId": notes_folder})

    side_path = f"{HOME}/seed/side-project"
    side_repo_path = f"{side_path}/side-app"
    if not os.path.isdir(f"{side_repo_path}/.git"):
        os.makedirs(side_repo_path)
        files.git(side_repo_path, "init", "-q", "-b", "main")
        files.write(f"{side_repo_path}/README.md", "# Side app\n")
        files.commit(side_repo_path, "Side app")
    side = add_project("Side Project", side_path)
    side_rid = add_repo(side, "side-app", side_repo_path)
    side_repo = Repo(side, side_rid)
    side_ws = api.until("side-app home workspace", lambda: next((w for w in side_repo.workspaces() if w.get("isDefault")), None))
    for title in ("Side chat A", "Side chat B"):
        cid = side_repo.chat(side_ws["id"], title)
        play(cid, f"Hello from {title}.", prefix=side_repo.prefix)
    add_project("Empty Space", f"{HOME}/seed/empty-space")
    note("  projects   Crowbar Seed (+ second-repo, home chats, home folder Notes), Side Project (side-app, 2 chats), Empty Space (no repos)")

    descriptor = open("/opt/rig/agent/scripted.yaml").read().replace("id: scripted\n", "id: uploaded-agent\n", 1) \
        .replace("display_name: Scripted\n", "display_name: Uploaded Agent\n", 1)
    files.write(f"{FIXTURES}/settings/agent-descriptor.yaml", descriptor)
    note(f"  fixtures   {FIXTURES}: drop/, clipboard/, import/ (spare-repo, icon.png), settings/, remote/, foreign-worktree")
    with open(os.path.expanduser("~/.rig/seed-full.txt"), "w") as f:
        f.write("\n".join(report) + "\n")


if __name__ == "__main__":
    t0 = time.time()
    main()
    print(f"full profile built in {time.time() - t0:.0f}s")
