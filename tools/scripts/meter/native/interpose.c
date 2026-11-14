// DYLD_INSERT_LIBRARIES interposer for the meter (see ../README.md).
//
// Loaded into every process of the metered tree; it logs only in "traced"
// processes: one whose executable is named in METER_ROOTS, or one that
// inherited METER_TRACE from a traced parent. A traced process re-injects the
// meter's variables into every child it starts, because Go builds a child's
// environment from its own copy (often an explicit cmd.Env).
//
// Every log write is a single write(2) of a stack buffer: the exec hooks run
// in a forked Go child, where malloc and stdio are unsafe.

#include <dlfcn.h>
#include <errno.h>
#include <fcntl.h>
#include <limits.h>
#include <mach-o/dyld.h>
#include <spawn.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <sys/time.h>
#include <unistd.h>

extern char **environ;

#define INTERPOSE(replacement, replacee)                                                   \
  __attribute__((used)) static struct {                                                    \
    const void *r;                                                                         \
    const void *e;                                                                         \
  } interpose_##replacee __attribute__((section("__DATA,__interpose"))) = {               \
      (const void *)(unsigned long)&replacement, (const void *)(unsigned long)&replacee}

#define LINE_MAX_BYTES 4096
#define ENV_MAX 1024

static int log_fd = -1;
static int traced = 0;
static char log_path[PATH_MAX];
static char env_log[PATH_MAX + 16];
static char env_dylib[PATH_MAX + 32];
static char env_meter_dylib[PATH_MAX + 16];
static char env_roots[1024];

typedef struct {
  char b[LINE_MAX_BYTES];
  size_t n;
} line_t;

static void put(line_t *l, const char *s) {
  if (!s) s = "(null)";
  for (; *s && l->n < LINE_MAX_BYTES - 2; s++) {
    char c = *s;
    l->b[l->n++] = (c == '\t' || c == '\n') ? ' ' : c;
  }
}

static void putc_(line_t *l, char c) {
  if (l->n < LINE_MAX_BYTES - 2) l->b[l->n++] = c;
}

static void putnum(line_t *l, long long v) {
  char tmp[24];
  int i = 0;
  int neg = v < 0;
  unsigned long long u = neg ? (unsigned long long)(-v) : (unsigned long long)v;
  do {
    tmp[i++] = (char)('0' + u % 10);
    u /= 10;
  } while (u && i < 22);
  if (neg) putc_(l, '-');
  while (i) putc_(l, tmp[--i]);
}

// ts \t pid \t ppid \t call \t result \t flags \t path [\t extra]
// extra: a rename's source; an exec's argv, then its cwd.
static void begin(line_t *l, const char *call) {
  struct timeval tv;
  gettimeofday(&tv, NULL);
  l->n = 0;
  putnum(l, (long long)tv.tv_sec * 1000 + tv.tv_usec / 1000);
  putc_(l, '\t');
  putnum(l, getpid());
  putc_(l, '\t');
  putnum(l, getppid());
  putc_(l, '\t');
  put(l, call);
}

static void field_num(line_t *l, long long v) {
  putc_(l, '\t');
  putnum(l, v);
}

static void field(line_t *l, const char *s) {
  putc_(l, '\t');
  put(l, s);
}

static void emit(line_t *l) {
  if (log_fd < 0) return;
  l->b[l->n++] = '\n';
  write(log_fd, l->b, l->n);
}

// A path relative to a directory fd is logged absolute, so it can be grouped.
static void field_at(line_t *l, int dirfd, const char *path) {
  putc_(l, '\t');
  if (path && path[0] != '/') {
    char dir[PATH_MAX];
    int ok = dirfd == AT_FDCWD ? getcwd(dir, sizeof dir) != NULL : fcntl(dirfd, F_GETPATH, dir) == 0;
    if (ok) {
      put(l, dir);
      putc_(l, '/');
    }
  }
  put(l, path);
}

static int is_root(const char *exe) {
  const char *base = strrchr(exe, '/');
  base = base ? base + 1 : exe;
  size_t len = strlen(base);
  for (const char *p = env_roots; *p;) {
    const char *end = strchr(p, ',');
    size_t n = end ? (size_t)(end - p) : strlen(p);
    if (n == len && strncmp(p, base, n) == 0) return 1;
    if (!end) break;
    p = end + 1;
  }
  return 0;
}

__attribute__((constructor)) static void meter_init(void) {
  const char *path = getenv("METER_LOG");
  const char *dylib = getenv("METER_DYLIB");
  const char *roots = getenv("METER_ROOTS");
  if (!path || !dylib) return;
  snprintf(log_path, sizeof log_path, "%s", path);
  snprintf(env_log, sizeof env_log, "METER_LOG=%s", path);
  snprintf(env_dylib, sizeof env_dylib, "DYLD_INSERT_LIBRARIES=%s", dylib);
  snprintf(env_meter_dylib, sizeof env_meter_dylib, "METER_DYLIB=%s", dylib);
  snprintf(env_roots, sizeof env_roots, "%s", roots ? roots : "");

  char exe[PATH_MAX];
  uint32_t size = sizeof exe;
  if (_NSGetExecutablePath(exe, &size) != 0) exe[0] = 0;
  traced = getenv("METER_TRACE") != NULL || is_root(exe);
  if (!traced) return;
  setenv("METER_TRACE", "1", 1);

  int fd = open(log_path, O_WRONLY | O_APPEND | O_CREAT | O_CLOEXEC, 0644);
  if (fd < 0) return;
  // Out of the low range a child's dup2 of its stdio and extra files lands in.
  int high = fcntl(fd, F_DUPFD_CLOEXEC, 512);
  if (high >= 0) {
    close(fd);
    fd = high;
  }
  log_fd = fd;
  line_t l;
  begin(&l, "load");
  field_num(&l, 0);
  field_num(&l, 0);
  field(&l, exe);
  emit(&l);
}

static int has_prefix(const char *s, const char *prefix) {
  return strncmp(s, prefix, strlen(prefix)) == 0;
}

static int meter_var(const char *s) {
  return has_prefix(s, "DYLD_INSERT_LIBRARIES=") || has_prefix(s, "METER_LOG=") ||
         has_prefix(s, "METER_DYLIB=") || has_prefix(s, "METER_TRACE=");
}

// The child's environment: the caller's, with the meter's variables ensured.
static char *const *child_env(char *const *envp, char **out) {
  if (!traced) return envp;
  if (!envp) envp = environ;
  size_t n = 0;
  for (char *const *e = envp; *e && n < ENV_MAX - 5; e++)
    if (!meter_var(*e)) out[n++] = *e;
  out[n++] = env_dylib;
  out[n++] = env_log;
  out[n++] = env_meter_dylib;
  out[n++] = "METER_TRACE=1";
  out[n] = NULL;
  return out;
}

static void log_exec(const char *call, long long result, const char *path, char *const argv[]) {
  if (!traced) return;
  line_t l;
  begin(&l, call);
  field_num(&l, result);
  field_num(&l, 0);
  field(&l, path);
  putc_(&l, '\t');
  for (int i = 0; argv && argv[i] && l.n < LINE_MAX_BYTES - PATH_MAX - 64; i++) {
    if (i) putc_(&l, ' ');
    put(&l, argv[i]);
  }
  // A Go child has already chdir'd to cmd.Dir here: it names the worktree.
  char cwd[PATH_MAX];
  field(&l, getcwd(cwd, sizeof cwd) ? cwd : "");
  emit(&l);
}

static void log_path_call(const char *call, long long result, int err, long long flags, int dirfd,
                          const char *path, const char *path2) {
  if (!traced || log_fd < 0) return;
  line_t l;
  begin(&l, call);
  field_num(&l, result < 0 ? -err : result);
  field_num(&l, flags);
  field_at(&l, dirfd, path);
  if (path2) field(&l, path2);
  emit(&l);
}

int meter_open(const char *path, int flags, ...) {
  mode_t mode = 0;
  if (flags & O_CREAT) {
    va_list ap;
    va_start(ap, flags);
    mode = (mode_t)va_arg(ap, int);
    va_end(ap);
  }
  int r = open(path, flags, mode);
  int err = errno;
  log_path_call("open", r, err, flags, AT_FDCWD, path, NULL);
  errno = err;
  return r;
}
INTERPOSE(meter_open, open);

int meter_openat(int dirfd, const char *path, int flags, ...) {
  mode_t mode = 0;
  if (flags & O_CREAT) {
    va_list ap;
    va_start(ap, flags);
    mode = (mode_t)va_arg(ap, int);
    va_end(ap);
  }
  int r = openat(dirfd, path, flags, mode);
  int err = errno;
  log_path_call("open", r, err, flags, dirfd, path, NULL);
  errno = err;
  return r;
}
INTERPOSE(meter_openat, openat);

int meter_creat(const char *path, mode_t mode) {
  int r = creat(path, mode);
  int err = errno;
  log_path_call("open", r, err, O_CREAT | O_WRONLY | O_TRUNC, AT_FDCWD, path, NULL);
  errno = err;
  return r;
}
INTERPOSE(meter_creat, creat);

int meter_rename(const char *from, const char *to) {
  int r = rename(from, to);
  int err = errno;
  log_path_call("rename", r, err, 0, AT_FDCWD, to, from);
  errno = err;
  return r;
}
INTERPOSE(meter_rename, rename);

int meter_renameat(int fromfd, const char *from, int tofd, const char *to) {
  int r = renameat(fromfd, from, tofd, to);
  int err = errno;
  log_path_call("rename", r, err, 0, tofd, to, from);
  errno = err;
  return r;
}
INTERPOSE(meter_renameat, renameat);

int meter_unlink(const char *path) {
  int r = unlink(path);
  int err = errno;
  log_path_call("unlink", r, err, 0, AT_FDCWD, path, NULL);
  errno = err;
  return r;
}
INTERPOSE(meter_unlink, unlink);

int meter_unlinkat(int dirfd, const char *path, int flag) {
  int r = unlinkat(dirfd, path, flag);
  int err = errno;
  log_path_call("unlink", r, err, flag, dirfd, path, NULL);
  errno = err;
  return r;
}
INTERPOSE(meter_unlinkat, unlinkat);

int meter_mkdir(const char *path, mode_t mode) {
  int r = mkdir(path, mode);
  int err = errno;
  log_path_call("mkdir", r, err, 0, AT_FDCWD, path, NULL);
  errno = err;
  return r;
}
INTERPOSE(meter_mkdir, mkdir);

int meter_mkdirat(int dirfd, const char *path, mode_t mode) {
  int r = mkdirat(dirfd, path, mode);
  int err = errno;
  log_path_call("mkdir", r, err, 0, dirfd, path, NULL);
  errno = err;
  return r;
}
INTERPOSE(meter_mkdirat, mkdirat);

int meter_execve(const char *path, char *const argv[], char *const envp[]) {
  char *env[ENV_MAX];
  log_exec("execve", 0, path, argv);
  return execve(path, argv, child_env(envp, env));
}
INTERPOSE(meter_execve, execve);

// libc's posix_spawnp resolves the file and calls posix_spawn, which is
// interposed too: the inner call logs the resolved path, once.
static __thread int spawn_logged;

int meter_posix_spawn(pid_t *pid, const char *path, const posix_spawn_file_actions_t *actions,
                      const posix_spawnattr_t *attr, char *const argv[], char *const envp[]) {
  char *env[ENV_MAX];
  pid_t child = 0;
  int r = posix_spawn(&child, path, actions, attr, argv, child_env(envp, env));
  if (pid) *pid = child;
  log_exec("spawn", r == 0 ? child : -r, path, argv);
  spawn_logged = 1;
  return r;
}
INTERPOSE(meter_posix_spawn, posix_spawn);

int meter_posix_spawnp(pid_t *pid, const char *file, const posix_spawn_file_actions_t *actions,
                       const posix_spawnattr_t *attr, char *const argv[], char *const envp[]) {
  char *env[ENV_MAX];
  pid_t child = 0;
  spawn_logged = 0;
  int r = posix_spawnp(&child, file, actions, attr, argv, child_env(envp, env));
  if (pid) *pid = child;
  if (!spawn_logged) log_exec("spawn", r == 0 ? child : -r, file, argv);
  return r;
}
INTERPOSE(meter_posix_spawnp, posix_spawnp);
