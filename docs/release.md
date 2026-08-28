# Release

Each release is triggered by a Git tag and publishes the driver image and/or the Helm chart from the tagged commit.

## The three versions

| Element | Format | Where |
|---------|--------|-------|
| Application image | `vX.Y.Z[-PRERELEASE]` | Docker tag, value of `appVersion` |
| Chart | `X.Y.Z[-PRERELEASE]` (no `v` prefix) | `version` in `Chart.yaml`, independent of the app |
| Release tag | `vAPP[-N]` | Git tag that triggers the CI; `N` is the number of chart-only releases for that app version |

The chart version is **decoupled** from the app version: it follows its own semver. The chart's `appVersion` is the version of the driver deployed by default (`image.tag` is empty by default, so the deployment uses `appVersion` as the image tag; users can override with `--set image.tag=...`).

Application prereleases use the standard SemVer form, for example `v1.0.0-rc.1`. A prerelease tag publishes both the image and the chart; only a positive numeric suffix such as `v1.0.0-1` denotes a chart-only release.

## Chart version bump

The chart `version` reflects changes to the chart itself:

| Situation | Bump |
|-----------|------|
| App release, only `appVersion` changes | patch |
| Chart fix (values, templates) | patch |
| Backward-compatible chart feature (new value, new option) | minor |
| Breaking chart change (renamed value, changed default) | major |

Rules:

- An app release always bumps the chart by a **patch**, even if the app itself is a minor or major release. The app version change is carried by `appVersion` and the image tag; if you want to control the app version, pin it with `image.tag`.
- The `version` bump happens in the release commit (the one carrying the tag), not in every pull request. Several pull requests can accumulate chart changes; the last one before tagging applies the bump according to the table. If the bump is forgotten, the OCI registry rejects the duplicate chart version at release time.

## Case 1: App release

The driver code changes, with or without chart changes.

1. In `Chart.yaml`, set `appVersion` to the new app version `vX.Y.Z`. Bump `version`: patch if the chart is otherwise unchanged, otherwise according to the table above.
2. Generate the release files:

   ```shell
   TAG=v1.0.0-rc.1 make docs
   RELEASE_TAG=v1.0.0-rc.1 make release-update
   ```

3. Merge into `main`, then create and push the tag:

   ```shell
   git tag vX.Y.Z
   git push origin vX.Y.Z
   ```

4. The CI builds and pushes the image `vX.Y.Z`, signs it, and publishes the chart.

> Example: app is at `v0.4.0`, chart `version` is `0.4.1`. A bugfix app release `v0.4.1` with no chart change → `appVersion: v0.4.1`, `version: 0.4.2`, tag `v0.4.1`.
>
> Example: a new app release `v0.5.0` that also adds a new chart value → `appVersion: v0.5.0`, `version: 0.5.0` (feature = minor), tag `v0.5.0`.

## Case 2: Chart-only release

Only `charts/**` changes, the application is unchanged.

1. In `Chart.yaml`, bump `version` according to the table above. Leave `appVersion` unchanged.
2. Regenerate the chart documentation:

   ```shell
   make docs
   ```

3. Merge into `main`, then tag with `N`, the next unused increment for the current app version:

   ```shell
   git tag v<app>-N
   git push origin v<app>-N
   ```

4. The CI publishes the chart only; the image is not rebuilt.

Conditions:

- The app image `v<app>` must already have been released.
- The pull request must not contain application code changes.
- If a chart change is pending when you prepare an app release, fold it into the app release (case 1) instead of tagging a chart-only release first.

> Example (current state): app `v0.4.0` is already released, the chart `version` is `0.4.1` (a chart fix is pending). Tag `v0.4.0-1` publishes chart `0.4.1` without touching the image.
>
> Example: after the app release `v0.5.0` (case 1), a chart bugfix → bump `version`, tag `v0.5.0-1`.
