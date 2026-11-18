#!/usr/bin/env python3
"""An OS drag source for `rig dragfile`: a small GTK 3 window offering files as a file manager does
(text/uri-list, plus text/plain paths), so XDND drops onto either app can be driven with XTest.
Exits when its drag ends, or after --timeout seconds.

    dragsource.py [--timeout S] <file>...
"""
import argparse
import os
import pathlib

import gi

gi.require_version("Gtk", "3.0")
gi.require_version("Gdk", "3.0")
from gi.repository import Gdk, GLib, Gtk  # noqa: E402

p = argparse.ArgumentParser()
p.add_argument("--timeout", type=float, default=30)
p.add_argument("files", nargs="+")
a = p.parse_args()
uris = [pathlib.Path(os.path.abspath(f)).as_uri() for f in a.files]

win = Gtk.Window(title="rig drag source")
win.set_default_size(360, 60)
win.set_accept_focus(False)
box = Gtk.EventBox()
label = Gtk.Label(label=f"rig drag source: {len(uris)} file(s)\n" + ", ".join(os.path.basename(f) for f in a.files))
box.add(label)
win.add(box)
box.drag_source_set(Gdk.ModifierType.BUTTON1_MASK,
                    [Gtk.TargetEntry.new("text/uri-list", 0, 0), Gtk.TargetEntry.new("text/plain", 0, 1)],
                    Gdk.DragAction.COPY | Gdk.DragAction.MOVE | Gdk.DragAction.LINK)


def data_get(_widget, _context, data, info, _time):
    if info == 0:
        data.set_uris(uris)
    else:
        data.set_text("\n".join(os.path.abspath(f) for f in a.files), -1)


def drag_end(*_):
    print("drag ended", flush=True)
    GLib.timeout_add(300, Gtk.main_quit)


box.connect("drag-data-get", data_get)
box.connect("drag-end", drag_end)
win.connect("destroy", Gtk.main_quit)
GLib.timeout_add(int(a.timeout * 1000), Gtk.main_quit)
win.show_all()
print("ready", flush=True)
Gtk.main()
