//go:build darwin && cgo

#include <errno.h>
#include <fcntl.h>
#include <string.h>
#include <unistd.h>

#include "backend_darwin.h"

static void writeAll(int fd, const void *buf, size_t len) {
	const char *p = buf;
	while (len > 0) {
		ssize_t n = write(fd, p, len);
		if (n < 0 && errno == EINTR) {
			continue;
		}
		if (n <= 0) {
			return;
		}
		p += n;
		len -= (size_t)n;
	}
}

// Each event becomes a record on the pipe: flags, path length, path bytes. The
// queue is serial, so records never interleave.
static void watchStreamCallback(
	ConstFSEventStreamRef stream,
	void *info,
	size_t n,
	void *paths,
	const FSEventStreamEventFlags flags[],
	const FSEventStreamEventId ids[]
) {
	int fd = (int)(intptr_t)info;
	char **ps = paths;
	for (size_t i = 0; i < n; i++) {
		uint32_t hdr[2] = {flags[i], (uint32_t)strlen(ps[i])};
		writeAll(fd, hdr, sizeof hdr);
		writeAll(fd, ps[i], hdr[1]);
	}
}

static void watchNoop(void *ctx) {}

dispatch_queue_t watchQueueCreate(void) {
	return dispatch_queue_create("crowbar.watch", DISPATCH_QUEUE_SERIAL);
}

void watchQueueRelease(dispatch_queue_t queue) {
	dispatch_release(queue);
}

FSEventStreamRef watchStreamStart(
	int fd,
	CFArrayRef paths,
	CFTimeInterval latency,
	dispatch_queue_t queue
) {
	// A write after the reader is gone must fail, not raise SIGPIPE.
	fcntl(fd, F_SETNOSIGPIPE, 1);
	FSEventStreamContext ctx = {0, (void *)(intptr_t)fd, NULL, NULL, NULL};
	FSEventStreamRef stream = FSEventStreamCreate(
		NULL, watchStreamCallback, &ctx, paths, kFSEventStreamEventIdSinceNow, latency,
		kFSEventStreamCreateFlagFileEvents | kFSEventStreamCreateFlagNoDefer);
	if (stream == NULL) {
		return NULL;
	}
	FSEventStreamSetDispatchQueue(stream, queue);
	if (!FSEventStreamStart(stream)) {
		FSEventStreamInvalidate(stream);
		FSEventStreamRelease(stream);
		return NULL;
	}
	return stream;
}

void watchStreamStop(FSEventStreamRef stream, dispatch_queue_t queue) {
	FSEventStreamStop(stream);
	FSEventStreamInvalidate(stream);
	// The queue is serial: once this returns no callback is running or pending.
	dispatch_sync_f(queue, NULL, watchNoop);
	FSEventStreamRelease(stream);
}

CFMutableArrayRef watchPathsCreate(void) {
	return CFArrayCreateMutable(kCFAllocatorDefault, 0, &kCFTypeArrayCallBacks);
}

void watchPathsAppend(CFMutableArrayRef paths, const char *path) {
	CFStringRef str = CFStringCreateWithCString(kCFAllocatorDefault, path, kCFStringEncodingUTF8);
	CFArrayAppendValue(paths, str);
	CFRelease(str);
}

void watchPathsRelease(CFMutableArrayRef paths) {
	CFRelease(paths);
}
