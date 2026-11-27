#!/usr/bin/env python3
"""Owns the CLIPBOARD selection and fails every read of it, for `rig clip deny|hang`.

    clipowner.py deny   every conversion is refused (SelectionNotify with property None), TARGETS too
    clipowner.py hang   every request is left unanswered, so the reader waits for its own timeout

Exits when another client takes the selection (`rig clip set`, a copy in either app)."""
import sys

from Xlib import X, display
from Xlib.protocol import event

mode = sys.argv[1]
d = display.Display()
win = d.screen().root.create_window(0, 0, 1, 1, 0, X.CopyFromParent)
clipboard = d.intern_atom("CLIPBOARD")
win.set_selection_owner(clipboard, X.CurrentTime)
d.sync()
if d.get_selection_owner(clipboard) != win:
    sys.exit("clipowner: could not own CLIPBOARD")
print(f"clipowner: CLIPBOARD owned, {mode}", flush=True)
while True:
    e = d.next_event()
    if e.type == X.SelectionClear:
        break
    if e.type != X.SelectionRequest or mode == "hang":
        continue
    e.requestor.send_event(event.SelectionNotify(time=e.time, requestor=e.requestor, selection=e.selection,
                                                 target=e.target, property=X.NONE))
    d.flush()
