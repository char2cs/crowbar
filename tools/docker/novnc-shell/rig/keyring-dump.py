#!/usr/bin/env python3
"""`rig keyring`: the Secret Service on the session bus — each collection, whether it is locked, and
each item's label and attributes (never a secret). Both apps keep host tokens there."""
import gi

gi.require_version("Gio", "2.0")
from gi.repository import Gio, GLib  # noqa: E402

SERVICE = "org.freedesktop.secrets"
ROOT = "/org/freedesktop/secrets"
bus = Gio.bus_get_sync(Gio.BusType.SESSION)


def call(path, iface, method, sig, args):
    return bus.call_sync(SERVICE, path, iface, method, GLib.Variant(sig, args), None, 0, 5000, None).unpack()


def prop(path, iface, name):
    return call(path, "org.freedesktop.DBus.Properties", "Get", "(ss)", (iface, name))[0]


print("default", call(ROOT, "org.freedesktop.Secret.Service", "ReadAlias", "(s)", ("default",))[0])
for coll in prop(ROOT, "org.freedesktop.Secret.Service", "Collections"):
    locked = prop(coll, "org.freedesktop.Secret.Collection", "Locked")
    print(coll.rsplit("/", 1)[1], "locked" if locked else "unlocked")
    for item in prop(coll, "org.freedesktop.Secret.Collection", "Items"):
        attrs = prop(item, "org.freedesktop.Secret.Item", "Attributes")
        print("  %s  %s" % (prop(item, "org.freedesktop.Secret.Item", "Label"),
                            " ".join("%s=%s" % kv for kv in sorted(attrs.items()))))
