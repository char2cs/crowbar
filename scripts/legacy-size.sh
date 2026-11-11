#!/bin/sh
# legacy-size.sh [home]: how much chat history an upgrade's chat-ledger step will
# convert, read-only. Prints chats, legacy records per table and the bytes of the
# legacy files. Opens each database with mode=ro and writes nothing.
set -eu
HOME_DIR=${1:-${CROWBAR_HOME:-$HOME/.crowbar}}
[ -d "$HOME_DIR" ] || { echo "no Crowbar home at $HOME_DIR" >&2; exit 1; }
exec python3 -I - "$HOME_DIR" <<'EOF'
import os, sqlite3, sys, urllib.parse

home = sys.argv[1]
store = os.path.join(home, "state", "store")
# The tables and journals the chat-ledger step's census counts, as it counts them.
tables = {
    "agent_runner.db": ["agent_chat_conversations"],
    "agent_activity.db": ["agent_turns", "agent_tool_calls", "agent_subagents",
                          "agent_interruptions", "agent_choices"],
}
chats, records, size = set(), {}, 0
for db, names in tables.items():
    path = os.path.join(store, db)
    if not os.path.isfile(path):
        continue
    for suffix in ("", "-wal", "-shm"):
        if os.path.isfile(path + suffix):
            size += os.path.getsize(path + suffix)
    con = sqlite3.connect("file:" + urllib.parse.quote(path) + "?mode=ro", uri=True)
    have = {r[0] for r in con.execute("SELECT name FROM sqlite_master WHERE type = 'table'")}
    for name in names:
        if name in have:
            rows = con.execute(f"SELECT chat_id, count(*) FROM {name} WHERE chat_id != '' GROUP BY chat_id")
            for chat, n in rows:
                chats.add(chat)
                records[name] = records.get(name, 0) + n
    con.close()
journals = os.path.join(home, "chats")
if os.path.isdir(journals):
    for entry in os.scandir(journals):
        prompts = os.path.join(entry.path, "prompt-requests")
        if not entry.is_dir(follow_symlinks=False) or not os.path.isdir(prompts):
            continue
        for f in os.scandir(prompts):
            if f.name.endswith(".json") and f.is_file(follow_symlinks=False):
                chats.add(entry.name)
                records["prompt journals"] = records.get("prompt journals", 0) + 1
                size += f.stat().st_size
print(f"home            {home}")
print(f"chats           {len(chats)}")
for name, n in records.items():
    print(f"  {name:<26} {n}")
print(f"legacy records  {sum(records.values())}")
print(f"legacy bytes    {size} ({size / 2**20:.1f} MiB)")
EOF
