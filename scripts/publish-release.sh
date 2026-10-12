#!/usr/bin/env bash
set -euo pipefail

# This is a publishing entry point, not a local build/check command.
[[ ${GITHUB_ACTIONS:-} == true && ${GITHUB_EVENT_NAME:-} == pull_request_target ]]
[[ ${RELEASE_PR:-} =~ ^[1-9][0-9]*$ ]]
: "${GH_TOKEN:?GitHub Actions token is required}"
repo=s4na/fragment
[[ ${GITHUB_REPOSITORY:-} == "$repo" ]]
root=$(git rev-parse --show-toplevel)
cd "$root"
mkdir -p bin/release
git config user.name 'github-actions[bot]'
git config user.email '41898282+github-actions[bot]@users.noreply.github.com'
git fetch origin main --tags

# An annotated tag is the durable reservation for this PR. Retries resume it.
tag=$(git for-each-ref --format='%(refname:short) %(contents:subject)' refs/tags |
  awk -v pr="$RELEASE_PR" '$2 == "fragment-release-pr:" && $3 == pr {print $1}')
if [[ -n "$tag" ]]; then
  [[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]
  # A later release has already replaced this one; never roll the tap back.
  latest=$(git tag --list | go run ./scripts/release latest)
  if [[ "$tag" != "$latest" ]]; then
    echo "PR #$RELEASE_PR is superseded by $latest"
    exit 0
  fi
  commit=$(git rev-parse "$tag^{commit}")
else
  tag=$(git tag --list | go run ./scripts/release next)
  # Queue order can differ from merge order. Always package the current main.
  commit=$(git rev-parse origin/main)
fi
git checkout --detach "$commit"
output="bin/release/$tag"
mkdir -p "$output"

# Never replace assets in an already published release. Use their actual bytes
# when recovering a failed Formula push, even if the build tools have changed.
if gh release view "$tag" --repo "$repo" --json isDraft --jq .isDraft > bin/release/is-draft; then
  draft=$(cat bin/release/is-draft)
else
  # An API/network/permission failure must not be treated as an absent release.
  gh api "repos/$repo/releases?per_page=100" > bin/release/releases.json
  if node -e 'const a=JSON.parse(require("fs").readFileSync(process.argv[1])); process.exit(a.some(r=>r.tag_name===process.argv[2])?0:1)' bin/release/releases.json "$tag"; then
    echo 'Release lookup failed for an existing release' >&2
    exit 1
  fi
  draft=missing
fi
if [[ "$draft" == false ]]; then
  gh release download "$tag" --repo "$repo" --pattern 'fragment_*.tar.gz' --dir "$output" --clobber
  go run ./scripts/release formula --version "$tag"
else
  sfw pnpm --dir web install --frozen-lockfile
  sfw pnpm --dir web build
  go run ./scripts/release build --version "$tag" --commit "$commit"
  if ! git show-ref --verify --quiet "refs/tags/$tag"; then
    git tag -a "$tag" "$commit" -m "fragment-release-pr: $RELEASE_PR"
    git push origin "refs/tags/$tag"
  fi
  if [[ "$draft" == missing ]]; then
    gh release create "$tag" --repo "$repo" --verify-tag --draft --title "$tag" --notes "PR #$RELEASE_PR のマージ後のmainを配布します。ビルド元: $commit"
  fi
  gh release upload "$tag" "$output"/*.tar.gz "$output/checksums.txt" --repo "$repo" --clobber
  gh release edit "$tag" --repo "$repo" --draft=false --latest
fi

# Publish the Formula only after every archive is available. Retry a normal
# fast-forward push if another PR was merged meanwhile; never force push.
formula="$root/$output/fragment.rb"
for attempt in 1 2 3 4 5; do
  git fetch origin main
  git checkout --detach origin/main
  mkdir -p Formula
  cp "$formula" Formula/fragment.rb
  git add Formula/fragment.rb
  if git diff --cached --quiet; then
    exit 0
  fi
  git commit -m "Homebrewの配布バージョンを $tag に更新する"
  if git push origin HEAD:main; then
    exit 0
  fi
  sleep "$attempt"
done
echo 'Release is published, but the Formula push failed. Re-run this job.' >&2
exit 1
