#!/usr/bin/env python3
"""Dumps an application's AT-SPI accessibility tree (`rig a11y dump`): role, name, states and
extents, indented by depth. Needs `rig a11y on` (a session bus, the AT-SPI registry, and the
apps restarted with their accessibility bridge on).

    a11y-dump.py [--pid PID | --name NAME] [--depth N]   no filter: every application on the bus
"""
import argparse
import sys

import pyatspi


def describe(node):
    try:
        states = sorted(pyatspi.stateToString(s) for s in node.getState().getStates())
    except Exception:  # noqa: BLE001 - a dead node still prints its role
        states = []
    try:
        e = node.queryComponent().getExtents(pyatspi.DESKTOP_COORDS)
        box = f" @{e.x},{e.y} {e.width}x{e.height}"
    except Exception:  # noqa: BLE001 - not every node is a component
        box = ""
    keep = [s for s in states if s in ("focused", "selected", "checked", "expanded", "disabled", "focusable", "editable")]
    return f"[{node.getRoleName()}] {node.name!r}{box} {' '.join(keep)}".rstrip()


def walk(node, depth, limit, out):
    out.write("  " * depth + describe(node) + "\n")
    if depth >= limit:
        return
    for i in range(node.childCount):
        try:
            child = node.getChildAtIndex(i)
        except Exception:  # noqa: BLE001 - children can vanish mid-walk
            continue
        if child is not None:
            walk(child, depth + 1, limit, out)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--pid", type=int)
    ap.add_argument("--name")
    ap.add_argument("--depth", type=int, default=40)
    args = ap.parse_args()
    desktop = pyatspi.Registry.getDesktop(0)
    found = 0
    for app in desktop:
        if app is None:
            continue
        if args.pid and app.get_process_id() != args.pid:
            continue
        if args.name and args.name.lower() not in (app.name or "").lower():
            continue
        found += 1
        sys.stdout.write(f"# application {app.name!r} pid {app.get_process_id()}\n")
        walk(app, 0, args.depth, sys.stdout)
    if not found:
        apps = [(a.name, a.get_process_id()) for a in desktop if a is not None]
        sys.exit(f"a11y-dump: no matching application on the bus; registered: {apps}")


if __name__ == "__main__":
    main()
