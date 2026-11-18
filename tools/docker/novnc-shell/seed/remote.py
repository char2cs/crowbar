#!/usr/bin/env python3
"""Seeds the rig's second daemon, the one `rig remote` runs as a paired host: the scripted
descriptors, a project with one git repo, and a chat in it. Run with CROWBAR_HOME pointing at
that daemon's home, which picks its socket."""
import glob
import os
import sys

here = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, here)
sys.path.insert(0, "/opt/rig/bin")
import files  # noqa: E402
from rigapi import Api  # noqa: E402
from rigops import Chats  # noqa: E402


def main():
    api = Api()
    api.healthy(30)
    for d in sorted(glob.glob("/opt/rig/agent/*.yaml")):
        with open(d, "rb") as f:
            api.post("/v0/settings/chat/descriptors", f.read())
    root = os.path.expanduser("~/remote-host/remote-app")
    if not os.path.isdir(f"{root}/.git"):
        os.makedirs(root, exist_ok=True)
        files.git(root, "init", "-q", "-b", "main")
        files.write(f"{root}/README.md", "# Remote app\n\nA repo that lives on the paired host.\n")
        files.write(f"{root}/src/main.ts", "export const where = 'remote';\n")
        files.commit(root, "Initial commit")
    api.post("/v0/projects", {"name": "Remote Project", "path": os.path.dirname(root)})
    project = api.until("the project", lambda: next((p for p in api.get("/v0/projects") or [] if p["name"] == "Remote Project"), None), 30)
    prefix = f"/v0/projects/{project['id']}/repos"
    api.post(prefix, {"name": "remote-app", "path": root})
    repo = api.until("the repo", lambda: next(iter(api.get(prefix) or []), None), 30)
    ws = api.until("the home workspace", lambda: next((w for w in api.get(f"{prefix}/{repo['id']}/workspaces") or []
                                                       if w.get("isDefault")), None), 30)
    Chats(api).create(f"{prefix}/{repo['id']}", ws["id"], title="Chat on the remote host")
    print("remote seeded: project Remote Project, repo remote-app, one chat")


if __name__ == "__main__":
    main()
