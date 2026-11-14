// PATH counting shim for the meter: logs its argv and parent, then execs the
// binary the same name resolves to on PATH with the shim dir left out.
// Installed as symlinks named git, gh, ... in METER_SHIM_DIR.

#include <limits.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/time.h>
#include <unistd.h>

int main(int argc, char **argv) {
  (void)argc;
  const char *dir = getenv("METER_SHIM_DIR");
  const char *path = getenv("PATH");
  const char *base = strrchr(argv[0], '/');
  base = base ? base + 1 : argv[0];
  if (!dir || !path) {
    fprintf(stderr, "meter shim %s: METER_SHIM_DIR or PATH unset\n", base);
    return 127;
  }

  const char *log = getenv("METER_SHIM_LOG");
  // Only the metered tree is counted, not the build tools sharing its PATH.
  FILE *f = log && getenv("METER_TRACE") ? fopen(log, "a") : NULL;
  if (f) {
    struct timeval tv;
    gettimeofday(&tv, NULL);
    fprintf(f, "%lld\t%d\t%d\t%s\t", (long long)tv.tv_sec * 1000 + tv.tv_usec / 1000, getpid(),
            getppid(), base);
    for (int i = 1; argv[i]; i++) fprintf(f, i > 1 ? " %s" : "%s", argv[i]);
    fputc('\n', f);
    fclose(f);
  }

  char *paths = strdup(path);
  for (char *entry = strtok(paths, ":"); entry; entry = strtok(NULL, ":")) {
    if (strcmp(entry, dir) == 0) continue;
    char candidate[PATH_MAX];
    snprintf(candidate, sizeof candidate, "%s/%s", entry, base);
    if (access(candidate, X_OK) == 0) {
      execv(candidate, argv);
      perror(candidate);
      return 126;
    }
  }
  fprintf(stderr, "meter shim: %s not found on PATH\n", base);
  return 127;
}
