#!/usr/bin/env bash
# Reproducible release archives of tripcord. release.yml runs this; so
# can anyone who wants to rebuild a release and compare SHA256SUMS.
#
#   scripts/release-build.sh build OUT VERSION GOOS/GOARCH...
#   scripts/release-build.sh sums OUT
#   scripts/release-build.sh notes VERSION
#
# build compiles ./cmd/tripcord at HEAD for each platform with
# CGO_ENABLED=0, -trimpath, and -ldflags "-s -w -buildid=
# -X main.version=VERSION -X main.commit=<HEAD>". It fails unless
# NOTICE and THIRD_PARTY_NOTICES name every module go version -m lists
# for that binary, at that version. It packs the binary with LICENSE,
# NOTICE, and THIRD_PARTY_NOTICES into OUT/tripcord_<version>_<os>_<arch>
# .tar.gz, or .zip for windows, and prints the archive name. Every
# entry has the HEAD commit time (SOURCE_DATE_EPOCH), mode 0755 or
# 0644, owner 0:0, and the same order, so one commit and one Go
# toolchain give the same bytes. The worktree must be clean: an
# uncommitted file could change the build.
#
# sums writes OUT/SHA256SUMS for every archive in OUT, sorted by name.
#
# notes prints the "## VERSION" section of docs/release-notes.md, the
# release body, with links to other docs made absolute at that tag.
set -euo pipefail
export LC_ALL=C TZ=UTC
umask 022

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
repo=vgmartinez/tripcord
version_re='^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$'
platforms=(linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64)

die() {
	printf 'release-build: %s\n' "$*" >&2
	exit 1
}

check_version() {
	[[ $1 =~ $version_re ]] || die "version $1 is not vX.Y.Z or vX.Y.Z-<prerelease>"
}

# check_notices fails unless NOTICE and THIRD_PARTY_NOTICES name every
# module the binary links.
check_notices() {
	local bin=$1 kind path ver rest missing=0
	while read -r kind path ver rest; do
		[ "$kind" = dep ] || continue
		if ! grep -A1 -xF "Module: ${path}" "$root/THIRD_PARTY_NOTICES" | grep -qxF "Version: ${ver}"; then
			printf 'release-build: THIRD_PARTY_NOTICES has no Module: %s / Version: %s entry\n' "$path" "$ver" >&2
			missing=1
		fi
		if ! grep -qF "  ${path} ${ver} (" "$root/NOTICE"; then
			printf 'release-build: NOTICE does not list %s %s\n' "$path" "$ver" >&2
			missing=1
		fi
	done < <(go version -m "$bin")
	[ "$missing" -eq 0 ] || die "$bin links a module the notices do not cover"
}

build() {
	[ $# -ge 3 ] || die "usage: build OUT VERSION GOOS/GOARCH..."
	local out=$1 version=$2
	shift 2
	check_version "$version"
	cd "$root"
	[ -z "$(git status --porcelain)" ] || die "the worktree has uncommitted changes"
	local commit epoch
	commit=$(git rev-parse --verify 'HEAD^{commit}')
	epoch=$(git log -1 --format=%ct HEAD)
	export SOURCE_DATE_EPOCH=$epoch
	mkdir -p -- "$out"
	out=$(CDPATH='' cd -- "$out" && pwd)
	local p goos goarch bin name stage
	for p in "$@"; do
		local known=0 q
		for q in "${platforms[@]}"; do
			[ "$p" = "$q" ] && known=1
		done
		[ "$known" -eq 1 ] || die "platform $p is not one of ${platforms[*]}"
		goos=${p%/*}
		goarch=${p#*/}
		bin=tripcord
		name="tripcord_${version#v}_${goos}_${goarch}.tar.gz"
		if [ "$goos" = windows ]; then
			bin=tripcord.exe
			name="tripcord_${version#v}_${goos}_${goarch}.zip"
		fi
		stage=$(mktemp -d)
		CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch GOFLAGS=-mod=readonly \
			go build -trimpath \
			-ldflags "-s -w -buildid= -X main.version=${version} -X main.commit=${commit}" \
			-o "$stage/$bin" ./cmd/tripcord
		check_notices "$stage/$bin"
		cp LICENSE NOTICE THIRD_PARTY_NOTICES "$stage/"
		chmod 0755 "$stage/$bin"
		chmod 0644 "$stage/LICENSE" "$stage/NOTICE" "$stage/THIRD_PARTY_NOTICES"
		touch -d "@${epoch}" "$stage/$bin" "$stage/LICENSE" "$stage/NOTICE" "$stage/THIRD_PARTY_NOTICES"
		rm -f -- "${out:?}/$name"
		if [ "$goos" = windows ]; then
			(cd "$stage" && zip -q -X -D -9 "$out/$name" "$bin" LICENSE NOTICE THIRD_PARTY_NOTICES)
		else
			(cd "$stage" && tar --format=ustar --no-recursion --owner=0 --group=0 --numeric-owner \
				--mtime="@${epoch}" -cf - "$bin" LICENSE NOTICE THIRD_PARTY_NOTICES) | gzip -n -9 > "$out/$name"
		fi
		rm -rf -- "$stage"
		printf '%s\n' "$name"
	done
}

sums() {
	[ $# -eq 1 ] || die "usage: sums OUT"
	cd -- "$1"
	local files=(tripcord_*.tar.gz tripcord_*.zip)
	local f
	for f in "${files[@]}"; do
		[ -f "$f" ] || die "no archive matches $f in $1"
	done
	mapfile -t files < <(printf '%s\n' "${files[@]}" | sort)
	sha256sum -- "${files[@]}" > SHA256SUMS
	cat SHA256SUMS
}

notes() {
	[ $# -eq 1 ] || die "usage: notes VERSION"
	local version=$1
	check_version "$version"
	local body
	body=$(awk -v want="## ${version}" '
		$0 == want || index($0, want " ") == 1 { on = 1; next }
		on && /^## / { exit }
		on { print }
	' "$root/docs/release-notes.md")
	[ -n "$(printf '%s' "$body" | tr -d '[:space:]')" ] || die "docs/release-notes.md has no \"## ${version}\" section"
	printf '%s\n' "$body" | sed -E "s#\]\(([A-Za-z0-9_./-]+\.md)(\#[A-Za-z0-9_-]+)?\)#](https://github.com/${repo}/blob/${version}/docs/\1\2)#g"
}

cmd=${1:-}
[ $# -gt 0 ] && shift
case "$cmd" in
build) build "$@" ;;
sums) sums "$@" ;;
notes) notes "$@" ;;
*) die "usage: $0 build OUT VERSION GOOS/GOARCH... | sums OUT | notes VERSION" ;;
esac
