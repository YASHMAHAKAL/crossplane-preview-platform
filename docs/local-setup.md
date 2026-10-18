# Repeat the local preview platform setup

This runbook recreates the **local control plane** for the existing `YASHMAHAKAL/crossplane-preview-platform` source repository and the separate private `YASHMAHAKAL/preview-gitops` repository. It does not create a cloud deployment. The bootstrap reuses an existing `kind-preview-platform` cluster and fails when an installed pinned version differs, rather than changing an unrelated or newer installation. This is an owner-specific setup guide; changing only the evaluator's `repository` setting does not make a fork work end to end.

## Prerequisites and access

- Linux with a user systemd manager, Docker access, and enough resources for kind, Crossplane, Argo CD, ingress, and one vCluster. The measured host had a 12 CPU, 15 GiB allocatable kind node; smaller hosts have not been verified.
- `kind`, `kubectl`, Helm, Go, Node 22 or 24, Python 3, `gh`, Git, and `curl` on `PATH`. The kind node image, Crossplane 2.4.0, Argo CD v3.5.2, provider-helm 1.2.0, and NGINX ingress 2.7.3 are pinned in the project files and bootstrap. `kind` v0.33.0 was used for the last verification.
- `gh auth login` with access to source PR metadata and GitHub Actions artifacts. PR write access is needed to open source changes. The watcher obtains the GitHub token from `gh` at service start; no token goes into Git.
- SSH read/write access to `git@github.com:YASHMAHAKAL/preview-gitops.git` for the evaluator's private checkout. Argo CD uses a **separate read-only deploy key** for that repository. The source repository and its GHCR app/Function packages must be readable by the cluster.
- A private `preview-gitops` repository on `main` containing `previews/.gitkeep` and `status/.gitkeep`. Its SSH credential must be able to push; the Argo deploy key needs read access only. The source repository, released Function package, and app image are currently public. GitHub repository visibility does not automatically set GHCR package visibility.
- Free local ports `34433` (kind API), `8088` (preview ingress), `8090` (status API), `3000` (Backstage frontend), and `7007` (Backstage backend). `kind.yaml` and the portal configuration fix these ports; preflight currently checks port `8088` before creating a cluster, while the other conflicts can surface later.

## Configuration inventory

The following values are tied to this demonstration. They are consistent in the committed files and need no edits when installing on another machine **with access to the same GitHub repositories**.

| Value | Where it is fixed | What to do for a different owner or repository |
| --- | --- | --- |
| `YASHMAHAKAL/crossplane-preview-platform` and trusted author | Evaluator config, Backstage catalog/templates, PR URL handling, teardown, and source links | Update all source references together. A private evaluator config alone changes the watcher but leaves Backstage and teardown pointed at the original repository. |
| `git@github.com:YASHMAHAKAL/preview-gitops.git` | Preflight, bootstrap, service installer, Argo CD ApplicationSet, and verification | Create a separate trusted GitOps repository, then update every consumer before bootstrap. Do not point Argo CD at source PR branches. |
| `ghcr.io/yashmahakal/function-preview-resources:v0.1.7` | Host Function manifest and isolation gate | Publish a Function package for the new owner, make it pullable by kind, and update the pinned reference. App images and candidate packages are published by the GitHub Actions workflows for the current source repository. |
| `preview-platform`, `kind-preview-platform`, and localhost ports | kind config, local scripts, Backstage cluster catalog, and portal config | Keep these names and ports for the documented flow; changing them requires coordinated edits. |
| Operator state under `~/.local` and the Argo key under `~/.config` | Local scripts and the key creation step below | These follow the current user's home directory. They are not committed to Git. |

Fork portability is a separate implementation task; this runbook verifies a fresh installation of the **current** owner-specific demo. To rehearse it without disrupting the running demo, use another machine or a clean OS environment with its own Docker daemon and ports. Do not delete the existing kind cluster merely to test bootstrap.

On a **new** cluster, create an Argo deploy key outside the project checkout, add its `.pub` file to the private GitOps repository's GitHub **Settings → Deploy keys** with write access disabled, then set its path:

```sh
mkdir -p ~/.config/crossplane-preview-platform
ssh-keygen -t ed25519 -N '' -f ~/.config/crossplane-preview-platform/argocd-gitops
export PREVIEW_ARGO_REPO_KEY_FILE="$HOME/.config/crossplane-preview-platform/argocd-gitops"
```

The bootstrap reads that key from disk only when the cluster lacks the named Argo repository Secret. It will not print the key or replace an existing Secret. Keep the private key outside Git and restrict it to your user. On an existing cluster with the configured Secret, the variable is not needed.

## Start and verify

From the repository root:

```sh
deploy/local/preflight.sh
deploy/local/bootstrap.sh
```

Preflight checks tools, Node version, Docker, GitHub authentication, private GitOps SSH access, and either the existing kind context or the deploy key and free host port 8088. Bootstrap creates only the `preview-platform` kind cluster when absent, installs the pinned controllers, configures Argo's private GitOps access, applies the Crossplane and Argo manifests, and installs the user watcher/status services. It waits for controller and watcher health before returning. The evaluator checkout is at `~/.local/share/crossplane-preview-platform/preview-gitops`; its local heartbeat and MCP token are under `~/.local/state/crossplane-preview-platform/`. Rerunning bootstrap on the same pinned installation is supported.

Start Backstage in a **separate terminal**:

```sh
deploy/local/start-portal.sh
```

The launcher installs the committed Yarn lockfile, runs the TypeScript check, obtains the existing `gh` credential, and starts the portal in the foreground at `http://localhost:3000`. It uses `MCP_TOKEN` or `PREVIEW_MCP_TOKEN` if one is already set; otherwise it creates a random token in the private state directory. It never prints the token. To prepare dependencies without starting the portal, run `deploy/local/start-portal.sh --prepare-only`.

In another terminal, run:

```sh
deploy/local/verify-platform.sh --portal
```

This read-only gate checks the kind node; Crossplane, provider, Function, XRD, and Compositions; Argo CD's ApplicationSet and private repository Secret; ingress routing; watcher heartbeat; status API; Backstage frontend; and the authenticated Incident Tracker Component catalog read. Run `deploy/local/verify-platform.sh` without `--portal` if Backstage is intentionally stopped. A successful check ends with two `verify:` lines. On failure, it names the component to inspect. Use `journalctl --user -u preview-watcher -u preview-status-api -f` for the local services and `kubectl --context kind-preview-platform -n argocd get applicationset preview-environments -o yaml` for GitOps generation errors.

For Codex MCP, launch a **new** Codex session with the same token that Backstage uses:

```sh
export PREVIEW_MCP_TOKEN="$(cat "$HOME/.local/state/crossplane-preview-platform/mcp-token")"
codex mcp list
```

If you supplied `MCP_TOKEN` or `PREVIEW_MCP_TOKEN` yourself, export that exact value instead. The committed [.codex/config.toml](../.codex/config.toml) references the variable name only. Avoid printing or committing its value.

## Repeat the user flow

1. Change an Incident Tracker app file on a new branch and open a PR against `main`. No preview request file, template, or mode selection is needed. Backstage Catalog and Codex MCP can discover the service; the current MCP actions are read-only.
2. Wait for the source `Preview image` CI run for that PR head. Enter the PR number on the Backstage **PR Previews** page, or ask Codex to call `preview.get-status`. The Go evaluator selects a namespace from the app diff. `ready` includes an Incident Tracker URL only after the current-head XR and app route are healthy. Open the URL and confirm your change appears.
   Run `deploy/local/verify-preview.sh <PR number> namespace` or `deploy/local/verify-preview.sh <PR number> vcluster` to check the status, matching XR head and mode, and the live app route independently.
3. For a bounded deployment preview, change [the Incident Tracker deployment config](deployment-preview.md) in a direct PR. The evaluator selects a namespace and applies the validated replicas/resources there. Run `deploy/local/verify-preview.sh <PR number> namespace`, then inspect the namespaced Deployment. Adding `critical` to the [baseline IncidentPolicy CRD](incident-policy-preview.md) severity enum selects a vCluster, including when mixed with the deployment config. Supported Crossplane source changes are documented below; other CRD, deployment, and Crossplane edits remain rejected. The [live PR evidence](live-pr-verification.md) records earlier CRD checks.
4. Close or merge the PR. Wait for `deleted`/`cleanup-verified`; confirm its Argo Application, XR, namespace, volume, and route are absent. [Reliability evaluation](reliability-evaluation.md) records raw readiness and cleanup times, including recovery after a watcher outage.
   Run `deploy/local/verify-preview.sh <PR number> deleted` for the read-only absence check.

The setup and verification commands do **not** open or close PRs. A fresh-machine install path was not exercised during the 2026-10-08 implementation because the existing cluster was retained; the idempotent bootstrap, portal preparation/start, authenticated catalog check, and control-plane gate passed on that cluster. The previous real PR trials prove both preview modes and cleanup. Someone without access to the private GitOps repository can run the offline `make verify` and render checks, but cannot run this owner-specific end-to-end GitHub demo until the repository references and package publication are adapted as listed above.

## Check Crossplane candidate isolation

Run `deploy/local/verify-crossplane-isolation.sh` while the host platform is healthy and no other isolation gate is running. It creates a temporary `crossplane-candidate-spike` host namespace and vCluster OSS 0.36.0, installs Crossplane 2.4.0 and the released Function **inside** the virtual API, then applies this checkout's XRD and Compositions. After the virtual service account's composed-resource permissions have aggregated, it applies the namespace XR fixture and waits for `Synced=True`. It checks the composed namespace, ResourceQuota, Deployment, Service, and Ingress inside the virtual API; verifies that neither the XR nor namespace appeared on the host; deletes the XR and waits for its namespace to disappear; and confirms that the host Function package reference did not change. The script then removes its vCluster and host namespace, checking that no persistent volume remains. The fixture uses a deliberately fake image digest, so workload readiness and a preview URL are outside this gate.

To test a Function candidate that is already published at an immutable OCI digest, pass that package reference as the first argument. Only do this for trusted candidate packages; the Function runs code in the local cluster's shared nodes. This manual script does **not** build a package from a PR head or verify package provenance. Automatic previews support exact-head Go Function source edits, the bounded namespace Composition pod quota edit, and the bounded XRD reason-code constraint; other Crossplane source edits still fail closed.

For a same-repository source PR, use the [exact-head candidate guide](crossplane-candidate-gate.md) to run the additional disposable gate. For an automatic Go Function preview, open a trusted PR that changes a direct `platform/crossplane/function/*.go` file. After both CI workflows pass, the watcher provisions a vCluster and installs the candidate Function inside it. `deploy/local/verify-crossplane-candidate-preview.sh <PR number>` checks the candidate package, exact-head child XR, local URL, and host isolation. The [live Function verification record](pr-driven-crossplane-function-preview-verification.md) describes that path. A [bounded namespace Composition edit](pr-driven-composition-preview.md) uses the same path and has an optional `--expect-pod-budget-5` check. A [bounded XRD edit](pr-driven-xrd-preview.md) uses `--expect-xrd-reason-codes`.

Use the [short demo guide](demo.md) when presenting the PR-driven namespace path, status UI, policy decision, and cleanup.

## Stop and clean up

Stop the foreground Backstage process with Ctrl+C. To stop the watcher and status services while retaining kind:

```sh
deploy/local/teardown.sh --services-only
```

To remove **only** the named `preview-platform` kind cluster after all source PRs and previews are closed:

```sh
deploy/local/teardown.sh --delete-cluster
```

The deletion command refuses to proceed if any source PR is open, a preview XR remains, or the trusted GitOps checkout still contains a preview directory. It retains the private GitOps checkout, Argo deploy key file, local MCP token, and other credentials for a future bootstrap; remove those separately only when no longer needed. The host's user manager has `Linger=no`, so user services stop after a full logout. The kind cluster also depends on Docker being available after reboot; run the verification gate after restarting the host.
