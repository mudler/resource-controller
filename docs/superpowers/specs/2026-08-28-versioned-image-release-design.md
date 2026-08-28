# Versioned image release design

## Goal

Publish the first `resource-controller` release as `v0.1.0`. Configure the
infra repository to deploy released controller images and follow later SemVer
tags through Flux image automation.

## Current state

The publish workflow builds controller and worker images from `master`. It
publishes the rolling `master` and `edge` tags. The workflow also defines
SemVer tags, but the repository has no Git tags or GitHub releases.

The worker Dockerfile copies `rc` from
`ghcr.io/mudler/resource-controller:edge`. A tag build can therefore combine a
tagged worker image with a controller binary from another commit.

The infra deployment uses `ghcr.io/mudler/resource-controller:edge`. It has no
Flux `ImageRepository` or `ImagePolicy` for this image.

## Release workflow change

Add an `RC_IMAGE` build argument to the worker Dockerfile. Keep `edge` as the
default so local builds keep their current behavior.

The publish workflow passes the controller manifest digest to the worker
build. Buildx selects the correct platform from that manifest. The controller
and worker images therefore contain the same `rc` revision on branch and tag
builds.

Run the repository test suite before the workflow change reaches `master`.
Push the change and require green `ci` and `publish` runs for that commit.

Create and push the annotated `v0.1.0` tag from the verified `master` commit.
The existing metadata rules publish these image tags:

- `ghcr.io/mudler/resource-controller:0.1.0`
- `ghcr.io/mudler/resource-controller:0.1`
- `ghcr.io/mudler/resource-controller:0`
- `ghcr.io/mudler/resource-controller:latest`
- The same tag set under `ghcr.io/mudler/rc-worker`

Wait for the tag-triggered publish workflow. Create a GitHub release for the
existing tag after the workflow succeeds.

## Infra automation change

Add `versions/resource-controller.yaml` with these Flux resources:

- An `ImageRepository` that scans
  `ghcr.io/mudler/resource-controller` once per minute.
- A `resource-controller-semver` `ImagePolicy` with a range of `>=0.1.0`.

Register the file in `versions/kustomization.yaml`.

Change the development controller manifest from `edge` to `0.1.0`. Add the
`flux-system:resource-controller-semver` image-policy marker to that line.
Remove the `Always` pull policy and the explanation for the rolling tag. A new
SemVer tag changes the pod template and starts a rollout without mutable-tag
cache behavior.

This deployment intentionally follows tagged releases although other
development applications follow branch builds. The controller and workers
must upgrade together, so release tags are the safer unit for this service.

## Safety and repository state

The infra checkout contains unrelated edits and untracked secret files. Do
not stage, modify, stash, or commit those files. Update the checkout from
`origin/main` only when Git can preserve those changes without a conflict.

Commit the release workflow fix in `resource-controller` before creating the
tag. Commit the Flux policy and manifest update in `infra` after the `0.1.0`
image exists.

## Verification

Run these checks before completion:

- `go test ./... -race -timeout 20m`
- A YAML and action syntax check for the modified workflow
- A Dockerfile build or equivalent Buildx validation for the worker argument
- `kustomize build versions`
- `kustomize build manifests/development`
- Confirmation that the tag publish workflow succeeds
- Confirmation that the `0.1.0` controller image exists in GHCR
- A final diff and status check in both repositories
