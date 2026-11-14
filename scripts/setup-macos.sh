#!/bin/bash
#
# setup.sh — prepare this Goravel workflow checkout on macOS.
#
# Installs the external services the app and its agent tooling reach for, then
# writes the shared config files only when a key, a folder or a project is
# missing. It creates no database/ and runs no migrate; artisan writes only
# its log under storage/logs.
#
#   gh        GitHub CLI, for the release commands and the mention poller
#   uv/uvx    the code-review-graph MCP server runs as ["uvx", "code-review-graph", "serve"]
#   opencode  the V2 installer, into ~/.opencode/bin
#   paseo     the cask, its daemon, and the project AGENT_PROJECT_NAME names
#   config    ~/.config/opencode: the MCP entry plus agents/ and skills/
#   repo      .env with an APP_KEY
#
# Prerequisites this script checks but never installs: macOS, the Xcode Command
# Line Tools and Homebrew (gh, uv, opencode and the paseo cask all come from
# it). Go 1.26+ from go.mod is required too, but only ./artisan key:generate
# and go run . need it, so it shows up there rather than up front.
#
# macOS only, non-interactive, macOS /bin/bash 3.2: no associative arrays, no
# mapfile, no ${var,,}, no sort -V, no jq.
#
# Usage: scripts/setup.sh [--dry-run] [-h|--help]

set -uo pipefail

REPO_ROOT=$(cd "$(dirname "$0")/.." && pwd)
readonly REPO_ROOT

readonly OPENCODE_BIN="$HOME/.opencode/bin"
readonly OPENCODE_CONFIG="$HOME/.config/opencode/opencode.json"
readonly OPENCODE_AGENTS="$HOME/.config/opencode/agents"
readonly OPENCODE_SKILLS="$HOME/.config/opencode/skills"
readonly PASEO_APP="/Applications/Paseo.app"
readonly PASEO_BIN_IN_APP="$PASEO_APP/Contents/Resources/bin/paseo"
readonly LOCAL_BIN="$HOME/.local/bin"
readonly AGENTS_REPO="goravel/agents"
readonly SKILLS_REPO="cxuu/golang-skills"
readonly DAEMON_WAIT=60

DRY_RUN=0
FAILED=0
CHANGES=0
HAVES=0
CANTS=0
CHANGES_TEXT=""

usage() {
  cat <<'EOF'
Prepare this checkout on macOS.

Installs gh, uv/uvx, opencode and paseo, then writes the ~/.config/opencode
entries (code-review-graph MCP server, agents, skills), .env with an APP_KEY,
and the paseo project AGENT_PROJECT_NAME names.

Requires: macOS, the Xcode Command Line Tools, Homebrew and Go 1.26+ (from
go.mod). None of them is installed here: gh, uv, opencode and the paseo cask
come from Homebrew, and ./artisan key:generate needs Go on PATH.

Every step resolves itself — it reports the evidence that its target is already
there, or names the one change it would make. Inside the repo the only writes
are .env and artisan's own log.

Options:
  --dry-run       print what would change, write nothing
  -h, --help      show this help

Exit status: 0 done, 1 a check failed, 2 unknown argument.
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

# run executes an argv command. Under --dry-run every step resolves and returns
# before reaching this point, so a call here means the preview missed a change.
run() {
  if [ "$DRY_RUN" -eq 1 ]; then
    cant "unplanned change" "$*"
    return 0
  fi
  "$@"
}

# sh_run executes a shell string, with the same dry-run guard.
sh_run() {
  if [ "$DRY_RUN" -eq 1 ]; then
    cant "unplanned change" "$1"
    return 0
  fi
  /bin/bash -c "$1"
}

# env_value prints one key from .env, without its quotes.
env_value() {
  [ -f .env ] || return 0
  sed -n "s/^$1=//p" .env | head -1 | sed -e 's/^"//' -e 's/"$//'
}

paseo_daemon_running() {
  paseo daemon status 2>/dev/null | grep -q 'localDaemon: running'
}

# paseo_project_path prints the path of the paseo project named by $1.
paseo_project_path() {
  paseo project ls --json 2>/dev/null | python3 -c '
import json, sys
name = sys.argv[1]
try:
    data = json.load(sys.stdin)
except Exception:
    sys.exit(1)
for project in data:
    if project.get("name") == name:
        print(project.get("path", ""))
        sys.exit(0)
sys.exit(1)
' "$1"
}

preconditions() {
  section "Preconditions"

  local uname_s
  uname_s=$(uname -s)
  if [ "$uname_s" != "Darwin" ]; then
    cant "macOS only" "this host reports $uname_s"
    exit 1
  fi
  have "macOS" "$(uname -m)"

  if ! xcode-select -p >/dev/null 2>&1; then
    cant "Command Line Tools" "missing, run xcode-select --install first"
    exit 1
  fi
  have "Command Line Tools" "$(xcode-select -p)"

  local tool missing=""
  for tool in curl python3 git; do
    command -v "$tool" >/dev/null 2>&1 || missing="$missing $tool"
  done
  if [ -n "$missing" ]; then
    cant "curl, python3, git" "missing:$missing"
    exit 1
  fi
  have "curl, python3, git" "on PATH"

  cd "$REPO_ROOT" || exit 1
  if [ ! -f .env.example ]; then
    cant "checkout" "$REPO_ROOT has no .env.example"
    exit 1
  fi
  have "checkout" "$REPO_ROOT"

  if command -v brew >/dev/null 2>&1; then
    have "Homebrew" "brew $(brew --version | head -1 | awk '{print $2}') at $(command -v brew)"
  else
    cant "Homebrew" "not on PATH, so gh, uv, opencode and the paseo cask cannot be installed here"
  fi
}

ensure_gh() {
  section "GitHub CLI"

  if command -v gh >/dev/null 2>&1; then
    have "gh" "$(gh --version | head -1 | awk '{print $3}') at $(command -v gh)"
    return 0
  fi

  if [ "$DRY_RUN" -eq 1 ]; then
    todo "gh" "would install gh (brew install gh)"
    return 0
  fi

  run brew install gh || { fail "gh" "brew install gh failed"; return 0; }
  if command -v gh >/dev/null 2>&1; then
    did "gh" "installed by brew install gh"
  else
    fail "gh" "brew install gh finished but gh is still not on PATH"
  fi
}

ensure_uv() {
  section "uv / uvx (code-review-graph MCP server)"

  if command -v uvx >/dev/null 2>&1; then
    have "uv / uvx" "$(command -v uvx)"
    return 0
  fi

  if [ "$DRY_RUN" -eq 1 ]; then
    todo "uv / uvx" "would install uv (brew install uv)"
    return 0
  fi

  run brew install uv || { fail "uv / uvx" "brew install uv failed"; return 0; }
  if command -v uvx >/dev/null 2>&1; then
    did "uv / uvx" "installed by brew install uv"
  else
    fail "uv / uvx" "brew install uv finished but uvx is still not on PATH"
  fi
}

ensure_opencode() {
  section "opencode (V2)"

  PATH="$OPENCODE_BIN:$PATH"
  export PATH

  if command -v opencode >/dev/null 2>&1; then
    have "opencode" "$(opencode --version 2>/dev/null | head -1 | awk '{print $NF}') at $(command -v opencode)"
    return 0
  fi

  if [ "$DRY_RUN" -eq 1 ]; then
    todo "opencode" "would run the V2 installer into $OPENCODE_BIN"
    return 0
  fi

  sh_run 'curl -fsSL https://opencode.ai/v2/install | bash' \
    || { fail "opencode" "the V2 installer failed"; return 0; }
  if command -v opencode >/dev/null 2>&1; then
    did "opencode" "installed into $OPENCODE_BIN"
  else
    fail "opencode" "the installer finished but opencode is still not on PATH"
  fi
}

ensure_paseo() {
  section "paseo"

  if brew list --cask paseo >/dev/null 2>&1; then
    have "paseo cask" "installed as a Homebrew cask"
  elif [ -d "$PASEO_APP" ]; then
    have "paseo app" "Paseo.app in /Applications"
  elif [ "$DRY_RUN" -eq 1 ]; then
    todo "paseo cask" "would install paseo (brew install --cask paseo)"
  else
    run brew install --cask paseo \
      || { fail "paseo cask" "brew install --cask paseo failed"; return 0; }
    if [ -d "$PASEO_APP" ]; then
      did "paseo cask" "installed paseo (brew install --cask paseo)"
    else
      fail "paseo cask" "installed but $PASEO_APP is still missing"
    fi
  fi

  if command -v paseo >/dev/null 2>&1; then
    have "paseo on PATH" "$(command -v paseo) -> $(paseo --version 2>/dev/null | head -1)"
    return 0
  fi

  if [ "$DRY_RUN" -eq 1 ]; then
    todo "paseo on PATH" "would link $LOCAL_BIN/paseo -> $PASEO_BIN_IN_APP"
    return 0
  fi

  # app/services/agent/paseo.go runs `env -i PATH=$PATH HOME=$HOME paseo ...`,
  # so the binary must be on the human's PATH, not only inside the .app.
  if [ ! -x "$PASEO_BIN_IN_APP" ]; then
    fail "paseo on PATH" "no binary at $PASEO_BIN_IN_APP and no paseo on PATH"
    return 0
  fi
  mkdir -p "$LOCAL_BIN"
  ln -sfn "$PASEO_BIN_IN_APP" "$LOCAL_BIN/paseo"
  PATH="$LOCAL_BIN:$PATH"
  export PATH
  if command -v paseo >/dev/null 2>&1; then
    did "paseo on PATH" "linked $LOCAL_BIN/paseo -> $PASEO_BIN_IN_APP"
  else
    fail "paseo on PATH" "$LOCAL_BIN is not on PATH, so paseo stays unreachable"
  fi
}

ensure_paseo_daemon() {
  section "paseo daemon"

  if paseo_daemon_running; then
    have "paseo daemon" "daemon already running"
    return 0
  fi

  if [ "$DRY_RUN" -eq 1 ]; then
    todo "paseo daemon" "would open -a Paseo and wait up to ${DAEMON_WAIT}s for the daemon"
    return 0
  fi

  open -a Paseo || { fail "paseo daemon" "open $PASEO_APP failed"; return 0; }
  printf '  waiting up to %ss for the daemon\n' "$DAEMON_WAIT"

  local waited=0
  while [ "$waited" -lt "$DAEMON_WAIT" ]; do
    sleep 2
    waited=$((waited + 2))
    if paseo_daemon_running; then
      did "paseo daemon" "running after ${waited}s"
      return 0
    fi
  done

  fail "paseo daemon" "no answer in ${DAEMON_WAIT}s, open Paseo.app once"
}

# mcp_entry <check|merge> prints present|absent for check and present|merged
# for merge, and exits 1 with a reason when the file cannot be read or is not
# the expected shape.
mcp_entry() {
  python3 - "$OPENCODE_CONFIG" "$1" <<'PY'
import json, os, shutil, sys, tempfile

path, mode = sys.argv[1], sys.argv[2]
entry = {"type": "local", "command": ["uvx", "code-review-graph", "serve"]}

try:
    with open(path) as handle:
        data = json.load(handle)
except FileNotFoundError:
    data = {"$schema": "https://opencode.ai/config.json"}
except json.JSONDecodeError as exc:
    sys.exit("invalid JSON: %s" % exc)

if not isinstance(data, dict):
    sys.exit("not a JSON object")

mcp = data.setdefault("mcp", {})
if not isinstance(mcp, dict):
    sys.exit("mcp is not an object")

servers = mcp.setdefault("servers", {})
if not isinstance(servers, dict):
    sys.exit("mcp.servers is not an object")

if "code-review-graph" in servers:
    print("present")
    sys.exit(0)

if mode == "check":
    print("absent")
    sys.exit(0)

servers["code-review-graph"] = entry

os.makedirs(os.path.dirname(path), exist_ok=True)
if os.path.exists(path):
    shutil.copy2(path, path + ".bak")

fd, tmp = tempfile.mkstemp(dir=os.path.dirname(path), prefix=".opencode.json.")
with os.fdopen(fd, "w") as handle:
    json.dump(data, handle, indent=2, ensure_ascii=False)
    handle.write("\n")
if os.path.exists(path):
    shutil.copymode(path, tmp)
else:
    os.chmod(tmp, 0o644)
os.replace(tmp, path)
print("merged")
PY
}

ensure_mcp_entry() {
  section "opencode config: code-review-graph MCP server"

  local state
  state=$(mcp_entry check) \
    || { fail "code-review-graph MCP" "could not read $OPENCODE_CONFIG (invalid JSON or unexpected shape)"; return 0; }

  if [ "$state" = "present" ]; then
    have "code-review-graph MCP" "mcp.servers[\"code-review-graph\"] in $OPENCODE_CONFIG"
    return 0
  fi

  if [ "$DRY_RUN" -eq 1 ]; then
    todo "code-review-graph MCP" "would add mcp.servers[\"code-review-graph\"] to $OPENCODE_CONFIG"
    return 0
  fi

  state=$(mcp_entry merge) \
    || { fail "code-review-graph MCP" "could not merge into $OPENCODE_CONFIG, left untouched"; return 0; }

  case "$state" in
    merged) did "code-review-graph MCP" "added mcp.servers[\"code-review-graph\"] to $OPENCODE_CONFIG (backup: $OPENCODE_CONFIG.bak)" ;;
    *)      fail "code-review-graph MCP" "unexpected merge result: $state" ;;
  esac
}

ensure_agents_and_skills() {
  section "agents and skills (~/.config/opencode)"

  local need_agents=0 need_skills=0
  [ -f "$OPENCODE_AGENTS/ship.md" ] || need_agents=1
  [ -f "$OPENCODE_SKILLS/goravel-planning/SKILL.md" ] || need_skills=1

  if [ "$need_agents" -eq 0 ] && [ "$need_skills" -eq 0 ]; then
    have "agents and skills" "ship.md and goravel-planning/SKILL.md already in ~/.config/opencode"
    return 0
  fi

  # The real run clones both repos and copies both trees whenever either marker
  # is missing, so the preview names both copies.
  if [ "$DRY_RUN" -eq 1 ]; then
    todo "agents" "would clone $AGENTS_REPO and copy agents/ -> $OPENCODE_AGENTS"
    todo "skills" "would clone $AGENTS_REPO and $SKILLS_REPO and copy skills/ -> $OPENCODE_SKILLS"
    return 0
  fi

  local tmp
  tmp=$(mktemp -d)

  if git clone --depth 1 -q "https://github.com/$AGENTS_REPO.git" "$tmp/agents"; then
    mkdir -p "$OPENCODE_AGENTS" "$OPENCODE_SKILLS"
    if rsync -a "$tmp/agents/agents/" "$OPENCODE_AGENTS/" \
      && rsync -a "$tmp/agents/skills/" "$OPENCODE_SKILLS/"; then
      did "agents" "copied from $AGENTS_REPO"
    else
      fail "agents" "copy from $AGENTS_REPO failed"
    fi
  else
    fail "agents" "clone $AGENTS_REPO failed"
  fi

  if git clone --depth 1 -q "https://github.com/$SKILLS_REPO.git" "$tmp/skills"; then
    mkdir -p "$OPENCODE_SKILLS"
    if rsync -a "$tmp/skills/skills/" "$OPENCODE_SKILLS/"; then
      did "skills" "copied from $SKILLS_REPO"
    else
      fail "skills" "copy from $SKILLS_REPO failed"
    fi
  else
    fail "skills" "clone $SKILLS_REPO failed"
  fi

  rm -rf "$tmp"
}

ensure_env() {
  section ".env"

  if [ -f .env ]; then
    have ".env" ".env already there, never overwritten"
  elif [ "$DRY_RUN" -eq 1 ]; then
    todo ".env" "would copy .env.example to .env"
  else
    cp .env.example .env || { fail ".env" "copying .env.example to .env failed"; return 0; }
    did ".env" "created from .env.example"
  fi

  local key
  key=$(env_value APP_KEY)
  if [ -n "$key" ]; then
    have "APP_KEY" "already set (${#key} chars)"
    return 0
  fi

  if [ "$DRY_RUN" -eq 1 ]; then
    todo "APP_KEY" "would run ./artisan key:generate"
    return 0
  fi

  # A throwaway DB_DATABASE keeps artisan's sqlite bootstrap out of the repo,
  # where config/database.go would leave a ./forge file. key:generate writes
  # support.EnvFilePath (.env) and exits 0 even when it fails, so the answer
  # comes from reading the file back.
  local scratch_db
  scratch_db=$(mktemp -t goravel-setup) || { fail "APP_KEY" "mktemp for the key:generate scratch database failed"; return 0; }
  DB_DATABASE="$scratch_db" ./artisan key:generate >/dev/null 2>&1
  rm -f "$scratch_db"
  key=$(env_value APP_KEY)
  if [ -n "$key" ]; then
    did "APP_KEY" "generated by ./artisan key:generate (${#key} chars)"
  elif command -v go >/dev/null 2>&1; then
    fail "APP_KEY" "./artisan key:generate left APP_KEY empty"
  else
    fail "APP_KEY" "./artisan key:generate needs go on PATH, and Go 1.26+ is a prerequisite of this script"
  fi
}

ensure_paseo_project() {
  section "paseo project (AGENT_PROJECT_NAME)"

  local name dir found
  name=$(env_value AGENT_PROJECT_NAME)
  [ -n "$name" ] || name=$(sed -n 's/^AGENT_PROJECT_NAME=//p' .env.example | head -1)
  if [ -z "$name" ]; then
    fail "paseo project" "AGENT_PROJECT_NAME is blank in .env and .env.example"
    return 0
  fi
  dir="$HOME/code/$name"

  if found=$(paseo_project_path "$name"); then
    have "paseo project" "$name at $found"
    if [ "$found" != "$dir" ]; then
      note "paseo project dir" "runs will start in $found, not $dir"
    fi
    return 0
  fi

  if [ "$DRY_RUN" -eq 1 ]; then
    todo "paseo project" "would create $name at $dir (paseo project create --json $dir)"
    return 0
  fi

  if ! paseo_daemon_running; then
    fail "paseo project" "daemon not running, so the project cannot be created"
    return 0
  fi

  mkdir -p "$dir"
  if ! paseo project create --json "$dir" >/dev/null 2>&1; then
    fail "paseo project" "paseo project create $dir failed"
    return 0
  fi

  if found=$(paseo_project_path "$name"); then
    did "paseo project" "created $name at $found"
  else
    fail "paseo project" "created but not named $name, rename it: paseo project rename <id> $name"
  fi
}

# verify repeats the same facts as a read-only pass. show keeps the totals those
# of the run itself, so the steps are not counted twice.
verify() {
  section "Verification"

  if command -v gh >/dev/null 2>&1; then
    show "gh" "$(gh --version | head -1 | awk '{print $3}')"
  else
    fail "gh" "not on PATH"
  fi

  if command -v uvx >/dev/null 2>&1; then
    show "uv / uvx" "$(command -v uvx)"
  else
    fail "uv / uvx" "not on PATH"
  fi

  if command -v opencode >/dev/null 2>&1; then
    show "opencode" "$(opencode --version 2>/dev/null | head -1 | awk '{print $NF}')"
  else
    fail "opencode" "not on PATH"
  fi

  if command -v paseo >/dev/null 2>&1; then
    show "paseo" "$(command -v paseo)"
  else
    fail "paseo" "not on PATH"
  fi

  if paseo_daemon_running; then
    show "paseo daemon" "running"
  else
    fail "paseo daemon" "not running"
  fi

  local name found
  name=$(env_value AGENT_PROJECT_NAME)
  [ -n "$name" ] || name=$(sed -n 's/^AGENT_PROJECT_NAME=//p' .env.example | head -1)
  if found=$(paseo_project_path "$name"); then
    show "paseo project" "$name at $found"
  else
    fail "paseo project" "$name is missing"
  fi

  if [ "$(mcp_entry check 2>/dev/null)" = "present" ]; then
    show "code-review-graph MCP" "in $OPENCODE_CONFIG"
  else
    fail "code-review-graph MCP" "not in $OPENCODE_CONFIG"
  fi

  if [ -f "$OPENCODE_AGENTS/ship.md" ]; then
    show "agents" "$(ls "$OPENCODE_AGENTS" | wc -l | tr -d ' ') entries in $OPENCODE_AGENTS"
  else
    fail "agents" "missing from $OPENCODE_AGENTS"
  fi

  if [ -f "$OPENCODE_SKILLS/goravel-planning/SKILL.md" ]; then
    show "skills" "$(ls "$OPENCODE_SKILLS" | wc -l | tr -d ' ') entries in $OPENCODE_SKILLS"
  else
    fail "skills" "missing from $OPENCODE_SKILLS"
  fi

  if [ -f .env ] && [ -n "$(env_value APP_KEY)" ]; then
    show ".env" "with APP_KEY"
  else
    fail ".env" "missing, or APP_KEY is empty"
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
        printf 'setup.sh: unknown argument: %s\n\n' "$1" >&2
        usage >&2
        exit 2
        ;;
    esac
    shift
  done

  legend
  preconditions
  ensure_gh
  ensure_uv
  ensure_opencode
  ensure_paseo
  ensure_paseo_daemon
  ensure_mcp_entry
  ensure_agents_and_skills
  ensure_env
  ensure_paseo_project

  if [ "$DRY_RUN" -eq 1 ]; then
    result
    printf '\n  Nothing was written and nothing was installed. Re-run without --dry-run to apply.\n'
    exit 0
  fi

  verify
  result

  if [ "$FAILED" -gt 0 ]; then
    printf '\n%s check(s) failed.\n' "$FAILED" >&2
    exit 1
  fi

  printf '\nNext:\n'
  printf '  go run .                # serves on APP_PORT (4500)\n'
  printf '  ./artisan poll-events   # one mention poll cycle\n'
  printf '\nDone.\n'
}

main "$@"
