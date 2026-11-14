# AGENTS.md

Guidance for AI agents working in this repository.

## Overview

`compose2nix` is a Go CLI that converts Docker Compose projects into NixOS
`oci-container` configuration. Core logic lives in the top-level Go files
(`compose.go`, `nix.go`, `systemd.go`, `template.go`, `main.go`, etc.).

## Build & test

```sh
make build     # build the binary into bin/
make test      # go test -v
make flake     # nix build of the default package
```

The version string lives in both `flake.nix` and `main.go`; always update both
locations together.

## Cut a release

Releases use `v0.X.Y` tags. At any given time the version in `flake.nix` and
`main.go` is the next release with a `-pre` suffix (e.g. `0.3.5-pre`). Cutting a
release drops the suffix, tags it, and bumps to the next `-pre`.

To cut release `v0.X.Y` (where `0.X.Y-pre` is the current in-tree version):

1. Create a release branch:

   ```sh
   git checkout -b release-v0.X.Y
   ```

2. **Release commit** — drop the `-pre` suffix. Update `version` in `flake.nix`
   and `appVersion` in `main.go` from `0.X.Y-pre` to `0.X.Y`, then:

   ```sh
   git commit -am "chore: bump to v0.X.Y"
   ```

3. **Pre-release commit** — bump to the next pre-release. Update both version
   strings from `0.X.Y` to `0.X.(Y+1)-pre`, then:

   ```sh
   git commit -am "v0.X.(Y+1) pre-release"
   ```

4. Push the branch and open a PR:

   ```sh
   git push -u origin release-v0.X.Y
   ```

5. Merge the PR into `main`, then pull `main` locally.

   > **Tag after merge, not before.** This repo uses **rebase merge**, which
   > replays the branch commits onto `main` under *new* SHAs. A tag created
   > before merge would point at the pre-merge commit, which never lands on
   > `main`. Always merge and pull first, then tag.

6. Tag the **release commit** (the `chore: bump to v0.X.Y` commit on `main`, not
   the pre-release commit on top of it) with a lightweight tag, then push it:

   ```sh
   git log --oneline -3 origin/main   # find "chore: bump to v0.X.Y"
   git tag v0.X.Y <release-commit-sha>
   git push origin v0.X.Y
   ```

### Example

Cutting `v0.3.5` when the tree is at `0.3.5-pre`:

- Commit 1 (`chore: bump to v0.3.5`): `0.3.5-pre` → `0.3.5`
- Commit 2 (`v0.3.6 pre-release`): `0.3.5` → `0.3.6-pre`
- Merge the PR, pull `main`, then tag `v0.3.5` on the `chore: bump to v0.3.5`
  commit and push the tag.
