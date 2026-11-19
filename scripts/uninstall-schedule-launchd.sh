#!/bin/bash
#
# uninstall-schedule-launchd.sh — remove the login scheduler agent on macOS.
#
# Boots com.goravel.workflow.schedule out of the GUI domain and deletes
# ~/Library/LaunchAgents/com.goravel.workflow.schedule.plist, undoing what
# scripts/setup-macos.sh installs. Safe when nothing is installed: every step
# reports what it found and the exit status is 0. The log at
# storage/logs/launchd-schedule.log is left behind as history.
#
# macOS only, non-interactive, macOS /bin/bash 3.2: no associative arrays, no
# mapfile, no ${var,,}, no sort -V, no jq.
#
# Usage: scripts/uninstall-schedule-launchd.sh [--dry-run] [-h|--help]

set -uo pipefail

REPO_ROOT=$(cd "$(dirname "$0")/.." && pwd)
readonly REPO_ROOT

readonly LABEL="com.goravel.workflow.schedule"
readonly PLIST="$HOME/Library/LaunchAgents/$LABEL.plist"
readonly LOG="$REPO_ROOT/storage/logs/launchd-schedule.log"

DRY_RUN=0
FAILED=0
CHANGES=0
HAVES=0
CANTS=0
CHANGES_TEXT=""

usage() {
  cat <<'EOF'
Remove the login scheduler agent this checkout installed.

Boots com.goravel.workflow.schedule out of the GUI domain and deletes its plist
in ~/Library/LaunchAgents, so ./artisan schedule:run no longer starts at login.
Nothing else is touched: .env and storage/logs/launchd-schedule.log stay.

Options:
  --dry-run       print what would change, write nothing
  -h, --help      show this help

Exit status: 0 done (or nothing was installed), 1 a check failed, 2 unknown
argument.
EOF
}

section() { printf '\n==> %s\n' "$*"; }
line()    { printf '  %-6s %-24s %s\n' "$1" "$2" "$3"; }

# The six statuses, defined once. Every line the script prints goes through
# line(), so the columns never drift.
legend() {
  section "Legend"
  line have "already in place"     "the detail is the evidence that was checked"
  line todo "would change"         "dry run only"
  line did  "changed"              "real run only"
  line note "information"          "nothing to do"
  line cant "could not be checked" "the detail says why"
  line FAIL "a check failed"       "a real run exits 1"
}

# have counts as satisfied; show repeats a have line without counting, so the
# Verification pass cannot double the totals of the steps before it.
have() { HAVES=$((HAVES + 1)); line have "$1" "$2"; }
show() { line have "$1" "$2"; }
todo() {
  CHANGES=$((CHANGES + 1))
  CHANGES_TEXT="$CHANGES_TEXT$1: $2"$'\n'
  line todo "$1" "$2"
}
did() {
  CHANGES=$((CHANGES + 1))
  CHANGES_TEXT="$CHANGES_TEXT$1: $2"$'\n'
  line did "$1" "$2"
}
note() { line note "$1" "$2"; }
cant() { CANTS=$((CANTS + 1)); line cant "$1" "$2"; }
fail() { FAILED=$((FAILED + 1)); line FAIL "$1" "${2:-failed}" >&2; }

domain() { printf 'gui/%s' "$(id -u)"; }
agent_loaded() { launchctl print "$(domain)/$LABEL" >/dev/null 2>&1; }

preconditions() {
  section "Preconditions"

  local uname_s
  uname_s=$(uname -s)
  if [ "$uname_s" != "Darwin" ]; then
    cant "macOS only" "this host reports $uname_s"
    exit 1
  fi
  have "macOS" "$(uname -m)"
}

stop_agent() {
  section "launchd agent"

  local target out
  target="$(domain)/$LABEL"

  if ! agent_loaded; then
    have "launchd agent" "$target is not loaded"
    return 0
  fi

  if [ "$DRY_RUN" -eq 1 ]; then
    todo "launchd agent" "would bootout $target"
    return 0
  fi

  # bootout takes the job's process group with it (the plist sets no
  # AbandonProcessGroup), so nothing is orphaned, but a poll in flight is cut
  # mid-tick instead of draining. ExitTimeOut (120 s in the plist) only bounds the
  # wait before launchd sends KILL.
  if out=$(launchctl bootout "$target" 2>&1); then
    did "launchd agent" "booted out $target"
  else
    fail "launchd agent" "launchctl bootout failed: $(printf '%s' "$out" | head -1)"
  fi
}

remove_plist() {
  section "LaunchAgent plist"

  if [ ! -f "$PLIST" ]; then
    have "plist" "no $PLIST"
    return 0
  fi

  if [ "$DRY_RUN" -eq 1 ]; then
    todo "plist" "would remove $PLIST"
    return 0
  fi

  rm -f "$PLIST" || { fail "plist" "removing $PLIST failed"; return 0; }
  did "plist" "removed $PLIST"
}

verify() {
  section "Verification"

  local target
  target="$(domain)/$LABEL"

  if agent_loaded; then
    fail "launchd agent" "$target is still loaded"
  else
    show "launchd agent" "$target not loaded"
  fi

  if [ -f "$PLIST" ]; then
    fail "plist" "$PLIST is still there"
  else
    show "plist" "gone"
  fi

  # launchd takes the job's whole process group, so the agent leaves no scheduler
  # behind. A match here belongs to someone else — normally a ./artisan
  # schedule:run started in a terminal — and this script does not touch it.
  local manual
  if manual=$(pgrep -f "artisan schedule:run" 2>/dev/null | tr '\n' ' '); then
    note "other scheduler" "pids ${manual% } run schedule:run outside this job, left alone"
  fi

  if [ -f "$LOG" ]; then
    note "launchd log" "$LOG kept as history"
  fi
}

result() {
  section "Result"
  local n=0 entry

  if [ "$CHANGES" -gt 0 ]; then
    if [ "$DRY_RUN" -eq 1 ]; then
      printf '  %s change(s) would be made:\n' "$CHANGES"
    else
      printf '  %s change(s) made:\n' "$CHANGES"
    fi
    # Subshell is fine: the numbering is printed here and never read again.
    printf '%s' "$CHANGES_TEXT" | while IFS= read -r entry; do
      n=$((n + 1))
      printf '    %s. %s\n' "$n" "$entry"
    done
  fi

  if [ "$CANTS" -gt 0 ]; then
    printf '  %s check(s) already satisfied, %s could not be checked here\n' "$HAVES" "$CANTS"
  else
    printf '  %s check(s) already satisfied\n' "$HAVES"
  fi
}

main() {
  while [ $# -gt 0 ]; do
    case "$1" in
      --dry-run) DRY_RUN=1 ;;
      -h|--help) usage; exit 0 ;;
      *)
        printf 'uninstall-schedule-launchd.sh: unknown argument: %s\n\n' "$1" >&2
        usage >&2
        exit 2
        ;;
    esac
    shift
  done

  legend
  preconditions
  stop_agent
  remove_plist

  if [ "$DRY_RUN" -eq 1 ]; then
    result
    printf '\n  Nothing was removed. Re-run without --dry-run to apply.\n'
    exit 0
  fi

  verify
  result

  if [ "$FAILED" -gt 0 ]; then
    printf '\n%s check(s) failed.\n' "$FAILED" >&2
    exit 1
  fi

  printf '\nDone. The scheduler starts again from ./artisan, not at login.\n'
}

main "$@"
