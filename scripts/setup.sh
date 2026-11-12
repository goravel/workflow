#!/bin/bash
#
# setup.sh — prepare this Goravel workflow checkout on macOS.
#
# Installs the external services the app and its agent tooling reach for, then
# writes the shared config files only when a key, a folder or a project is
# missing. It creates no database/, no storage/, and runs no migrate.
#
#   brew      Homebrew, the only package source for gh, the Docker tools, uv and
#             the paseo cask
#   go        official go.dev tarball in /usr/local/go (never Homebrew)
#   gh        GitHub CLI, for the release commands and the mention poller
#   docker    docker, docker-compose and colima — installed, not started
#   uv/uvx    the code-review-graph MCP server runs as ["uvx", "code-review-graph", "serve"]
#   opencode  the V2 installer, into ~/.opencode/bin
#   paseo     the cask, its daemon, and the project AGENT_PROJECT_NAME names
#   config    ~/.config/opencode: the MCP entry plus agents/ and skills/
#   repo      .env with an APP_KEY, and go mod download
#
# macOS only, non-interactive, macOS /bin/bash 3.2: no associative arrays, no
# mapfile, no ${var,,}, no sort -V, no jq.
#
# Usage: scripts/setup.sh [-y|--yes] [-n|--dry-run] [-h|--help]

set -uo pipefail

REPO_ROOT=$(cd "$(dirname "$0")/.." && pwd)
readonly REPO_ROOT

readonly GO_ROOT="/usr/local/go"
readonly GO_FLOOR="1.26.0"
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

usage() {
  cat <<'EOF'
Prepare this checkout on macOS.

Installs Homebrew, Go, gh, Docker (docker, docker-compose, colima), uv/uvx,
opencode and paseo, then writes the ~/.config/opencode entries
(code-review-graph MCP server, agents, skills), .env with an APP_KEY, and the
paseo project AGENT_PROJECT_NAME names.

Every step skips itself when its target is already there; `go mod download`
always runs because a warm cache makes it a no-op. Inside the repo the only
writes are .env and artisan's own log.

Options:
  -y, --yes       accept everything (already the default)
  -n, --dry-run   print what would happen, write nothing
  -h, --help      show this help

Exit status: 0 done, 1 a check failed, 2 unknown argument.
EOF
}

section() { printf '\n==> %s\n' "$*"; }
ok()      { printf '  ok   %s\n' "$*"; }
skip()    { printf '  skip %s\n' "$*"; }
warn()    { printf '  warn %s\n' "$*"; }
fail()    { printf '  FAIL %s\n' "$*" >&2; FAILED=$((FAILED + 1)); }
plan()    { printf '  plan %s\n' "$*"; }

# run executes an argv command, or prints it under --dry-run.
run() {
  if [ "$DRY_RUN" -eq 1 ]; then
    plan "$*"
    return 0
  fi
  "$@"
}

# sh_run executes a shell string, or prints it under --dry-run.
sh_run() {
  if [ "$DRY_RUN" -eq 1 ]; then
    plan "$1"
    return 0
  fi
  /bin/bash -c "$1"
}

# version_ge <have> <want> is true when have >= want for dot-separated numbers.
# No sort -V on macOS: split on "." and compare field by field.
version_ge() {
  local have=${1#go} want=${2#go}
  local IFS=.
  local ha=($have) wa=($want) i h w
  for i in 0 1 2; do
    h=${ha[i]:-0}
    w=${wa[i]:-0}
    h=${h%%[!0-9]*}
    w=${w%%[!0-9]*}
    [ -n "$h" ] || h=0
    [ -n "$w" ] || w=0
    if [ "$((10#$h))" -gt "$((10#$w))" ]; then return 0; fi
    if [ "$((10#$h))" -lt "$((10#$w))" ]; then return 1; fi
  done
  return 0
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
    printf 'setup.sh: macOS only, this host reports %s\n' "$uname_s" >&2
    exit 1
  fi
  ok "macOS $(uname -m)"

  if ! xcode-select -p >/dev/null 2>&1; then
    printf 'setup.sh: Command Line Tools are missing, install them first:\n  xcode-select --install\n' >&2
    exit 1
  fi
  ok "Command Line Tools: $(xcode-select -p)"

  local tool missing=""
  for tool in curl python3 git; do
    command -v "$tool" >/dev/null 2>&1 || missing="$missing $tool"
  done
  if [ -n "$missing" ]; then
    printf 'setup.sh: missing required tools:%s\n' "$missing" >&2
    exit 1
  fi
  ok "curl, python3, git"

  cd "$REPO_ROOT" || exit 1
  if [ ! -f .env.example ]; then
    printf 'setup.sh: %s has no .env.example, run scripts/setup.sh from the checkout\n' "$REPO_ROOT" >&2
    exit 1
  fi
  ok "checkout $REPO_ROOT"
}

ensure_homebrew() {
  section "Homebrew"

  if command -v brew >/dev/null 2>&1; then
    ok "brew $(brew --version | head -1 | awk '{print $2}') at $(command -v brew)"
  elif [ -x /opt/homebrew/bin/brew ] || [ -x /usr/local/bin/brew ]; then
    ok "brew installed outside PATH"
  else
    sh_run 'NONINTERACTIVE=1 /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"' \
      || { fail "Homebrew install"; return 0; }
  fi

  local prefix
  for prefix in /opt/homebrew /usr/local; do
    if [ -x "$prefix/bin/brew" ]; then
      eval "$("$prefix/bin/brew" shellenv)"
      ok "brew shellenv: $prefix"
      return 0
    fi
  done

  if [ "$DRY_RUN" -eq 0 ]; then
    fail "brew not found after install"
  fi
}

ensure_go() {
  section "Go (official tarball, floor $GO_FLOOR)"

  if [ -x "$GO_ROOT/bin/go" ]; then
    PATH="$GO_ROOT/bin:$PATH"
    export PATH
  fi

  local have=""
  if command -v go >/dev/null 2>&1; then
    have=$(go version | awk '{print $3}')
  fi

  if [ -n "$have" ] && version_ge "$have" "$GO_FLOOR"; then
    ok "go ${have#go} at $(command -v go)"
    return 0
  fi

  if [ "$DRY_RUN" -eq 1 ]; then
    plan "install the latest stable Go from https://go.dev/VERSION?m=text into $GO_ROOT"
    return 0
  fi

  local stable arch tarball tmp
  stable=$(curl -fsSL 'https://go.dev/VERSION?m=text' | head -1) \
    || { fail "read https://go.dev/VERSION?m=text"; return 0; }
  stable=${stable#go}
  if [ -z "$stable" ]; then
    fail "https://go.dev/VERSION?m=text returned nothing"
    return 0
  fi

  case "$(uname -m)" in
    arm64)  arch=arm64 ;;
    x86_64) arch=amd64 ;;
    *)      fail "unsupported machine $(uname -m)"; return 0 ;;
  esac

  tarball="go${stable}.darwin-${arch}.tar.gz"
  tmp=$(mktemp -d)
  if ! curl -fsSL "https://dl.google.com/go/$tarball" -o "$tmp/$tarball"; then
    fail "download $tarball"
    rm -rf "$tmp"
    return 0
  fi

  if [ -w /usr/local ]; then
    rm -rf "$GO_ROOT" && tar -C /usr/local -xzf "$tmp/$tarball"
  else
    printf '  sudo is needed to write %s\n' "$GO_ROOT"
    sudo rm -rf "$GO_ROOT" && sudo tar -C /usr/local -xzf "$tmp/$tarball"
  fi || { fail "extract $tarball into /usr/local"; rm -rf "$tmp"; return 0; }
  rm -rf "$tmp"

  PATH="$GO_ROOT/bin:$PATH"
  export PATH

  if command -v go >/dev/null 2>&1 && version_ge "$(go version | awk '{print $3}')" "$GO_FLOOR"; then
    ok "go ${stable} in $GO_ROOT"
  else
    fail "go is still missing or below $GO_FLOOR after install"
    return 0
  fi

  # The tarball lands outside every shell's PATH: record it for new terminals,
  # or the `go run .` in Next: fails as soon as this process exits.
  if grep -qF "$GO_ROOT/bin" "$HOME/.zprofile" 2>/dev/null; then
    skip "$GO_ROOT/bin already in ~/.zprofile"
  elif printf 'export PATH="%s/bin:$PATH"\n' "$GO_ROOT" >> "$HOME/.zprofile" 2>/dev/null; then
    ok "added $GO_ROOT/bin to ~/.zprofile"
  else
    warn "add this to your shell profile: export PATH=\"$GO_ROOT/bin:\$PATH\""
  fi
}

ensure_modules() {
  section "Go modules"
  # The one step that always runs: a warm module cache makes it a no-op.
  run go mod download || fail "go mod download"
}

ensure_gh() {
  section "GitHub CLI"

  if command -v gh >/dev/null 2>&1; then
    ok "gh $(gh --version | head -1 | awk '{print $3}')"
    return 0
  fi

  run brew install gh || { fail "brew install gh"; return 0; }
  if [ "$DRY_RUN" -eq 0 ]; then
    if command -v gh >/dev/null 2>&1; then ok "gh installed"; else fail "gh still missing"; fi
  fi
}

ensure_docker() {
  section "Docker (docker, docker-compose, colima)"

  local tool missing=""
  for tool in docker docker-compose colima; do
    command -v "$tool" >/dev/null 2>&1 || missing="$missing $tool"
  done

  if [ -z "$missing" ]; then
    ok "docker, docker-compose, colima present"
  else
    printf '  installing:%s\n' "$missing"
    # shellcheck disable=SC2086
    run brew install $missing || { fail "brew install$missing"; return 0; }
  fi

  if [ "$DRY_RUN" -eq 1 ]; then
    plan "link docker-compose into $HOME/.docker/cli-plugins when \`docker compose\` cannot find it"
    plan "leave the daemon alone: the summary reports \`colima start\`"
    return 0
  fi

  # `brew install docker-compose` ships Compose as a plugin outside the docker
  # CLI search path (brew info docker-compose: add cliPluginsExtraDirs). A link
  # in ~/.docker/cli-plugins is enough and edits no config file.
  if ! docker compose version >/dev/null 2>&1; then
    local compose_bin
    compose_bin="$(brew --prefix)/opt/docker-compose/bin/docker-compose"
    if [ -x "$compose_bin" ]; then
      mkdir -p "$HOME/.docker/cli-plugins"
      ln -sfn "$compose_bin" "$HOME/.docker/cli-plugins/docker-compose"
      ok "linked docker compose plugin into ~/.docker/cli-plugins"
    else
      warn "no compose plugin at $compose_bin, \`docker compose\` stays unavailable"
    fi
  fi

  if docker info >/dev/null 2>&1; then
    ok "docker daemon answering"
  else
    warn "docker daemon not running (start it with: colima start)"
  fi
}

ensure_uv() {
  section "uv / uvx (code-review-graph MCP server)"

  if command -v uvx >/dev/null 2>&1; then
    ok "uvx at $(command -v uvx)"
    return 0
  fi

  run brew install uv || { fail "brew install uv"; return 0; }
  if [ "$DRY_RUN" -eq 0 ]; then
    if command -v uvx >/dev/null 2>&1; then ok "uvx installed"; else fail "uvx still missing"; fi
  fi
}

ensure_opencode() {
  section "opencode (V2)"

  PATH="$OPENCODE_BIN:$PATH"
  export PATH

  if command -v opencode >/dev/null 2>&1; then
    ok "opencode $(opencode --version 2>/dev/null | head -1 | awk '{print $NF}')"
    return 0
  fi

  sh_run 'curl -fsSL https://opencode.ai/v2/install | bash' || { fail "opencode V2 installer"; return 0; }
  if [ "$DRY_RUN" -eq 0 ]; then
    if command -v opencode >/dev/null 2>&1; then
      ok "opencode installed in $OPENCODE_BIN"
    else
      fail "opencode still missing"
    fi
  fi
}

ensure_paseo() {
  section "paseo"

  if brew list --cask paseo >/dev/null 2>&1; then
    ok "paseo cask installed"
  elif [ -d "$PASEO_APP" ]; then
    ok "Paseo.app already in /Applications"
  else
    run brew install --cask paseo || { fail "brew install --cask paseo"; return 0; }
  fi

  if [ "$DRY_RUN" -eq 1 ]; then
    plan "put paseo on PATH (fallback link $LOCAL_BIN/paseo -> $PASEO_BIN_IN_APP)"
    return 0
  fi

  # app/services/agent/paseo.go runs `env -i PATH=$PATH HOME=$HOME paseo ...`,
  # so the binary must be on the human's PATH, not only inside the .app.
  if ! command -v paseo >/dev/null 2>&1; then
    if [ ! -x "$PASEO_BIN_IN_APP" ]; then
      fail "paseo binary missing at $PASEO_BIN_IN_APP"
      return 0
    fi
    mkdir -p "$LOCAL_BIN"
    ln -sfn "$PASEO_BIN_IN_APP" "$LOCAL_BIN/paseo"
    PATH="$LOCAL_BIN:$PATH"
    export PATH
  fi

  if command -v paseo >/dev/null 2>&1; then
    ok "paseo $(paseo --version 2>/dev/null | head -1) at $(command -v paseo)"
  else
    fail "paseo is not on PATH"
  fi
}

ensure_paseo_daemon() {
  section "paseo daemon"

  if [ "$DRY_RUN" -eq 1 ]; then
    plan "open -a Paseo when the daemon does not answer, wait up to ${DAEMON_WAIT}s"
    return 0
  fi

  if paseo_daemon_running; then
    ok "daemon already running"
    return 0
  fi

  open -a Paseo || { fail "open $PASEO_APP"; return 0; }
  printf '  waiting up to %ss for the daemon\n' "$DAEMON_WAIT"

  local waited=0
  while [ "$waited" -lt "$DAEMON_WAIT" ]; do
    sleep 2
    waited=$((waited + 2))
    if paseo_daemon_running; then
      ok "daemon running after ${waited}s"
      return 0
    fi
  done

  fail "daemon did not answer in ${DAEMON_WAIT}s, open Paseo.app once"
}

ensure_mcp_entry() {
  section "opencode config: code-review-graph MCP server"

  if [ "$DRY_RUN" -eq 1 ]; then
    plan "add mcp.servers[\"code-review-graph\"] to $OPENCODE_CONFIG when absent"
    return 0
  fi

  mkdir -p "$(dirname "$OPENCODE_CONFIG")"

  local result
  result=$(python3 - "$OPENCODE_CONFIG" <<'PY'
import json, os, shutil, sys, tempfile

path = sys.argv[1]
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

servers["code-review-graph"] = entry

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
  ) || { fail "could not merge into $OPENCODE_CONFIG, left untouched"; return 0; }

  case "$result" in
    present) skip "mcp.servers[\"code-review-graph\"] already there" ;;
    merged)  ok "added mcp.servers[\"code-review-graph\"] (backup: $OPENCODE_CONFIG.bak)" ;;
    *)       fail "unexpected merge result: $result" ;;
  esac
}

ensure_agents_and_skills() {
  section "agents and skills (~/.config/opencode)"

  if [ -f "$OPENCODE_AGENTS/ship.md" ] && [ -f "$OPENCODE_SKILLS/goravel-planning/SKILL.md" ]; then
    skip "agents and skills already in ~/.config/opencode"
    return 0
  fi

  if [ "$DRY_RUN" -eq 1 ]; then
    plan "git clone --depth 1 $AGENTS_REPO: agents/ -> $OPENCODE_AGENTS, skills/ -> $OPENCODE_SKILLS"
    plan "git clone --depth 1 $SKILLS_REPO: skills/ -> $OPENCODE_SKILLS"
    return 0
  fi

  local tmp
  tmp=$(mktemp -d)

  if git clone --depth 1 -q "https://github.com/$AGENTS_REPO.git" "$tmp/agents"; then
    mkdir -p "$OPENCODE_AGENTS" "$OPENCODE_SKILLS"
    if rsync -a "$tmp/agents/agents/" "$OPENCODE_AGENTS/" \
      && rsync -a "$tmp/agents/skills/" "$OPENCODE_SKILLS/"; then
      ok "agents and skills from $AGENTS_REPO"
    else
      fail "copy agents and skills from $AGENTS_REPO"
    fi
  else
    fail "clone $AGENTS_REPO"
  fi

  if git clone --depth 1 -q "https://github.com/$SKILLS_REPO.git" "$tmp/skills"; then
    mkdir -p "$OPENCODE_SKILLS"
    if rsync -a "$tmp/skills/skills/" "$OPENCODE_SKILLS/"; then
      ok "skills from $SKILLS_REPO"
    else
      fail "copy skills from $SKILLS_REPO"
    fi
  else
    fail "clone $SKILLS_REPO"
  fi

  rm -rf "$tmp"
}

ensure_env() {
  section ".env"

  if [ -f .env ]; then
    skip ".env already there, never overwritten"
  elif [ "$DRY_RUN" -eq 1 ]; then
    plan "cp .env.example .env"
  else
    cp .env.example .env || { fail "copy .env.example to .env"; return 0; }
    ok ".env created from .env.example"
  fi

  if [ "$DRY_RUN" -eq 1 ]; then
    plan "./artisan key:generate when APP_KEY is empty"
    return 0
  fi

  if [ -n "$(env_value APP_KEY)" ]; then
    ok "APP_KEY already set"
    return 0
  fi

  # A throwaway DB_DATABASE keeps artisan's sqlite bootstrap out of the repo,
  # where config/database.go would leave a ./forge file. key:generate writes
  # support.EnvFilePath (.env) and exits 0 even when it fails, so the answer
  # comes from reading the file back.
  local scratch_db
  scratch_db=$(mktemp -t goravel-setup) || { fail "mktemp for the key:generate scratch database"; return 0; }
  DB_DATABASE="$scratch_db" ./artisan key:generate >/dev/null 2>&1
  rm -f "$scratch_db"
  if [ -n "$(env_value APP_KEY)" ]; then
    ok "APP_KEY generated"
  else
    fail "./artisan key:generate left APP_KEY empty"
  fi
}

ensure_paseo_project() {
  section "paseo project (AGENT_PROJECT_NAME)"

  local name dir
  name=$(env_value AGENT_PROJECT_NAME)
  if [ -z "$name" ]; then
    name=$(sed -n 's/^AGENT_PROJECT_NAME=//p' .env.example | head -1)
  fi
  if [ -z "$name" ]; then
    fail "AGENT_PROJECT_NAME is blank in .env and .env.example"
    return 0
  fi
  dir="$HOME/code/$name"

  if [ "$DRY_RUN" -eq 1 ]; then
    plan "create the paseo project $name at $dir when \`paseo project ls --json\` does not list it"
    return 0
  fi

  if ! paseo_daemon_running; then
    fail "daemon not running, so the project cannot be created"
    return 0
  fi

  local found
  if found=$(paseo_project_path "$name"); then
    ok "project $name at $found"
    if [ "$found" != "$dir" ]; then
      warn "paseo has $name at $found, runs will start there, not in $dir"
    fi
    return 0
  fi

  mkdir -p "$dir"
  if ! paseo project create --json "$dir" >/dev/null 2>&1; then
    fail "paseo project create $dir"
    return 0
  fi

  if found=$(paseo_project_path "$name"); then
    ok "project $name created at $found"
  else
    fail "project created but not named $name, rename it: paseo project rename <id> $name"
  fi
}

report() {
  local label=$1 value=$2
  if [ -z "$value" ] || [ "$value" = "missing" ]; then
    fail "$label missing"
  else
    printf '  ok   %-18s %s\n' "$label" "$value"
  fi
}

doctor() {
  section "Summary"

  report "macOS"    "$(uname -s) $(uname -m)"
  report "brew"     "$(brew --version 2>/dev/null | head -1 | awk '{print $2}')"
  local go_have
  go_have=$(go version 2>/dev/null | awk '{print $3}')
  if [ -n "$go_have" ]; then
    printf '  ok   %-18s %s (floor %s)\n' "go" "$go_have" "$GO_FLOOR"
  else
    fail "go missing"
  fi
  report "gh"       "$(gh --version 2>/dev/null | head -1 | awk '{print $3}')"
  report "docker"   "$(docker --version 2>/dev/null)"
  report "compose"  "$(docker-compose version 2>/dev/null | head -1 | awk '{print $4}')"
  report "colima"   "$(colima version 2>/dev/null | head -1 | awk '{print $3}')"
  report "uvx"      "$(command -v uvx || echo missing)"
  report "opencode" "$(opencode --version 2>/dev/null | head -1 | awk '{print $NF}')"
  report "paseo"    "$(command -v paseo || echo missing)"

  if docker info >/dev/null 2>&1; then
    ok "docker daemon      answering"
  else
    warn "docker daemon      not running, start it with: colima start"
  fi

  if paseo_daemon_running; then
    ok "paseo daemon       running"
  else
    fail "paseo daemon not running"
  fi

  local name dir found=""
  name=$(env_value AGENT_PROJECT_NAME)
  [ -n "$name" ] || name=$(sed -n 's/^AGENT_PROJECT_NAME=//p' .env.example | head -1)
  dir="$HOME/code/$name"
  if found=$(paseo_project_path "$name"); then
    if [ "$found" = "$dir" ]; then
      ok "paseo project      $name at $found"
    else
      warn "paseo project      $name at $found (not $dir)"
    fi
  else
    fail "paseo project $name missing"
  fi

  if python3 -c '
import json, sys
with open(sys.argv[1]) as handle:
    data = json.load(handle)
sys.exit(0 if "code-review-graph" in data.get("mcp", {}).get("servers", {}) else 1)
' "$OPENCODE_CONFIG" 2>/dev/null; then
    ok "mcp entry          code-review-graph"
  else
    fail "mcp entry code-review-graph missing from $OPENCODE_CONFIG"
  fi

  if [ -f "$OPENCODE_AGENTS/ship.md" ]; then
    ok "agents             $(ls "$OPENCODE_AGENTS" | wc -l | tr -d ' ') entries in $OPENCODE_AGENTS"
  else
    fail "agents missing from $OPENCODE_AGENTS"
  fi

  if [ -f "$OPENCODE_SKILLS/goravel-planning/SKILL.md" ] && [ -f "$OPENCODE_SKILLS/go-testing/SKILL.md" ]; then
    ok "skills             $(ls "$OPENCODE_SKILLS" | wc -l | tr -d ' ') entries in $OPENCODE_SKILLS"
  else
    fail "skills missing from $OPENCODE_SKILLS"
  fi

  if [ -f .env ] && [ -n "$(env_value APP_KEY)" ]; then
    ok "env                .env with APP_KEY"
  else
    fail ".env missing or APP_KEY empty"
  fi
}

main() {
  while [ $# -gt 0 ]; do
    case "$1" in
      -y|--yes)     : ;;
      -n|--dry-run) DRY_RUN=1 ;;
      -h|--help)    usage; exit 0 ;;
      *)
        printf 'setup.sh: unknown argument: %s\n\n' "$1" >&2
        usage >&2
        exit 2
        ;;
    esac
    shift
  done

  preconditions
  ensure_homebrew
  ensure_go
  ensure_modules
  ensure_gh
  ensure_docker
  ensure_uv
  ensure_opencode
  ensure_paseo
  ensure_paseo_daemon
  ensure_mcp_entry
  ensure_agents_and_skills
  ensure_env
  ensure_paseo_project

  if [ "$DRY_RUN" -eq 1 ]; then
    section "Dry run"
    printf '  nothing was written and nothing was installed\n'
    exit 0
  fi

  doctor

  if [ "$FAILED" -gt 0 ]; then
    printf '\n%s check(s) failed.\n' "$FAILED" >&2
    exit 1
  fi

  printf '\nNext:\n'
  printf '  colima start            # container VM, a few minutes the first time\n'
  printf '  go run .                # serves on APP_PORT (4500)\n'
  printf '  ./artisan poll-events   # one mention poll cycle\n'
  printf '\nDone.\n'
}

main "$@"
