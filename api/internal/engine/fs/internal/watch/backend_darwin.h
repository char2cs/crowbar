#include <CoreServices/CoreServices.h>

dispatch_queue_t watchQueueCreate(void);
void watchQueueRelease(dispatch_queue_t queue);
FSEventStreamRef watchStreamStart(int fd, CFArrayRef paths, CFTimeInterval latency, dispatch_queue_t queue);
void watchStreamStop(FSEventStreamRef stream, dispatch_queue_t queue);
CFMutableArrayRef watchPathsCreate(void);
void watchPathsAppend(CFMutableArrayRef paths, const char *path);
void watchPathsRelease(CFMutableArrayRef paths);
