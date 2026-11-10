# Installation

tripcord is one static binary. There is no npm, PyPI, Homebrew, or
Docker package.

## From a release

Each release on the
[Releases](https://github.com/vgmartinez/tripcord/releases) page has six
archives and a `SHA256SUMS` file:

| Platform | Archive |
| --- | --- |
| Linux x86-64 | `tripcord_<version>_linux_amd64.tar.gz` |
| Linux arm64 | `tripcord_<version>_linux_arm64.tar.gz` |
| macOS x86-64 | `tripcord_<version>_darwin_amd64.tar.gz` |
| macOS arm64 | `tripcord_<version>_darwin_arm64.tar.gz` |
| Windows x86-64 | `tripcord_<version>_windows_amd64.zip` |
| Windows arm64 | `tripcord_<version>_windows_arm64.zip` |

Each archive holds the `tripcord` binary (`tripcord.exe` on Windows),
`LICENSE`, `NOTICE`, and `THIRD_PARTY_NOTICES`. `<version>` has no
leading `v`: the `v0.1.0` release has `tripcord_0.1.0_linux_amd64.tar.gz`.

Download an archive and `SHA256SUMS`, check the checksum, and verify
the build provenance with the [GitHub CLI](https://cli.github.com/):

```sh
tag=v0.1.0
file="tripcord_${tag#v}_linux_amd64.tar.gz"
gh release download "$tag" --repo vgmartinez/tripcord --pattern "$file" --pattern SHA256SUMS
sha256sum -c SHA256SUMS --ignore-missing
gh attestation verify "$file" --repo vgmartinez/tripcord
tar -xzf "$file" tripcord
./tripcord --version
```

`sha256sum -c SHA256SUMS` checks every archive; `--ignore-missing`
skips the ones you did not download. On macOS, use
`shasum -a 256 -c SHA256SUMS --ignore-missing`. `gh attestation verify`
checks that the archive was built by this repository's `release`
workflow from the tagged commit. `tripcord --version` prints the tag
and the commit, for example `v0.1.0 (commit <sha>)`.

### Rebuilding a release

The release build is reproducible: the same commit and Go toolchain
give byte-identical archives. To check a release against its source:

```sh
git clone https://github.com/vgmartinez/tripcord.git
cd tripcord
git checkout v0.1.0
scripts/release-build.sh build "$PWD/../dist" v0.1.0 \
  linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64
scripts/release-build.sh sums "$PWD/../dist"
```

Then compare `../dist/SHA256SUMS` with the release's `SHA256SUMS`. The
script needs the Go version in `go.mod`, GNU `tar`, `gzip`, and `zip`,
and a clean worktree.

## With `go install`

With Go 1.27.3 or newer:

```sh
go install github.com/vgmartinez/tripcord/cmd/tripcord@main
```

`@v0.1.0` installs that release's source instead. Either way,
`tripcord version` prints `dev`: the version and commit are only set at
link time, by the release build
([Development](development.md#module-and-version)).

## From a checkout

```sh
git clone https://github.com/vgmartinez/tripcord.git
cd tripcord
CGO_ENABLED=0 go build -trimpath -o tripcord ./cmd/tripcord
```

## In GitHub Actions

The [GitHub Action](github-action.md) and the reusable workflows build
tripcord from source at the commit they pin. They do not download a
release archive.
