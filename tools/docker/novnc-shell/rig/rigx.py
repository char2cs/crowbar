#!/usr/bin/env python3
"""X11 capture, input and measurement for the rig (called by `rig`; see README.md).

Frames are read from the root window at the app window's rectangle (windows never overlap in the
rig, and Xvfb has no compositor, so that is exactly what the window shows). A "changed frame" is
one whose pixels differ from the previous frame in at least --min-px pixels; X Damage on the root
window says when to look, so a frame is timed when the X server reports the drawing, not when we
happen to poll.
"""

import argparse
import json
import os
import select
import sqlite3
import statistics
import subprocess
import sys
import time

import numpy as np
from PIL import Image, ImageDraw
from Xlib import X, XK, display
from Xlib.ext import damage, xtest

RIG = os.environ.get("RIG_BIN", "/opt/rig/bin/rig")
PERF_DIR = os.path.expanduser("~/.rig/perf")


def now_ms():
    return time.monotonic() * 1000.0


# ---------------------------------------------------------------- X access


class Screen:
    def __init__(self):
        self.d = display.Display()
        self.root = self.d.screen().root
        self.d.damage_query_version()
        self.dmg = self.root.damage_create(damage.DamageReportNonEmpty)
        self.d.sync()

    def rect_of(self, wid):
        win = self.d.create_resource_object("window", wid)
        geom = win.get_geometry()
        origin = win.translate_coords(self.root, 0, 0)
        return (-origin.x, -origin.y, geom.width, geom.height)

    def grab(self, rect):
        x, y, w, h = rect
        raw = self.root.get_image(x, y, w, h, X.ZPixmap, 0xFFFFFFFF).data
        return np.frombuffer(raw, np.uint32).reshape(h, w) & 0x00FFFFFF

    def wait_damage(self, timeout_ms):
        """Block until the X server reports drawing anywhere on screen; returns its time or None."""
        deadline = now_ms() + timeout_ms
        while True:
            while self.d.pending_events():
                if self.d.next_event().type == self.d.extension_event.DamageNotify:
                    seen = now_ms()
                    self.d.damage_subtract(self.dmg)
                    self.d.flush()
                    return seen
            left = deadline - now_ms()
            if left <= 0:
                return None
            select.select([self.d.fileno()], [], [], left / 1000.0)

    def drain(self):
        self.d.damage_subtract(self.dmg)
        self.d.sync()
        while self.d.pending_events():
            self.d.next_event()

    def key(self, name):
        """Press then release `name` (an X keysym name, optionally ctrl+/shift+/alt+ prefixed)."""
        *mods, base = name.split("+")
        names = {"ctrl": "Control_L", "shift": "Shift_L", "alt": "Alt_L", "super": "Super_L"}
        codes = [self.d.keysym_to_keycode(XK.string_to_keysym(names.get(m, m))) for m in mods]
        main = self.d.keysym_to_keycode(XK.string_to_keysym(base))
        for code in codes:
            xtest.fake_input(self.d, X.KeyPress, code)
        xtest.fake_input(self.d, X.KeyPress, main)
        xtest.fake_input(self.d, X.KeyRelease, main)
        for code in reversed(codes):
            xtest.fake_input(self.d, X.KeyRelease, code)
        self.d.flush()

    def move(self, x, y):
        xtest.fake_input(self.d, X.MotionNotify, x=x, y=y)
        self.d.flush()

    def button(self, number):
        xtest.fake_input(self.d, X.ButtonPress, number)
        xtest.fake_input(self.d, X.ButtonRelease, number)
        self.d.flush()


def changed_px(a, b):
    return int(np.count_nonzero(a != b))


def to_image(frame):
    """A grabbed frame (one 0x00RRGGBB word per pixel) as an RGB image."""
    bgrx = frame.astype("<u4").view(np.uint8).reshape(frame.shape[0], frame.shape[1], 4)
    return Image.fromarray(np.ascontiguousarray(bgrx[:, :, 2::-1]))


class Watch:
    """Tracks one rectangle's frames; `step` returns the time of a changed frame or None."""

    def __init__(self, screen, rect, min_px):
        self.s, self.rect, self.min_px = screen, rect, min_px
        self.frame = screen.grab(rect)

    def step(self, timeout_ms):
        seen = self.s.wait_damage(timeout_ms)
        if seen is None:
            return None
        frame = self.s.grab(self.rect)
        if changed_px(frame, self.frame) < self.min_px:
            return None
        self.frame = frame
        return seen

    def quiet(self, quiet_ms, limit_ms):
        """Wait until no changed frame for quiet_ms; returns the last change time (or None)."""
        last = None
        start = now_ms()
        mark = start
        while now_ms() - start < limit_ms:
            seen = self.step(max(1.0, quiet_ms - (now_ms() - mark)))
            if seen is not None:
                last = mark = seen
            elif now_ms() - mark >= quiet_ms:
                return last
        raise SystemExit(f"rig: the window never settled within {limit_ms / 1000:.0f}s")


def stats(values):
    ordered = sorted(values)
    p95 = ordered[min(len(ordered) - 1, int(round(0.95 * (len(ordered) - 1))))]
    return {
        "n": len(values),
        "median": round(statistics.median(values), 1),
        "p95": round(p95, 1),
        "min": round(ordered[0], 1),
        "max": round(ordered[-1], 1),
    }


def report(metric, app, result):
    result = {"metric": metric, "app": app, **result}
    os.makedirs(PERF_DIR, exist_ok=True)
    with open(os.path.join(PERF_DIR, "results.jsonl"), "a") as log:
        log.write(json.dumps({"at": time.strftime("%Y-%m-%dT%H:%M:%S"), **result}) + "\n")
    print(json.dumps(result))


def rig(*args, capture=False):
    run = subprocess.run([RIG, *args], check=True, text=True, capture_output=capture)
    return run.stdout.strip() if capture else None


def window_rect(screen, app):
    return screen.rect_of(int(rig("winid", app, capture=True)))


def at(rect, point):
    x, y = (int(v) for v in point.split(","))
    return rect[0] + x, rect[1] + y


# ---------------------------------------------------------------- process trees


def tree(root_pid):
    children = {}
    for entry in os.listdir("/proc"):
        if not entry.isdigit():
            continue
        try:
            with open(f"/proc/{entry}/stat") as f:
                fields = f.read().rsplit(")", 1)[1].split()
            with open(f"/proc/{entry}/cmdline", "rb") as f:
                cmd = f.read()
        except OSError:
            continue
        if b"rig-proxy.py" in cmd:
            continue  # the rig's socket proxy is not part of either app
        children.setdefault(int(fields[1]), []).append(int(entry))
    found, todo = [], [root_pid]
    while todo:
        pid = todo.pop()
        found.append(pid)
        todo.extend(children.get(pid, []))
    return found


def cpu_ticks(pid):
    try:
        with open(f"/proc/{pid}/stat") as f:
            fields = f.read().rsplit(")", 1)[1].split()
        return int(fields[11]) + int(fields[12])
    except OSError:
        return None


def pss_kib(pids):
    total = 0
    for pid in pids:
        try:
            with open(f"/proc/{pid}/smaps_rollup") as f:
                for line in f:
                    if line.startswith("Pss:"):
                        total += int(line.split()[1])
        except OSError:
            pass
    return total


def names(pids):
    out = []
    for pid in pids:
        try:
            with open(f"/proc/{pid}/comm") as f:
                out.append(f.read().strip())
        except OSError:
            pass
    return out


# ---------------------------------------------------------------- commands


def cmd_shot(a):
    s = Screen()
    rect = s.rect_of(int(a.window))
    to_image(s.grab(rect)).save(a.out)
    print(a.out)


def cmd_drag(a):
    """Press at (x1, y1), move to (x2, y2) in steps, hold, release: one X client does the input and
    reads the frames, so captures never race the drag (window-relative points, XTest input)."""
    s = Screen()
    rect = s.rect_of(int(a.window))
    src = (a.src_x, a.src_y) if a.src_x is not None else (rect[0] + a.x1, rect[1] + a.y1)
    dst = (rect[0] + a.x2, rect[1] + a.y2)
    names = {"ctrl": "Control_L", "shift": "Shift_L", "alt": "Alt_L", "super": "Super_L"}
    mods = [s.d.keysym_to_keycode(XK.string_to_keysym(names.get(m, m))) for m in a.mods.split("+") if m]
    shots = []

    def capture(tag):
        if a.frames:
            path = os.path.join(a.out, f"{a.app}-{len(shots):02d}-{tag}.png")
            to_image(s.grab(rect)).save(path)
            shots.append(path)

    s.move(*src)
    time.sleep(0.05)
    for code in mods:
        xtest.fake_input(s.d, X.KeyPress, code)
    xtest.fake_input(s.d, X.ButtonPress, a.button)
    s.d.flush()
    time.sleep(a.hold / 1000)
    for i in range(1, a.steps + 1):
        s.move(round(src[0] + (dst[0] - src[0]) * i / a.steps), round(src[1] + (dst[1] - src[1]) * i / a.steps))
        time.sleep(a.step_ms / 1000)
        if i == a.steps // 2:
            time.sleep(0.1)
            capture("mid")
    for _ in range(max(a.frames - 1, 0)):
        time.sleep(a.every / 1000)
        capture("held")
    time.sleep(a.hold / 1000)
    xtest.fake_input(s.d, X.ButtonRelease, a.button)
    for code in reversed(mods):
        xtest.fake_input(s.d, X.KeyRelease, code)
    s.d.flush()
    if a.frames:
        time.sleep(0.5)
        capture("dropped")
    for path in shots:
        print(path)


def cmd_compose(a):
    left, right = Image.open(a.left).convert("RGB"), Image.open(a.right).convert("RGB")
    label, gap = 28, 16
    w = left.width + right.width + gap
    h = max(left.height, right.height) + label
    out = Image.new("RGB", (w, h), (128, 128, 128))
    out.paste(left, (0, label))
    out.paste(right, (left.width + gap, label))
    draw = ImageDraw.Draw(out)
    draw.text((8, 8), f"{a.left_label}  {left.width}x{left.height}", fill=(255, 255, 255))
    draw.text((left.width + gap + 8, 8), f"{a.right_label}  {right.width}x{right.height}", fill=(255, 255, 255))
    out.save(a.out)
    print(a.out)


def cmd_startup(a):
    """exec -> the first frame equal (within --settle-px pixels) to the frame the window settles on."""
    s = Screen()
    rect = tuple(int(v) for v in a.slot.split(","))
    background = s.grab(rect)
    s.drain()
    frames = []
    t0 = now_ms()
    starter = subprocess.Popen([RIG, "start", a.app], stdout=subprocess.DEVNULL)
    prev = background
    mark = t0
    while now_ms() - t0 < a.limit * 1000:
        seen = s.wait_damage(100)
        frame = s.grab(rect)
        if changed_px(frame, prev) >= a.min_px:
            frames.append(((seen or now_ms()) - t0, frame))
            prev = frame
            mark = now_ms()
        elif now_ms() - mark >= a.quiet and frames and starter.poll() is not None:
            break
    else:
        raise SystemExit(f"rig: {a.app} never settled within {a.limit}s")
    if starter.returncode:
        raise SystemExit(f"rig: starting {a.app} failed")
    final = frames[-1][1]
    ready = next(t for t, f in frames if changed_px(f, final) <= a.settle_px)
    shot = os.path.join(PERF_DIR, f"{a.app}-startup-final.png")
    os.makedirs(PERF_DIR, exist_ok=True)
    to_image(final).save(shot)
    report("startup", a.app, {
        "interactive_ms": round(ready, 1),
        "first_paint_ms": round(frames[0][0], 1),
        "changed_frames": len(frames),
        "final_frame": shot,
    })


def cmd_input(a):
    s = Screen()
    rect = window_rect(s, a.app)
    if a.focus:
        s.move(*at(rect, a.focus))
        s.button(1)
    w = Watch(s, rect, a.min_px)
    w.quiet(500, 20000)
    lat = []
    for _ in range(a.n):
        w.quiet(a.gap, 20000)
        s.drain()
        w.frame = s.grab(rect)
        t0 = now_ms()
        s.key(a.key)
        seen = None
        while seen is None and now_ms() - t0 < 5000:
            seen = w.step(5000 - (now_ms() - t0))
        if seen is None:
            raise SystemExit(f"rig: {a.key} changed nothing in 5s (is the focus point an input?)")
        lat.append(seen - t0)
    w.quiet(a.gap, 20000)
    for _ in range(a.n if a.undo else 0):
        s.key(a.undo)
        time.sleep(0.02)
    report("input_latency", a.app, {"key": a.key, "ms": stats(lat)})


def cmd_switch(a):
    s = Screen()
    rect = window_rect(s, a.app)
    targets = [at(rect, a.a), at(rect, a.b)]
    w = Watch(s, rect, a.min_px)
    s.move(*targets[0])
    s.button(1)
    w.quiet(a.quiet, 30000)
    first, done = [], []
    for i in range(a.n):
        x, y = targets[(i + 1) % 2]
        s.move(x, y)
        w.quiet(300, 10000)  # hover effects settle before the timed click
        s.drain()
        w.frame = s.grab(rect)
        t0 = now_ms()
        s.button(1)
        seen = None
        while seen is None and now_ms() - t0 < 5000:
            seen = w.step(5000 - (now_ms() - t0))
        if seen is None:
            raise SystemExit("rig: the click changed nothing in 5s (are the two points switch targets?)")
        last = w.quiet(a.quiet, 30000) or seen
        first.append(seen - t0)
        done.append(last - t0)
    report("switch", app=a.app, result={"first_change_ms": stats(first), "settled_ms": stats(done)})


def cmd_scroll(a):
    s = Screen()
    rect = window_rect(s, a.app)
    s.move(*at(rect, a.at))
    w = Watch(s, rect, a.min_px)
    w.quiet(500, 20000)
    # The seeded chat opens scrolled to its end, so the timed pass scrolls up; the second puts it back.
    for direction, measured in ((4, True), (5, False)):
        times = []
        ticks = int(a.seconds * a.rate)
        t0 = now_ms()
        for i in range(ticks):
            s.button(direction)
            due = t0 + (i + 1) * 1000.0 / a.rate
            while now_ms() < due:
                seen = w.step(due - now_ms())
                if seen is not None:
                    times.append(seen)
        tail_end = now_ms() + 1000
        while now_ms() < tail_end:
            seen = w.step(tail_end - now_ms())
            if seen is not None:
                times.append(seen)
        if not measured:
            break
        if len(times) < 3:
            raise SystemExit("rig: scrolling changed fewer than 3 frames (is the point over a scrollable view?)")
        gaps = [b - c for b, c in zip(times[1:], times)]
        span = times[-1] - times[0]
        report("scroll", a.app, {
            "wheel_ticks": ticks,
            "changed_frames": len(times),
            "fps": round((len(times) - 1) * 1000.0 / span, 1) if span else 0.0,
            "frame_interval_ms": stats(gaps),
            "frames_over_33ms": sum(1 for g in gaps if g > 33.4),
        })


def cmd_idle(a):
    pid = int(a.pid)
    before = {p: cpu_ticks(p) for p in tree(pid)}
    time.sleep(a.seconds)
    pids = tree(pid)
    used = 0
    for p in pids:
        after = cpu_ticks(p)
        if after is not None:
            used += after - (before.get(p) or 0)
    hz = os.sysconf("SC_CLK_TCK")
    report("idle_cpu", a.app, {
        "seconds": a.seconds,
        "cpu_seconds": round(used / hz, 2),
        "cpu_percent_of_one_core": round(100.0 * used / hz / a.seconds, 2),
        "processes": names(pids),
    })


def cmd_memory(a):
    s = Screen()
    rect = window_rect(s, a.app)
    pid = int(a.pid)
    w = Watch(s, rect, a.min_px)
    w.quiet(5000, 120000)  # the same 5 s quiet period that ends `startup`
    time.sleep(a.settle)
    base = pss_kib(tree(pid))
    steps = []
    for _ in range(a.tabs):
        if a.focus:  # keys typed inside a terminal tab go to the terminal, so refocus first
            s.move(*at(rect, a.focus))
            s.button(1)
            w.quiet(a.quiet, 30000)
        s.key(a.key)
        w.quiet(a.quiet, 30000)
        time.sleep(a.settle)
        steps.append(pss_kib(tree(pid)))
    shot = os.path.join(PERF_DIR, f"{a.app}-memory-tabs.png")
    to_image(s.grab(rect)).save(shot)
    deltas = [b - c for b, c in zip(steps, [base] + steps)]
    report("memory_pss", a.app, {
        "startup_mib": round(base / 1024, 1),
        "after_each_tab_mib": [round(v / 1024, 1) for v in steps],
        "per_tab_mib": round(statistics.mean(deltas) / 1024, 1) if deltas else None,
        "processes": names(tree(pid)),
        "new_tab_key": a.key,
        "tabs_frame": shot,
    })


def cmd_tauri_set(a):
    """Write one setting into the Tauri app's WebKit localStorage (stopped app, existing database)."""
    with sqlite3.connect(f"file:{a.db}?mode=rw", uri=True) as db:
        db.execute(
            "INSERT INTO ItemTable (key, value) VALUES (?, ?)",
            ("crowbar:settings:" + a.key, json.dumps(a.value).encode("utf-16-le")),
        )


def cmd_rust_set(a):
    """Write one setting into the Rust shell's settings.json (the app must be stopped)."""
    try:
        with open(a.file) as f:
            values = json.load(f)
    except FileNotFoundError:
        values = {}
    values[a.key] = a.value
    os.makedirs(os.path.dirname(a.file), mode=0o700, exist_ok=True)
    with open(a.file + ".tmp", "w") as f:
        json.dump(values, f, indent=2)
    os.replace(a.file + ".tmp", a.file)


def cmd_health(a):
    import socket

    sock = socket.socket(socket.AF_UNIX)
    sock.settimeout(2)
    try:
        sock.connect(a.socket)
        sock.sendall(b"GET /v0/health HTTP/1.1\r\nHost: crowbar\r\nConnection: close\r\n\r\n")
        reply = b""
        while chunk := sock.recv(65536):
            reply += chunk
    except OSError as error:
        raise SystemExit(f"rig: {a.socket}: {error}")
    head, _, body = reply.partition(b"\r\n\r\n")
    if b" 200 " not in head.split(b"\r\n")[0] or b'"status":"ok"' not in body:
        raise SystemExit(f"rig: daemon not ready: {body[:200]!r}")
    print(body.decode(errors="replace"))


def main():
    p = argparse.ArgumentParser()
    sub = p.add_subparsers(dest="cmd", required=True)
    c = sub.add_parser("shot"); c.add_argument("window"); c.add_argument("out"); c.set_defaults(f=cmd_shot)
    c = sub.add_parser("drag")
    c.add_argument("app"); c.add_argument("window")
    for name in ("x1", "y1", "x2", "y2"):
        c.add_argument(name, type=int)
    c.add_argument("--steps", type=int, default=20); c.add_argument("--step-ms", type=float, default=20)
    c.add_argument("--button", type=int, default=1); c.add_argument("--hold", type=float, default=150)
    c.add_argument("--mods", default=""); c.add_argument("--frames", type=int, default=0)
    c.add_argument("--every", type=float, default=100); c.add_argument("--out", default=os.path.expanduser("~/shots/drag"))
    c.add_argument("--src-x", type=int); c.add_argument("--src-y", type=int)  # a root-window start point
    c.set_defaults(f=cmd_drag)
    c = sub.add_parser("compose")
    for name in ("left", "right", "left_label", "right_label", "out"):
        c.add_argument(name)
    c.set_defaults(f=cmd_compose)

    def perf(name, fn):
        c = sub.add_parser(name)
        c.add_argument("app")
        c.add_argument("--min-px", type=int, default=int(os.environ.get("RIG_MIN_PX", "24")))
        c.set_defaults(f=fn)
        return c

    c = perf("startup", cmd_startup)
    c.add_argument("--slot", required=True)
    c.add_argument("--quiet", type=float, default=5000)
    c.add_argument("--settle-px", type=int, default=512)
    c.add_argument("--limit", type=float, default=120)
    c = perf("input", cmd_input)
    c.add_argument("--focus"); c.add_argument("--key", default="m"); c.add_argument("--undo", default="BackSpace")
    c.add_argument("--n", type=int, default=30); c.add_argument("--gap", type=float, default=250)
    c = perf("switch", cmd_switch)
    c.add_argument("--a", required=True); c.add_argument("--b", required=True)
    c.add_argument("--n", type=int, default=10); c.add_argument("--quiet", type=float, default=1500)
    c = perf("scroll", cmd_scroll)
    c.add_argument("--at", required=True); c.add_argument("--rate", type=float, default=60)
    c.add_argument("--seconds", type=float, default=2)
    c = perf("idle", cmd_idle)
    c.add_argument("--pid", required=True); c.add_argument("--seconds", type=float, default=60)
    c = perf("memory", cmd_memory)
    c.add_argument("--pid", required=True); c.add_argument("--tabs", type=int, default=5)
    c.add_argument("--key", default="ctrl+j"); c.add_argument("--focus")
    c.set_defaults(min_px=1000)  # settling only: ignore a blinking terminal cursor
    c.add_argument("--quiet", type=float, default=1500); c.add_argument("--settle", type=float, default=3)
    c = sub.add_parser("tauri-set")
    c.add_argument("db"); c.add_argument("key"); c.add_argument("value"); c.set_defaults(f=cmd_tauri_set)
    c = sub.add_parser("rust-set")
    c.add_argument("file"); c.add_argument("key"); c.add_argument("value"); c.set_defaults(f=cmd_rust_set)
    c = sub.add_parser("health"); c.add_argument("socket"); c.set_defaults(f=cmd_health)
    a = p.parse_args()
    a.f(a)


if __name__ == "__main__":
    sys.exit(main())
