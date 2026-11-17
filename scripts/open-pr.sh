#!/usr/bin/env bash
# Commit the current changes on a branch and open a GitHub PR / GitLab MR.
#
# Opt-in: disabled unless TERRAVIEW_ALLOW_PR=1.
# It only stages, commits, pushes the feature branch, and creates the PR/MR.
# It refuses to run on the base branch, never force-pushes, and never creates tags.

set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

if [[ "${TERRAVIEW_ALLOW_PR:-0}" != "1" ]]; then
  cat >&2 <<'EOF'
open-pr.sh is disabled (opt-in).

It commits your changes and opens a GitHub PR / GitLab MR. Enable it for one command:

  TERRAVIEW_ALLOW_PR=1 ./scripts/open-pr.sh -m "Your commit message"
  TERRAVIEW_ALLOW_PR=1 make pr MSG="Your commit message"

It refuses to run on the base branch and never force-pushes or creates tags.
EOF
  exit 1
fi

usage() {
  cat <<'EOF'
Usage: TERRAVIEW_ALLOW_PR=1 ./scripts/open-pr.sh -m MESSAGE [options]

Commit the current changes on a feature branch and open a GitHub PR / GitLab MR.

Options:
  -m, --message MSG   Commit message (required when there are uncommitted changes)
  -t, --title TITLE   PR/MR title (default: latest commit subject)
  -b, --body BODY     PR/MR description (default: commit subjects since the base)
      --base BRANCH   Target branch (default: the remote's default branch)
      --branch NAME   Branch to commit on (default: current, or pr/<slug> on the base)
      --remote NAME   Git remote (default: origin)
      --provider NAME github or gitlab (default: detect from the remote host)
      --api-base URL  API base URL override (GitHub Enterprise, self-hosted GitLab)
      --draft         Open the PR/MR as a draft
      --fetch         Fetch the base branch before checking it (network)
      --strict-base   Abort when the base or current branch is behind
      --staged        Commit only already-staged changes
      --no-push       Do not push (use when the branch is already on the remote)
      --dry-run       Print the plan without committing, pushing, or calling the API
  -h, --help          Show this help

Tokens:
  GitHub: GH_TOKEN or GITHUB_TOKEN, else `git credential fill`
  GitLab: GITLAB_TOKEN or GLAB_TOKEN, else `git credential fill`

Requires: git, python3
EOF
}

die() { echo "open-pr: $*" >&2; exit 1; }

MESSAGE=""
TITLE=""
BODY=""
BASE=""
BRANCH=""
REMOTE="origin"
PROVIDER=""
API_BASE=""
DRAFT=0
DRY_RUN=0
NO_PUSH=0
STAGED_ONLY=0
FETCH=0
STRICT_BASE=0

require_value() {
  [[ $# -ge 2 && -n "$2" ]] || die "option $1 requires a value"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -m|--message) require_value "$@"; MESSAGE="$2"; shift 2 ;;
    -t|--title)   require_value "$@"; TITLE="$2"; shift 2 ;;
    -b|--body)    require_value "$@"; BODY="$2"; shift 2 ;;
    --base)       require_value "$@"; BASE="$2"; shift 2 ;;
    --branch)     require_value "$@"; BRANCH="$2"; shift 2 ;;
    --remote)     require_value "$@"; REMOTE="$2"; shift 2 ;;
    --provider)   require_value "$@"; PROVIDER="$2"; shift 2 ;;
    --api-base)   require_value "$@"; API_BASE="$2"; shift 2 ;;
    --draft)      DRAFT=1; shift ;;
    --fetch)      FETCH=1; shift ;;
    --strict-base) STRICT_BASE=1; shift ;;
    --staged)     STAGED_ONLY=1; shift ;;
    --no-push)    NO_PUSH=1; shift ;;
    --dry-run)    DRY_RUN=1; shift ;;
    -h|--help)    usage; exit 0 ;;
    *) die "unknown option: $1 (try --help)" ;;
  esac
done

case "$PROVIDER" in
  ""|github|gitlab) ;;
  *) die "--provider must be github or gitlab" ;;
esac

command -v git >/dev/null 2>&1 || die "git is required"
command -v python3 >/dev/null 2>&1 || die "python3 is required"

ROOT=$(git rev-parse --show-toplevel 2>/dev/null) || die "not inside a git repository"
cd "$ROOT"

REMOTE_URL=$(git remote get-url "$REMOTE" 2>/dev/null) || die "remote '$REMOTE' not found"

read -r HOST REMOTE_PATH < <(python3 - "$REMOTE_URL" <<'PY'
import re, sys, urllib.parse

url = sys.argv[1]
if url.startswith("git@"):
    host, path = url[4:].split(":", 1)
elif "://" in url:
    parsed = urllib.parse.urlsplit(url)
    host = parsed.hostname or ""
    path = parsed.path.lstrip("/")
else:
    host, path = "", url
path = re.sub(r"\.git$", "", path.strip("/"))
print(f"{host}\t{path}")
PY
) || die "unsupported remote URL: $REMOTE_URL"

[[ -n "$HOST" && -n "$REMOTE_PATH" ]] || die "unsupported remote URL: $REMOTE_URL"

if [[ -z "$PROVIDER" ]]; then
  case "$HOST" in
    github.com|*github*) PROVIDER="github" ;;
    gitlab.com|*gitlab*) PROVIDER="gitlab" ;;
    *) die "cannot detect the provider for '$HOST'; pass --provider github|gitlab" ;;
  esac
fi

if [[ -z "$API_BASE" ]]; then
  case "$PROVIDER" in
    github)
      if [[ "$HOST" == "github.com" ]]; then
        API_BASE="https://api.github.com"
      else
        API_BASE="https://$HOST/api/v3"
      fi
      ;;
    gitlab) API_BASE="https://$HOST/api/v4" ;;
  esac
fi

if [[ -z "$BASE" ]]; then
  BASE=$(git symbolic-ref --quiet --short "refs/remotes/$REMOTE/HEAD" 2>/dev/null || true)
  BASE=${BASE#"$REMOTE/"}
  [[ -n "$BASE" ]] || BASE="main"
fi

BASE_REF="$BASE"
if ! git rev-parse --verify --quiet "$BASE_REF" >/dev/null 2>&1; then
  if git rev-parse --verify --quiet "$REMOTE/$BASE" >/dev/null 2>&1; then
    BASE_REF="$REMOTE/$BASE"
  else
    die "cannot resolve base branch '$BASE'"
  fi
fi

CURRENT=$(git symbolic-ref --quiet --short HEAD 2>/dev/null || true)
[[ -n "$CURRENT" ]] || die "detached HEAD: check out a branch first"

if [[ -z "$BRANCH" ]]; then
  if [[ "$CURRENT" == "$BASE" ]]; then
    SLUG=$(printf '%s' "${TITLE:-$MESSAGE}" | tr '[:upper:]' '[:lower:]' \
      | sed -E 's/[^a-z0-9]+/-/g; s/^-+//; s/-+$//' | cut -c1-40)
    [[ -n "$SLUG" ]] || SLUG="terraview-$(date +%Y%m%d-%H%M%S)"
    BRANCH="pr/$SLUG"
  else
    BRANCH="$CURRENT"
  fi
fi

[[ "$BRANCH" != "$BASE" ]] || die "refusing to commit/open a PR on the base branch '$BASE'; pass --branch NAME"

BRANCH_ACTION="use"
if [[ "$CURRENT" != "$BRANCH" ]]; then
  if [[ "$CURRENT" != "$BASE" ]]; then
    die "currently on '$CURRENT'; refusing to move changes to '$BRANCH' (switch branches first)"
  fi
  if git show-ref --verify --quiet "refs/heads/$BRANCH"; then
    BRANCH_ACTION="switch"
  else
    BRANCH_ACTION="create"
  fi
fi

HAS_CHANGES=0
if [[ "$STAGED_ONLY" == "1" ]]; then
  if ! git diff --cached --quiet; then HAS_CHANGES=1; fi
else
  if [[ -n "$(git status --porcelain)" ]]; then HAS_CHANGES=1; fi
fi

AHEAD_REF="$BRANCH"
if ! git rev-parse --verify --quiet "refs/heads/$BRANCH" >/dev/null 2>&1; then
  AHEAD_REF="HEAD"
fi
AHEAD=$(git rev-list --count "$BASE_REF..$AHEAD_REF")

if [[ "$HAS_CHANGES" == "0" && "$AHEAD" == "0" ]]; then
  die "nothing to commit and no commits ahead of $BASE"
fi

if [[ "$HAS_CHANGES" == "1" && -z "$MESSAGE" ]]; then
  die "uncommitted changes detected; pass -m/--message"
fi

CHECK_ARGS=(--base "$BASE" --remote "$REMOTE")
if [[ "$FETCH" == "1" ]]; then CHECK_ARGS+=(--fetch); fi
if [[ "$STRICT_BASE" == "1" ]]; then CHECK_ARGS+=(--strict); fi
if ! bash "$SCRIPT_DIR/check-base.sh" "${CHECK_ARGS[@]}"; then
  die "base branch check failed (--strict-base); update '$BASE' or drop --strict-base"
fi

case "$PROVIDER" in
  github) WEB_URL="https://$HOST/$REMOTE_PATH/compare/$BASE...$BRANCH?expand=1" ;;
  gitlab)
    QUERY=$(python3 -c 'import sys, urllib.parse; print("merge_request%5Bsource_branch%5D=" + urllib.parse.quote(sys.argv[1], safe="") + "&merge_request%5Btarget_branch%5D=" + urllib.parse.quote(sys.argv[2], safe=""))' "$BRANCH" "$BASE")
    WEB_URL="https://$HOST/$REMOTE_PATH/-/merge_requests/new?$QUERY"
    ;;
esac

if [[ "$DRY_RUN" == "1" ]]; then
  echo "Repository: $ROOT"
  echo "Remote:     $REMOTE ($REMOTE_URL)"
  echo "Provider:   $PROVIDER"
  echo "Base:       $BASE"
  echo "Branch:     $BRANCH"
  case "$BRANCH_ACTION" in
    create) echo "Checkout:   create branch $BRANCH from $CURRENT" ;;
    switch) echo "Checkout:   switch to existing branch $BRANCH" ;;
  esac
  if [[ "$HAS_CHANGES" == "1" ]]; then
    echo "Changes:    will commit: $MESSAGE"
  else
    echo "Changes:    none (using existing commits)"
  fi
  if [[ "$NO_PUSH" == "1" ]]; then
    echo "Push:       skipped (--no-push)"
  else
    echo "Push:       git push --set-upstream $REMOTE $BRANCH"
  fi
  echo "PR/MR:      $WEB_URL"
  exit 0
fi

case "$BRANCH_ACTION" in
  create) git switch -c "$BRANCH" ;;
  switch) git switch "$BRANCH" ;;
esac

if [[ "$HAS_CHANGES" == "1" ]]; then
  if [[ "$STAGED_ONLY" == "0" ]]; then
    git add -A
  fi
  git commit -m "$MESSAGE"
fi

if [[ "$NO_PUSH" == "0" ]]; then
  git push --set-upstream "$REMOTE" "$BRANCH"
else
  echo "Skipping push (--no-push)."
fi

TITLE="${TITLE:-$(git log -1 --pretty=%s)}"
[[ -n "$TITLE" ]] || TITLE="$BRANCH"

if [[ -z "$BODY" ]]; then
  BODY=$(git log --reverse --format='- %s' "$BASE_REF..HEAD" 2>/dev/null || true)
  [[ -n "$BODY" ]] || BODY="$TITLE"
fi

TOKEN=""
case "$PROVIDER" in
  github) TOKEN="${GH_TOKEN:-${GITHUB_TOKEN:-}}" ;;
  gitlab) TOKEN="${GITLAB_TOKEN:-${GLAB_TOKEN:-}}" ;;
esac
if [[ -z "$TOKEN" ]]; then
  TOKEN=$(printf 'protocol=https\nhost=%s\n\n' "$HOST" \
    | GIT_TERMINAL_PROMPT=0 git credential fill 2>/dev/null \
    | sed -n 's/^password=//p' | tr -d '\r' || true)
fi

if [[ -z "$TOKEN" ]]; then
  echo "No token found for $HOST." >&2
  case "$PROVIDER" in
    github) echo "Set GH_TOKEN or GITHUB_TOKEN (repo scope)." >&2 ;;
    gitlab) echo "Set GITLAB_TOKEN (api scope)." >&2 ;;
  esac
  echo "Branch '$BRANCH' is pushed; open the PR/MR manually: $WEB_URL" >&2
  exit 1
fi

OWNER=""
REPO=""
if [[ "$PROVIDER" == "github" ]]; then
  OWNER="${REMOTE_PATH%%/*}"
  REPO="${REMOTE_PATH#*/}"
  REPO="${REPO%%/*}"
fi

IFS= read -r -d '' PYCODE <<'PY' || true
import json, os, sys, urllib.error, urllib.parse, urllib.request

token = sys.stdin.readline().strip()
provider = os.environ["PR_PROVIDER"]
api = os.environ["PR_API_BASE"].rstrip("/")
title = os.environ["PR_TITLE"]
body = os.environ["PR_BODY"]
head = os.environ["PR_HEAD"]
base = os.environ["PR_BASE"]
draft = os.environ.get("PR_DRAFT") == "1"

headers = {
    "User-Agent": "terraview-open-pr",
    "Accept": "application/json",
}

if provider == "github":
    owner = os.environ["PR_OWNER"]
    repo = os.environ["PR_REPO"]
    url = f"{api}/repos/{owner}/{repo}/pulls"
    payload = {"title": title, "head": head, "base": base, "body": body}
    if draft:
        payload["draft"] = True
    headers["Authorization"] = "Bearer " + token
    headers["Accept"] = "application/vnd.github+json"
    headers["Content-Type"] = "application/json"
    data = json.dumps(payload).encode()
    url_key = "html_url"
elif provider == "gitlab":
    project = os.environ["PR_PROJECT"]
    url = f"{api}/projects/{urllib.parse.quote(project, safe='')}/merge_requests"
    mr_title = title
    if draft and not title.lower().startswith("draft:"):
        mr_title = "Draft: " + title
    payload = {
        "source_branch": head,
        "target_branch": base,
        "title": mr_title,
        "description": body,
    }
    headers["PRIVATE-TOKEN"] = token
    headers["Content-Type"] = "application/x-www-form-urlencoded"
    data = urllib.parse.urlencode(payload).encode()
    url_key = "web_url"
else:
    sys.stderr.write(f"Unsupported provider: {provider}\n")
    sys.exit(2)

request = urllib.request.Request(url, data=data, headers=headers, method="POST")
try:
    with urllib.request.urlopen(request, timeout=30) as response:
        result = json.loads(response.read().decode())
except urllib.error.HTTPError as exc:
    detail = exc.read().decode(errors="replace")
    sys.stderr.write(f"API error {exc.code}: {detail[:1000]}\n")
    sys.exit(1)
except Exception as exc:
    sys.stderr.write(f"API request failed: {exc}\n")
    sys.exit(1)

print(result.get(url_key) or result.get("html_url") or result.get("web_url") or "created")
PY

if ! printf '%s\n' "$TOKEN" | \
  PR_PROVIDER="$PROVIDER" PR_API_BASE="$API_BASE" \
  PR_OWNER="$OWNER" PR_REPO="$REPO" PR_PROJECT="$REMOTE_PATH" \
  PR_TITLE="$TITLE" PR_BODY="$BODY" PR_HEAD="$BRANCH" PR_BASE="$BASE" \
  PR_DRAFT="$DRAFT" \
  python3 -c "$PYCODE"
then
  echo "Could not create the $PROVIDER request automatically." >&2
  echo "Branch '$BRANCH' is pushed; open the PR/MR manually: $WEB_URL" >&2
  exit 1
fi
