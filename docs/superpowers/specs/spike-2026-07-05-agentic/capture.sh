#!/bin/sh
# Crowbar Phase-0 spike: dump each hook's stdin payload verbatim, labelled + timestamped.
LOG="/private/tmp/claude-501/-Users-char2cs--crowbar-projects-71244879-4ed1-416e-a6b4-60eeac355663-9e6b3e9c-0f25-47f4-989f-45c922542272-workspaces-43e4091a-26c2-4c70-ad4a-833e690443a0-worktree/ddef59a4-f47f-4fdc-ad7c-b2e61a68c3af/scratchpad/spike/hooks.log"
{
  printf '===== EVENT=%s ts=%s pid=%s =====\n' "$1" "$(date +%s)" "$$"
  cat
  printf '\n'
} >> "$LOG"
