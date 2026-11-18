#!/usr/bin/env python3
"""Forward one app's daemon socket to the rig's single shared daemon.

Each app spawns `crowbar-api serve` and expects that child to be the daemon. The rig's
crowbar-api wrapper execs this proxy instead, so the child's pid is this process: health
replies get their "pid" rewritten to ours (the Rust shell refuses a daemon whose pid is not
the child it spawned). Every other request is spliced through unchanged, byte for byte.
Usage: rig-proxy.py <listen socket> <daemon socket>
"""

import asyncio
import os
import re
import signal
import sys

LISTEN, UPSTREAM = sys.argv[1], sys.argv[2]
PID = os.getpid()
HEALTH = re.compile(rb"^GET /v0/health[ ?]")
PID_FIELD = re.compile(rb'"pid":\s*\d+')
CHUNK = 1 << 16


async def splice(reader, writer):
    try:
        while data := await reader.read(CHUNK):
            writer.write(data)
            await writer.drain()
    except (ConnectionError, asyncio.IncompleteReadError):
        pass
    finally:
        writer.close()


def header_value(head, name):
    for line in head.split(b"\r\n")[1:]:
        key, _, value = line.partition(b":")
        if key.strip().lower() == name:
            return value.strip()
    return None


async def read_body(reader, head):
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


async def health(request):
    reader, writer = await asyncio.open_unix_connection(UPSTREAM)
    try:
        writer.write(request)
        await writer.drain()
        head = await reader.readuntil(b"\r\n\r\n")
        body = PID_FIELD.sub(b'"pid":%d' % PID, await read_body(reader, head))
    finally:
        writer.close()
    kept = [
        line
        for line in head[:-4].split(b"\r\n")
        if line.split(b":")[0].strip().lower() not in (b"content-length", b"transfer-encoding")
    ]
    kept.append(b"Content-Length: %d" % len(body))
    return b"\r\n".join(kept) + b"\r\n\r\n" + body


async def serve(client_reader, client_writer):
    try:
        while True:
            request = await client_reader.readuntil(b"\r\n\r\n")
            if not HEALTH.match(request):
                break
            client_writer.write(await health(request))
            await client_writer.drain()
            if (header_value(request, b"connection") or b"").lower() == b"close":
                client_writer.close()
                return
    except (asyncio.IncompleteReadError, ConnectionError, asyncio.LimitOverrunError, OSError):
        client_writer.close()
        return
    try:
        up_reader, up_writer = await asyncio.open_unix_connection(UPSTREAM)
    except OSError:
        client_writer.close()
        return
    up_writer.write(request)
    await asyncio.gather(splice(client_reader, up_writer), splice(up_reader, client_writer))


async def main():
    os.makedirs(os.path.dirname(LISTEN), mode=0o700, exist_ok=True)
    if os.path.exists(LISTEN):
        os.unlink(LISTEN)
    server = await asyncio.start_unix_server(serve, LISTEN, limit=1 << 20)
    stop = asyncio.Event()
    for sig in (signal.SIGTERM, signal.SIGINT):
        asyncio.get_running_loop().add_signal_handler(sig, stop.set)
    async with server:
        await stop.wait()
    os.unlink(LISTEN)


asyncio.run(main())
