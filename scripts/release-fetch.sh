#!/usr/bin/env bash
# Downloads a published tripcord release and checks it before anything
# is packaged from it. The homebrew, npm, and pypi workflows run this;
# so can anyone with the GitHub CLI.
#
#   GH_TOKEN=... scripts/release-fetch.sh vX.Y.Z OUT
#
# It fails unless the release for the tag is published and not a
# prerelease, its SHA256SUMS lists exactly the six release archives,
# every archive matches SHA256SUMS, and SHA256SUMS and every archive have
# a build provenance attestation signed by this repository's release.yml
# for refs/tags/vX.Y.Z. OUT then holds SHA256SUMS and the six archives.
set -euo pipefail
export LC_ALL=C

repo=vgmartinez/tripcord

die() {
	printf 'release-fetch: %s\n' "$*" >&2
	exit 1
}

[ $# -eq 2 ] || die "usage: $0 vX.Y.Z OUT"
tag=$1
out=$2
[[ $tag =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || die "tag $tag is not vX.Y.Z; prerelease tags are not packaged"
version=${tag#v}

state=$(gh release view "$tag" --repo "$repo" --json isDraft,isPrerelease --jq '"\(.isDraft) \(.isPrerelease)"')
[ "$state" = "false false" ] || die "release $tag is not a published, non-prerelease release (isDraft isPrerelease = $state)"

mkdir -p -- "$out"
[ -z "$(ls -A -- "$out")" ] || die "$out is not empty"
gh release download "$tag" --repo "$repo" --dir "$out" --pattern SHA256SUMS --pattern "tripcord_${version}_*"

cd -- "$out"
want=$(printf '%s\n' \
	"tripcord_${version}_darwin_amd64.tar.gz" \
	"tripcord_${version}_darwin_arm64.tar.gz" \
	"tripcord_${version}_linux_amd64.tar.gz" \
	"tripcord_${version}_linux_arm64.tar.gz" \
	"tripcord_${version}_windows_amd64.zip" \
	"tripcord_${version}_windows_arm64.zip")
listed=$(awk '{ print $2 }' SHA256SUMS | sort)
[ "$listed" = "$want" ] || die "SHA256SUMS does not list exactly the six ${tag} archives"
have=$(printf '%s\n' tripcord_* | sort)
[ "$have" = "$want" ] || die "the release does not have exactly the six ${tag} archives"
sha256sum --strict -c SHA256SUMS

for f in SHA256SUMS tripcord_*; do
	gh attestation verify "$f" --repo "$repo" \
		--signer-workflow "${repo}/.github/workflows/release.yml" \
		--source-ref "refs/tags/${tag}" \
		--deny-self-hosted-runners >/dev/null
	printf 'release-fetch: %s: provenance verified (release.yml at refs/tags/%s)\n' "$f" "$tag"
done
