// FSEvents observer for the meter: prints one line per file event under the
// given roots (ts_ms \t flags \t path) until killed. Sees writes by processes
// the interposer cannot load into (git via the xcrun shim, SIP binaries).

#include <CoreServices/CoreServices.h>
#include <stdio.h>
#include <sys/time.h>

static void on_events(ConstFSEventStreamRef stream, void *info, size_t count, void *paths,
                      const FSEventStreamEventFlags flags[], const FSEventStreamEventId ids[]) {
  (void)stream, (void)info, (void)ids;
  struct timeval tv;
  gettimeofday(&tv, NULL);
  long long ts = (long long)tv.tv_sec * 1000 + tv.tv_usec / 1000;
  for (size_t i = 0; i < count; i++)
    printf("%lld\t%u\t%s\n", ts, (unsigned)flags[i], ((char **)paths)[i]);
  fflush(stdout);
}

int main(int argc, char **argv) {
  if (argc < 2) {
    fprintf(stderr, "usage: %s <root>...\n", argv[0]);
    return 2;
  }
  CFMutableArrayRef roots = CFArrayCreateMutable(NULL, argc - 1, &kCFTypeArrayCallBacks);
  for (int i = 1; i < argc; i++) {
    CFStringRef root = CFStringCreateWithCString(NULL, argv[i], kCFStringEncodingUTF8);
    CFArrayAppendValue(roots, root);
    CFRelease(root);
  }
  FSEventStreamRef stream = FSEventStreamCreate(
      NULL, on_events, NULL, roots, kFSEventStreamEventIdSinceNow, 0.05,
      kFSEventStreamCreateFlagFileEvents | kFSEventStreamCreateFlagNoDefer);
  dispatch_queue_t queue = dispatch_queue_create("meter.fsevents", NULL);
  FSEventStreamSetDispatchQueue(stream, queue);
  if (!FSEventStreamStart(stream)) {
    fprintf(stderr, "FSEventStreamStart failed\n");
    return 1;
  }
  printf("ready\n");
  fflush(stdout);
  dispatch_main();
}
