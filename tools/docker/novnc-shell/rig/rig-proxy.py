#!/usr/bin/env python3
"""A daemon proxy with opt-in fault injection: each app's local daemon socket, or a remote host's TLS port.

Local (the rig's crowbar-api wrapper execs this as the app's `serve`): the app's socket is
forwarded to the rig's one shared daemon, and health replies get their "pid" rewritten to ours
(the Rust shell refuses a daemon whose pid is not the child it spawned).
Remote (`rig remote`): TLS on the host's public port with the daemon's own key and certificate,
so the fingerprint a client pinned still matches, re-encrypted to the daemon on an inner port.

Fault rules live in a file (one JSON object per line); SIGUSR1 reloads it. With no rules a
connection is a plain splice. With rules, each request head is parsed and the first matching rule
answers it with a status, drops the connection or delays it; a spliced keep-alive connection
switches to parsing at its next request. Rules with method WS (daemon -> app) or WS-SEND
(app -> daemon) match each /v0/stream message by "<t|op> <scope> [kind]" and drop, delay or
refuse (a `closed` frame with reason "refused" sent to the app) that message alone.

Usage: rig-proxy.py <listen> <upstream> [--faults FILE] [--tls CERT KEY]
       listen/upstream: a unix socket path, or tcp:HOST:PORT
"""

import argparse
import asyncio
import json
import os
import re
import signal
import ssl
import struct

HEALTH = re.compile(rb"^GET /v0/health[ ?]")
PID_FIELD = re.compile(rb'"pid":\s*\d+')
REQUEST_LINE = re.compile(rb"^[A-Z]+ \S+ HTTP/1\.[01]\r\n")
STREAM = re.compile(rb"^GET /v0/stream[ ?]")
CHUNK = 1 << 16
CODES = {400: "invalid_argument", 401: "unauthorized", 403: "forbidden", 404: "not_found", 409: "conflict",
         413: "too_large", 422: "unprocessable", 424: "failed_dependency", 502: "bad_gateway", 503: "unavailable",
         504: "timeout"}
WS_METHODS = ("WS", "WS-SEND")
PID = os.getpid()
args = None
rules = []


def load_rules():
    global rules
    loaded = []
    try:
        with open(args.faults) as f:
            for line in f:
                if line.strip():
                    r = json.loads(line)
                    loaded.append((r["method"].upper(), re.compile(r["path"]), r["action"], r.get("value")))
    except (OSError, ValueError) as err:
        if not isinstance(err, FileNotFoundError):
            print(f"rig-proxy: faults: {err}", flush=True)
    rules = loaded


def http_rules():
    return [r for r in rules if r[0] not in WS_METHODS]


def header_value(head, name):
    for line in head.split(b"\r\n")[1:]:
        key, _, value = line.partition(b":")
        if key.strip().lower() == name:
            return value.strip()
    return None


class Reader:
    """The client's stream with a pushback buffer, so a spliced chunk can be re-read as a request."""

    def __init__(self, stream):
        self.stream, self.buf = stream, b""

    async def read(self):
        if self.buf:
            data, self.buf = self.buf, b""
            return data
        return await self.stream.read(CHUNK)

    async def readuntil(self, sep):
        if not self.buf:
            return await self.stream.readuntil(sep)
        while sep not in self.buf:
            more = await self.stream.read(CHUNK)
            if not more:
                raise asyncio.IncompleteReadError(self.buf, None)
            self.buf += more
        end = self.buf.index(sep) + len(sep)
        data, self.buf = self.buf[:end], self.buf[end:]
        return data

    async def readexactly(self, n):
        data, self.buf = self.buf[:n], self.buf[n:]
        if len(data) < n:
            data += await self.stream.readexactly(n - len(data))
        return data


async def body_of(reader, head):
    """The raw bytes of a request's body as sent (chunk framing included)."""
    length = header_value(head, b"content-length")
    if length is not None:
        return await reader.readexactly(int(length))
    if (header_value(head, b"transfer-encoding") or b"").lower() != b"chunked":
        return b""
    raw = b""
    while True:
        size_line = await reader.readuntil(b"\r\n")
        raw += size_line
        size = int(size_line.split(b";")[0], 16)
        if size == 0:
            while (trailer := await reader.readuntil(b"\r\n")) != b"\r\n":
                raw += trailer
            return raw + b"\r\n"
        raw += await reader.readexactly(size + 2)


async def read_response_body(reader, head):
    length = header_value(head, b"content-length")
    if length is not None:
        return await reader.readexactly(int(length))
    if (header_value(head, b"transfer-encoding") or b"").lower() == b"chunked":
        body = b""
        while True:
            size = int((await reader.readuntil(b"\r\n")).split(b";")[0], 16)
            chunk = await reader.readexactly(size + 2)
            if size == 0:
                return body
            body += chunk[:-2]
    return await reader.read()


def open_upstream():
    if args.upstream.startswith("tcp:"):
        host, port = args.upstream[4:].rsplit(":", 1)
        ctx = ssl.create_default_context()
        ctx.check_hostname, ctx.verify_mode = False, ssl.CERT_NONE  # our own daemon, one hop away
        return asyncio.open_connection(host, int(port), ssl=ctx)
    return asyncio.open_unix_connection(args.upstream)


async def health(request):
    reader, writer = await open_upstream()
    try:
        writer.write(request)
        await writer.drain()
        head = await reader.readuntil(b"\r\n\r\n")
        body = PID_FIELD.sub(b'"pid":%d' % PID, await read_response_body(reader, head))
    finally:
        writer.close()
    kept = [
        line
        for line in head[:-4].split(b"\r\n")
        if line.split(b":")[0].strip().lower() not in (b"content-length", b"transfer-encoding")
    ]
    kept.append(b"Content-Length: %d" % len(body))
    return b"\r\n".join(kept) + b"\r\n\r\n" + body


def matching_rule(head):
    method, target = head.split(b" ", 2)[:2]
    method, path = method.decode(), target.decode(errors="replace")
    # `*` leaves CORS preflights alone: a failed preflight reaches the web as a network error.
    methods = (method,) if method == "OPTIONS" else ("*", method)
    return next((r for r in http_rules() if r[0] in methods and r[1].search(path)), None)


def fault_response(status, head):
    body = json.dumps({"success": False, "error": f"rig fault: injected {status}",
                       "code": CODES.get(status, "internal")}).encode()
    lines = [b"HTTP/1.1 %d Rig Fault" % status, b"Content-Type: application/json; charset=utf-8",
             b"Content-Length: %d" % len(body)]
    origin = header_value(head, b"origin")
    if origin is not None:
        # The daemon's CORS headers: without them WebKit turns the answer into a network error.
        lines += [b"Access-Control-Allow-Origin: " + origin, b"Access-Control-Allow-Credentials: true",
                  b"Access-Control-Expose-Headers: X-Crowbar-Diff-Truncated", b"Vary: Origin"]
    return b"\r\n".join(lines) + b"\r\n\r\n" + body


# ---------------------------------------------------------------- /v0/stream messages


def ws_frame(text):
    """One unmasked text frame (daemon -> app)."""
    data = text.encode()
    n = len(data)
    head = bytes([0x81, n]) if n < 126 else bytes([0x81, 126]) + struct.pack(">H", n) if n < 1 << 16 \
        else bytes([0x81, 127]) + struct.pack(">Q", n)
    return head + data


async def read_frame(reader):
    """One raw frame: (bytes as sent, fin, opcode, unmasked payload)."""
    head = await reader.readexactly(2)
    n = head[1] & 0x7F
    ext = b""
    if n == 126:
        ext = await reader.readexactly(2)
        n = struct.unpack(">H", ext)[0]
    elif n == 127:
        ext = await reader.readexactly(8)
        n = struct.unpack(">Q", ext)[0]
    mask = await reader.readexactly(4) if head[1] & 0x80 else b""
    payload = await reader.readexactly(n)
    plain = bytes(b ^ mask[i % 4] for i, b in enumerate(payload)) if mask and rules else payload
    return head + ext + mask + payload, head[0] & 0x80, head[0] & 0x0F, plain


def ws_match(direction, message):
    wanted = [r for r in rules if r[0] == direction]
    if not wanted:
        return None, None
    try:
        msg = json.loads(message)
    except ValueError:
        return None, None
    if direction == "WS":
        key = " ".join(x for x in (msg.get("t", ""), msg.get("scope", ""), msg.get("kind", "")) if x)
    else:
        key = " ".join(x for x in (msg.get("op", ""), msg.get("scope", "")) if x)
    return next((r for r in wanted if r[1].search(key)), None), msg


async def relay_messages(direction, reader, writer, to_app):
    """Forwards whole WebSocket messages, applying WS / WS-SEND rules to each text message."""
    raw, text = b"", b""
    try:
        while True:
            frame, fin, opcode, payload = await read_frame(reader)
            if opcode >= 0x8:  # control frames pass at once, even between fragments
                writer.write(frame)
                await writer.drain()
                if opcode == 0x8:
                    return
                continue
            raw, text = raw + frame, text + payload
            if not fin:
                continue
            rule, msg = ws_match(direction, text)
            message, raw, text = raw, b"", b""
            if rule is None:
                writer.write(message)
            elif rule[2] == "delay":
                asyncio.get_running_loop().call_later(rule[3] / 1000, lambda m=message: writer.is_closing() or writer.write(m))
            elif rule[2] == "refuse":
                closed = {"t": "closed", "scope": msg.get("scope", ""), "reason": "refused",
                          "problem": {"code": "unavailable", "message": "rig fault: refused"}}
                to_app.write(ws_frame(json.dumps(closed)))
            # drop: the message is not forwarded
            await writer.drain()
    except (asyncio.IncompleteReadError, ConnectionError):
        pass
    finally:
        writer.close()


class Connection:
    def __init__(self, client_reader, client_writer):
        self.client, self.out = Reader(client_reader), client_writer
        self.up_writer = None
        self.down = None

    async def run(self):
        while True:
            head = await self.client.readuntil(b"\r\n\r\n")
            rule = matching_rule(head) if rules else None
            if rule and rule[2] == "drop":
                return
            if rule and rule[2] == "delay":
                await asyncio.sleep(rule[3] / 1000)
            elif rule:
                await body_of(self.client, head)
                self.out.write(fault_response(rule[3], head))
                await self.out.drain()
                continue
            if HEALTH.match(head) and not args.upstream.startswith("tcp:"):
                self.out.write(await health(head))
                await self.out.drain()
                if (header_value(head, b"connection") or b"").lower() == b"close":
                    return
                continue
            if not await self.forward(head):
                return

    async def forward(self, head):
        """Sends one request upstream; returns False once the connection is done."""
        if header_value(head, b"upgrade") is not None and STREAM.match(head) and self.up_writer is None:
            await self.stream(head)
            return False
        if self.up_writer is None:
            up_reader, self.up_writer = await open_upstream()
            self.down = asyncio.ensure_future(splice(lambda: up_reader.read(CHUNK), self.out))
        if header_value(head, b"upgrade") is not None:
            self.up_writer.write(head)
            await splice(self.client.read, self.up_writer)
            return False
        if rules:
            # Head first: a client sending `Expect: 100-continue` waits for the daemon before its body.
            self.up_writer.write(head)
            await self.up_writer.drain()
            self.up_writer.write(await body_of(self.client, head))
            await self.up_writer.drain()
            return True
        self.up_writer.write(head)
        return await self.pump()

    async def stream(self, head):
        """The event stream on its own upstream connection, relayed message by message."""
        up_reader, self.up_writer = await open_upstream()
        self.up_writer.write(head)
        await self.up_writer.drain()
        answer = await up_reader.readuntil(b"\r\n\r\n")
        self.out.write(answer)
        await self.out.drain()
        if b" 101 " not in answer.split(b"\r\n", 1)[0]:
            self.out.write(await up_reader.read(CHUNK))
            return
        await asyncio.gather(relay_messages("WS", up_reader, self.out, self.out),
                             relay_messages("WS-SEND", self.client, self.up_writer, self.out))

    async def pump(self):
        """Splices client -> daemon until a chunk that starts a request arrives while rules are set."""
        while data := await self.client.read():
            if rules and REQUEST_LINE.match(data):
                self.client.buf = data
                return True
            self.up_writer.write(data)
            await self.up_writer.drain()
        return False

    def close(self):
        self.out.close()
        if self.up_writer is not None:
            self.up_writer.close()


async def splice(read, writer):
    try:
        while data := await read():
            writer.write(data)
            await writer.drain()
    except (ConnectionError, asyncio.IncompleteReadError, ssl.SSLError):
        pass
    finally:
        writer.close()


async def serve(client_reader, client_writer):
    # Once the client stops sending (or a rule drops it), both sides close, as a plain splice does.
    conn = Connection(client_reader, client_writer)
    try:
        await conn.run()
    except (asyncio.IncompleteReadError, ConnectionError, asyncio.LimitOverrunError, OSError, ValueError):
        pass
    finally:
        conn.close()


async def main():
    load_rules()
    if args.listen.startswith("tcp:"):
        host, port = args.listen[4:].rsplit(":", 1)
        ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        ctx.minimum_version = ssl.TLSVersion.TLSv1_3  # as the daemon's own listener
        ctx.load_cert_chain(*args.tls)
        server = await asyncio.start_server(serve, host or None, int(port), ssl=ctx, limit=1 << 20)
    else:
        os.makedirs(os.path.dirname(args.listen), mode=0o700, exist_ok=True)
        if os.path.exists(args.listen):
            os.unlink(args.listen)
        server = await asyncio.start_unix_server(serve, args.listen, limit=1 << 20)
    stop = asyncio.Event()
    loop = asyncio.get_running_loop()
    for sig in (signal.SIGTERM, signal.SIGINT):
        loop.add_signal_handler(sig, stop.set)
    loop.add_signal_handler(signal.SIGUSR1, load_rules)
    async with server:
        await stop.wait()
    if not args.listen.startswith("tcp:"):
        os.unlink(args.listen)


if __name__ == "__main__":
    p = argparse.ArgumentParser()
    p.add_argument("listen")
    p.add_argument("upstream")
    p.add_argument("--faults", default=os.path.expanduser("~/.rig/faults"))
    p.add_argument("--tls", nargs=2, metavar=("CERT", "KEY"))
    args = p.parse_args()
    asyncio.run(main())
