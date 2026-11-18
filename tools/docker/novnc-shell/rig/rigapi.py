#!/usr/bin/env python3
"""The daemon's HTTP API over its unix socket, for `rig api` and the seed profiles. Stdlib only.

    rigapi.py <METHOD> <path> [json-body | @file]      prints the envelope's data as JSON
    rigapi.py ledger <chat-id>                         prints the chat's ledger rows, one JSON per line
"""
import base64
import http.client
import json
import os
import socket
import struct
import subprocess
import sys
import time


def socket_path():
    raw = subprocess.run(["/opt/crowbar/crowbar-api.real", "paths"], capture_output=True, text=True, check=True).stdout
    return json.loads(raw)["socket"]


class UnixConnection(http.client.HTTPConnection):
    def __init__(self, path, timeout=60):
        super().__init__("localhost", timeout=timeout)
        self.path = path

    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.settimeout(self.timeout)
        self.sock.connect(self.path)


class ApiError(Exception):
    def __init__(self, status, envelope):
        super().__init__(f"{status} {envelope.get('code', '')}: {envelope.get('error', envelope)}")
        self.status = status
        self.envelope = envelope


class Api:
    def __init__(self, sock=None):
        self.sock = sock or socket_path()

    def call(self, method, path, body=None, headers=None, raw=False):
        conn = UnixConnection(self.sock)
        hdrs = dict(headers or {})
        data = None
        if body is not None:
            if isinstance(body, (bytes, str)):
                data = body.encode() if isinstance(body, str) else body
                hdrs.setdefault("Content-Type", "application/yaml")
            else:
                data = json.dumps(body).encode()
                hdrs["Content-Type"] = "application/json"
        conn.request(method, path, body=data, headers=hdrs)
        resp = conn.getresponse()
        payload = resp.read()
        conn.close()
        if raw:
            return resp.status, payload
        if not payload.strip():
            if resp.status >= 400:
                raise ApiError(resp.status, {})
            return None
        envelope = json.loads(payload)
        if resp.status >= 400 or envelope.get("success") is False:
            raise ApiError(resp.status, envelope)
        return envelope.get("data")

    def get(self, path):
        return self.call("GET", path)

    def post(self, path, body=None, **kw):
        return self.call("POST", path, body if body is not None else {}, **kw)

    def patch(self, path, body):
        return self.call("PATCH", path, body)

    def put(self, path, body):
        return self.call("PUT", path, body)

    def delete(self, path):
        return self.call("DELETE", path, headers={"Crowbar-Discard-Work": "true"})

    def until(self, what, probe, timeout=60, every=0.25):
        """Polls probe() until it returns something truthy (202 routes have no GET)."""
        deadline = time.time() + timeout
        while time.time() < deadline:
            got = probe()
            if got:
                return got
            time.sleep(every)
        raise TimeoutError(f"rigapi: timed out waiting for {what}")

    def healthy(self, timeout=60):
        def probe():
            try:
                return (self.get("/v0/health") or {}).get("status") == "ok"
            except (OSError, ApiError, ValueError):
                return False
        return self.until("a healthy daemon", probe, timeout, 0.2)


class Stream:
    """One /v0/stream WebSocket over the daemon socket: sub to scopes, read frames."""

    def __init__(self, sock, timeout=20):
        self.s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.s.settimeout(timeout)
        self.s.connect(sock)
        key = base64.b64encode(os.urandom(16)).decode()
        self.s.sendall((f"GET /v0/stream HTTP/1.1\r\nHost: localhost\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n"
                        f"Sec-WebSocket-Key: {key}\r\nSec-WebSocket-Version: 13\r\n\r\n").encode())
        self.buf = b""
        while b"\r\n\r\n" not in self.buf:
            self.buf += self.s.recv(4096)
        head, self.buf = self.buf.split(b"\r\n\r\n", 1)
        if b" 101 " not in head.split(b"\r\n")[0]:
            raise ApiError(0, {"error": head.decode(errors="replace")})

    def _exact(self, n):
        while len(self.buf) < n:
            chunk = self.s.recv(65536)
            if not chunk:
                raise ConnectionError("stream closed")
            self.buf += chunk
        out, self.buf = self.buf[:n], self.buf[n:]
        return out

    def sub(self, scope):
        payload = json.dumps({"op": "sub", "scope": scope}).encode()
        mask, n = os.urandom(4), len(payload)
        header = bytes([0x81]) + (bytes([0x80 | n]) if n < 126 else bytes([0x80 | 126]) + struct.pack(">H", n))
        self.s.sendall(header + mask + bytes(b ^ mask[i % 4] for i, b in enumerate(payload)))

    def frames(self):
        message = b""
        while True:
            b0, b1 = self._exact(2)
            n = b1 & 0x7F
            if n == 126:
                n = struct.unpack(">H", self._exact(2))[0]
            elif n == 127:
                n = struct.unpack(">Q", self._exact(8))[0]
            data = self._exact(n)
            if b0 & 0x0F == 0x8:
                return
            message += data
            if b0 & 0x80:
                yield json.loads(message)
                message = b""

    def close(self):
        self.s.close()


def ledger(sock, chat_id, timeout=20):
    """A chat's ledger rows, read from the stream's chat:<id> scope up to its `synced` frame."""
    stream = Stream(sock, timeout)
    stream.sub(f"chat:{chat_id}")
    rows = []
    for frame in stream.frames():
        if frame.get("row"):
            rows.append(frame["row"])
        if frame.get("t") == "synced" and frame.get("scope") == f"chat:{chat_id}":
            break
        if frame.get("t") in ("closed", "error"):
            raise ApiError(0, frame)
    stream.close()
    return rows


def hold(sock, scopes, seconds):
    """Holds scopes for a while, as a viewing client does (a forge: scope polls pull requests)."""
    stream = Stream(sock, max(seconds, 1))
    for scope in scopes:
        stream.sub(scope)
    deadline = time.time() + seconds
    try:
        for _ in stream.frames():
            if time.time() > deadline:
                break
    except (socket.timeout, ConnectionError):
        pass
    stream.close()


def main(argv):
    if len(argv) < 2:
        print(__doc__.strip(), file=sys.stderr)
        return 2
    if argv[0] == "ledger":
        for row in ledger(socket_path(), argv[1]):
            print(json.dumps(row))
        return 0
    method, path = argv[0].upper(), argv[1]
    body = None
    if len(argv) > 2:
        arg = argv[2]
        if arg.startswith("@"):
            with open(arg[1:], "rb") as f:
                body = f.read()
        else:
            body = json.loads(arg)
    try:
        data = Api().call(method, path, body)
    except ApiError as err:
        print(f"rigapi: {err}", file=sys.stderr)
        return 1
    if data is not None:
        print(json.dumps(data, indent=2))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
