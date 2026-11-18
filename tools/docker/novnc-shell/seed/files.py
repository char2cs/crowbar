"""File and git fixtures for the seed profiles: every file kind a checkout can hold, the branch-diff
shapes, uncommitted changes, and the loose files the rig drops and imports. Stdlib only."""
import os
import struct
import subprocess
import zlib


def git(cwd, *args, check=True):
    return subprocess.run(["git", *args], cwd=cwd, check=check, capture_output=True, text=True).stdout.strip()


def commit(cwd, message, *extra):
    git(cwd, "add", "-A")
    git(cwd, "-c", "user.name=Crowbar Seed", "-c", "user.email=seed@crowbar.local", "commit", "-q", "-m", message, *extra)


def write(path, content, mode=None):
    os.makedirs(os.path.dirname(path) or ".", exist_ok=True)
    with open(path, "wb" if isinstance(content, bytes) else "w") as f:
        f.write(content)
    if mode is not None:
        os.chmod(path, mode)


def png(width, height, rgb):
    """A solid-colour PNG, written by hand so the image needs no imaging library."""
    def chunk(kind, data):
        return struct.pack(">I", len(data)) + kind + data + struct.pack(">I", zlib.crc32(kind + data) & 0xFFFFFFFF)
    row = b"\x00" + bytes(rgb) * width
    raw = zlib.compress(row * height)
    return (b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", width, height, 8, 2, 0, 0, 0))
            + chunk(b"IDAT", raw) + chunk(b"IEND", b""))


def sparse(path, size):
    """A file of `size` bytes that takes no disk (the 25 MiB read cap, OS drops over 25 MB)."""
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "wb") as f:
        f.truncate(size)


LONG_TS = "".join(
    f"export function step{i:03d}(input: number): number {{\n  // step {i} of the long module\n  return input + {i}\n}}\n\n"
    for i in range(1, 301))

README = """# Review fixture

A checkout with every kind of file the file tree, editor and preview have to handle.

| File | What it exercises |
| ---- | ----------------- |
| `.env`, `.env.example` | dotenv files, one ignored |
| `assets/logo.png` | an image preview |
| `bin/blob.bin` | a binary file |
| `big/` | a folder over 100 files |

```ts
import { step001 } from './src/long-module'
console.log(step001(1))
```

Jump to [the notes](#notes), read [the guide](docs/guide.md) or [the design](./docs/design.md).

## Notes

Relative links resolve against this file.
"""

DESIGN_MD = """---
title: Design notes
tags: [fixture, markdown]
draft: false
---

# Design notes

![Logo](../assets/logo.png)

![Remote image](http://127.0.0.1:8088/logo.png)

Some text after the images.
"""

# Deeply nested inline/HTML mixes and an unterminated table: what crashed the rich editor before.
PLATE_CRASH_MD = """# Editor stress

<details><summary>open <b>me</b></summary>

- [ ] **bold *italic `code` ~~strike~~***
  > - nested > quote > list
  >   | a | b |
  >   |---|
  >   | 1 | 2 | 3 | 4 |

</details>

[link with ![image](x.png) inside](#)

```
unclosed fence
"""


def checkout_kinds(root, crowbar_state):
    """Every file kind the audit names, committed except the ignored, oversized and unreadable ones."""
    write(f"{root}/.gitignore", ".env\nbuild/\n*.log\nhuge/\nprivate/\n")
    write(f"{root}/.editorconfig", "root = true\n[*]\nindent_style = space\n")
    write(f"{root}/.env", "API_TOKEN=rig-not-a-secret\nDEBUG=1\n")
    write(f"{root}/.env.example", "API_TOKEN=\nDEBUG=0\n")
    write(f"{root}/build/out.log", "an ignored build log\n")
    write(f"{root}/README.md", README)
    write(f"{root}/docs/design.md", DESIGN_MD)
    write(f"{root}/docs/guide.md", "# Guide\n\nSee the [README](../README.md#notes).\n")
    write(f"{root}/docs/editor-stress.md", PLATE_CRASH_MD)
    write(f"{root}/src/long-module.ts", LONG_TS)
    write(f"{root}/web/index.html", "<!doctype html>\n<html><head><title>Fixture</title></head>\n"
                                    "<body><h1>Fixture page</h1><p>Static HTML in the checkout.</p></body></html>\n")
    write(f"{root}/assets/logo.png", png(64, 64, (40, 120, 220)))
    write(f"{root}/bin/blob.bin", bytes(range(256)) * 64)
    write(f"{root}/chain/only/child/leaf/file.txt", "a single-child folder chain\n")
    deep = "/".join(f"level-{i:02d}-with-a-rather-long-folder-name" for i in range(1, 9))
    write(f"{root}/deep/{deep}/a-file-at-the-bottom-of-a-very-long-path.ts", "export const deep = true\n")
    for i in range(1, 121):
        write(f"{root}/big/file-{i:03d}.ts", f"export const n{i} = {i}\n")
    os.makedirs(f"{root}/links", exist_ok=True)
    if not os.path.lexists(f"{root}/links/readme-link.md"):
        os.symlink("../README.md", f"{root}/links/readme-link.md")
    if not os.path.lexists(f"{root}/links/crowbar-state"):
        os.symlink(crowbar_state, f"{root}/links/crowbar-state")
    write(f"{root}/scripts/run.sh", "#!/bin/sh\necho run\n", mode=0o755)
    # Untracked: too big to open, and unreadable.
    sparse(f"{root}/huge/over-25MiB.bin", 26 * 1024 * 1024)
    write(f"{root}/private/unreadable.txt", "nobody may read this\n", mode=0o000)


def review_branch(root):
    """Commits on the review branch: a delete, a rename, binary and image changes, two hunks with
    hidden context between, a minified file, a 25 000-line hunk (over the 20 000-line patch cap),
    many files, 250 search hits, then an empty commit."""
    os.remove(f"{root}/docs/old-guide.md")
    git(root, "mv", "src/legacy-name.ts", "src/modern-name.ts")
    write(f"{root}/assets/logo.png", png(64, 64, (220, 80, 40)))
    write(f"{root}/data/blob.bin", bytes(reversed(range(256))) * 64)
    lines = LONG_TS.splitlines(keepends=True)
    lines[10] = "  // changed near the top\n"
    lines[1400] = "  // changed near the bottom\n"
    write(f"{root}/src/long-module.ts", "".join(lines))
    write(f"{root}/dist/bundle.min.js",
          "".join(f"var a{i}=function(b){{return b*{i}}};" for i in range(6000)) + "\n")
    write(f"{root}/src/generated/huge.ts", "".join(f"export const g{i} = {i}\n" for i in range(25000)))
    for i in range(1, 61):
        write(f"{root}/src/many/module-{i:02d}.ts", f"export const m{i} = 'needle {i}'\n")
    write(f"{root}/notes/needles.txt", "".join(f"needle {i}: find me\n" for i in range(250)))
    commit(root, "Review fixture: delete, rename, binary, image, hunks, minified, huge, many files")
    git(root, "-c", "user.name=Crowbar Seed", "-c", "user.email=seed@crowbar.local",
        "commit", "-q", "--allow-empty", "-m", "An empty commit")


def review_base(root):
    """Files on main that the review branch deletes, renames or changes."""
    write(f"{root}/docs/old-guide.md", "# Old guide\n\nThis file is deleted on the review branch.\n")
    write(f"{root}/src/legacy-name.ts", "export const legacy = 'renamed on the review branch'\n")
    write(f"{root}/assets/logo.png", png(64, 64, (40, 120, 220)))
    write(f"{root}/data/blob.bin", bytes(range(256)) * 64)
    write(f"{root}/src/long-module.ts", LONG_TS)
    commit(root, "Base files the review fixture changes")


def uncommitted(root):
    """Every kind of uncommitted change, some under src/: modified, staged, deleted, renamed, untracked."""
    with open(f"{root}/README.md", "a") as f:
        f.write("\nAn unstaged edit to the README.\n")
    with open(f"{root}/src/pricing.ts", "a") as f:
        f.write("\n// A staged edit.\nexport const ROUNDING = 'half-up'\n")
    git(root, "add", "src/pricing.ts")
    os.remove(f"{root}/src/pricing.test.ts")
    os.makedirs(f"{root}/src/stock", exist_ok=True)
    git(root, "mv", "src/inventory.ts", "src/stock/inventory.ts")
    write(f"{root}/src/utils/format/currency.ts", "export const format = (c: number) => `$${(c / 100).toFixed(2)}`\n")
    write(f"{root}/scratch.txt", "an untracked scratch file\n")


def many_lines(root, n=1500):
    write(f"{root}/src/report/table.ts", "".join(f"export const row{i} = {{ id: {i}, label: 'row {i}' }}\n" for i in range(n)))
    commit(root, f"Add a {n}-line table")


def loose(fixtures):
    """Files outside every project: to drop onto the apps, to import, to paste."""
    write(f"{fixtures}/drop/notes.txt", "Plain text to drop into a chat or the editor.\n")
    write(f"{fixtures}/drop/file with spaces.md", "# A file whose name has spaces\n")
    write(f"{fixtures}/drop/screenshot.png", png(320, 200, (90, 160, 90)))
    write(f"{fixtures}/drop/script.sh", "#!/bin/sh\necho dropped\n", mode=0o755)
    sparse(f"{fixtures}/drop/over-25MB.bin", 26 * 1024 * 1024)
    write(f"{fixtures}/clipboard/lines.txt", "".join(f"line {i} of a multi-line paste\n" for i in range(1, 13)))
    write(f"{fixtures}/clipboard/image.png", png(120, 80, (200, 60, 160)))
    write(f"{fixtures}/import/icon.png", png(128, 128, (250, 190, 40)))
    write(f"{fixtures}/settings/crowbar-settings.json",
          '{"version":1,"settings":{"themeMode":"light","sidebarPosition":"right"}}\n')
    write(f"{fixtures}/settings/invalid.json", '{"version":1, "settings": {"themeMode": \n')
    spare = f"{fixtures}/import/spare-repo"
    if not os.path.isdir(f"{spare}/.git"):
        os.makedirs(spare, exist_ok=True)
        git(spare, "init", "-q", "-b", "trunk")
        write(f"{spare}/README.md", "# Spare repo\n\nA git folder to import as a new repo.\n")
        write(f"{spare}/src/index.ts", "export const spare = true\n")
        commit(spare, "Spare repo")
