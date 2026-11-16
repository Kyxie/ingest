#!/usr/bin/env bash
# Tests create commits/tags only in disposable repositories with local file remotes.
set -euo pipefail
release_script="$(cd "$(dirname "$0")" && pwd)/release.sh"
test_root="$(mktemp -d)"
trap 'rm -rf "$test_root"' EXIT
export GIT_CONFIG_GLOBAL=/dev/null
export GIT_CONFIG_NOSYSTEM=1

git init --quiet --bare "$test_root/origin.git"
git init --quiet --initial-branch=main "$test_root/repo"
cd "$test_root/repo"
git config --local user.name Kyxie
git config --local user.email kyriexie@outlook.com
git config --local commit.gpgsign false
git config --local tag.gpgsign false
git remote add origin "$test_root/origin.git"
git commit --quiet --allow-empty -m 'test: initialize release fixture'
git push --quiet origin main
initial_head="$(git rev-parse HEAD)"

expect_failure() {
  local message=$1
  shift
  if bash "$release_script" "$@" >"$test_root/output" 2>&1; then
    echo "Release unexpectedly succeeded: $message" >&2
    exit 1
  fi
  if ! grep -Fq "$message" "$test_root/output"; then
    cat "$test_root/output" >&2
    exit 1
  fi
}

git config --local user.email other@example.invalid
expect_failure 'Git identity must be'
git config --local user.email kyriexie@outlook.com
git config --local user.name Other
expect_failure 'Git identity must be'
git config --local user.name Kyxie
expect_failure 'Usage:' invalid
touch untracked
expect_failure 'Commit or remove working tree changes'
rm untracked
git checkout --quiet --detach
expect_failure 'Release from a branch'
git checkout --quiet main
test -z "$(git tag --list)"

for expectation in 'patch v0.0.1' 'minor v0.1.0' 'major v1.0.0'; do
  read -r bump expected <<< "$expectation"
  actual="$(bash "$release_script" "$bump" 2>"$test_root/push-output")"
  test "$actual" = "$expected"
  test "$(git rev-parse "$expected")" = "$(git rev-parse HEAD)"
  test "$(git ls-remote origin "refs/tags/$expected" | awk '{print $1}')" = "$(git rev-parse HEAD)"
done

# A branch ahead of origin is published together with its new tag.
git commit --quiet --allow-empty -m 'test: advance local fixture'
test "$(bash "$release_script" patch 2>"$test_root/push-output")" = v1.0.1
test "$(git ls-remote origin refs/heads/main | awk '{print $1}')" = "$(git rev-parse HEAD)"

# Move only the disposable remote to simulate a behind/diverged branch.
git update-ref refs/heads/advanced HEAD
git checkout --quiet --detach "$initial_head"
git branch --force main "$initial_head"
git checkout --quiet main
expect_failure 'behind or diverged'
git commit --quiet --allow-empty -m 'test: diverge local fixture'
expect_failure 'behind or diverged'
git checkout --quiet --detach advanced
git branch --force main advanced
git checkout --quiet main

# A server hook rejects the tag: atomic push must preserve the remote branch.
cat > "$test_root/origin.git/hooks/pre-receive" <<'HOOK'
#!/bin/sh
exit 1
HOOK
chmod +x "$test_root/origin.git/hooks/pre-receive"
remote_before="$(git ls-remote origin refs/heads/main | awk '{print $1}')"
git commit --quiet --allow-empty -m 'test: rejected fixture push'
expect_failure 'local release tag was removed' patch
test -z "$(git tag --list v1.0.2)"
test -z "$(git ls-remote origin refs/tags/v1.0.2)"
test "$(git ls-remote origin refs/heads/main | awk '{print $1}')" = "$remote_before"
echo 'Release tests passed: identity, guards, versions, branch state, atomic rollback.'
