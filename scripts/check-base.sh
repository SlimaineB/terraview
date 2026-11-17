#!/usr/bin/env bash
# Warn when the local base branch (the merge target) or the current branch is
# behind the local remote-tracking ref. Cheap by default: no network access.
#
# Usage: ./scripts/check-base.sh [--base BRANCH] [--remote NAME] [--fetch] [--strict]

set -euo pipefail

REMOTE="${REMOTE:-origin}"
BASE=""
FETCH=0
STRICT=0

usage() {
  cat <<'EOF'
Usage: ./scripts/check-base.sh [options]

Check that the local base branch and the current branch are based on the
latest local remote-tracking ref (for example origin/main). No network access
unless --fetch is passed.

Options:
      --base BRANCH   Base branch (default: the remote's default branch)
      --remote NAME   Git remote (default: origin)
      --fetch         Fetch the base branch first (network)
      --strict        Exit 1 when the base or current branch is behind
  -h, --help          Show this help

Exit codes: 0 up to date or not checkable, 1 stale with --strict, 2 usage error.
EOF
}

die() { echo "check-base: $*" >&2; exit 2; }

require_value() {
  [[ $# -ge 2 && -n "$2" ]] || die "option $1 requires a value"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --base)   require_value "$@"; BASE="$2"; shift 2 ;;
    --remote) require_value "$@"; REMOTE="$2"; shift 2 ;;
    --fetch)  FETCH=1; shift ;;
    --strict) STRICT=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown option: $1 (try --help)" ;;
  esac
done

if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  echo "check-base: not a git repository; skipping." >&2
  exit 0
fi

ROOT=$(git rev-parse --show-toplevel)
cd "$ROOT"

if [[ -z "$BASE" ]]; then
  BASE=$(git symbolic-ref --quiet --short "refs/remotes/$REMOTE/HEAD" 2>/dev/null || true)
  BASE=${BASE#"$REMOTE/"}
  if [[ -z "$BASE" ]]; then
    for candidate in main master; do
      if git show-ref --verify --quiet "refs/remotes/$REMOTE/$candidate"; then
        BASE="$candidate"
        break
      fi
    done
  fi
  [[ -n "$BASE" ]] || BASE="main"
fi

REMOTE_REF="$REMOTE/$BASE"

if [[ "$FETCH" == "1" ]]; then
  if ! git fetch --quiet "$REMOTE" "$BASE"; then
    echo "check-base: git fetch $REMOTE $BASE failed; using local refs." >&2
  fi
fi

if ! git rev-parse --verify --quiet "$REMOTE_REF" >/dev/null 2>&1; then
  echo "check-base: no local ref for $REMOTE_REF; run 'git fetch $REMOTE' to enable the check." >&2
  exit 0
fi

count() { git rev-list --count "$1" 2>/dev/null || echo 0; }

WARNINGS=0
warn() { echo "check-base: $*" >&2; WARNINGS=$((WARNINGS + 1)); }

CURRENT=$(git symbolic-ref --quiet --short HEAD 2>/dev/null || true)

if git rev-parse --verify --quiet "refs/heads/$BASE" >/dev/null 2>&1; then
  BASE_BEHIND=$(count "$BASE..$REMOTE_REF")
  BASE_AHEAD=$(count "$REMOTE_REF..$BASE")
  if [[ "$BASE_BEHIND" -gt 0 ]]; then
    warn "local '$BASE' is $BASE_BEHIND commit(s) behind $REMOTE_REF."
  fi
  if [[ "$BASE_AHEAD" -gt 0 ]]; then
    warn "local '$BASE' has $BASE_AHEAD commit(s) not on $REMOTE_REF."
  fi
fi

if [[ -n "$CURRENT" && "$CURRENT" != "$BASE" ]]; then
  BRANCH_BEHIND=$(count "HEAD..$REMOTE_REF")
  if [[ "$BRANCH_BEHIND" -gt 0 ]]; then
    warn "current branch '$CURRENT' is $BRANCH_BEHIND commit(s) behind $REMOTE_REF; rebase or merge before planning."
  fi
fi

if [[ "$WARNINGS" -gt 0 ]]; then
  if [[ "$FETCH" == "0" ]]; then
    echo "check-base: based on local remote-tracking refs; run 'git fetch $REMOTE' or pass --fetch to refresh." >&2
  fi
  if [[ "$STRICT" == "1" ]]; then
    exit 1
  fi
  exit 0
fi

if [[ "$FETCH" == "0" ]]; then
  echo "check-base: '$BASE' and the current branch are up to date with $REMOTE_REF (local refs)."
else
  echo "check-base: '$BASE' and the current branch are up to date with $REMOTE_REF."
fi
