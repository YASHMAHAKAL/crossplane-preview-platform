# Product and workflow

This file is the behavior contract. For schemas and ownership boundaries, see [architecture-contracts.md](architecture-contracts.md).

## Actors and objects

| Actor or object | Responsibility |
| --- | --- |
| Platform operator | Starts `kind`, installs controllers, configures GitHub access, registers templates and catalog entities, and sets trusted repository policy. |
| Developer | Creates the Incident Tracker service, changes its code or preview requirements, and opens a PR. |
| Backstage user | Discovers services, sees the supported requests and accepted inputs, starts a template or preview request, and reads the preview decision/status. |
| Codex user | Asks what services and actions exist, supplies inputs through Backstage MCP actions, and receives the same result and status as the UI. |
| Evaluator | Observes trusted PRs and CI artifacts, derives an isolation decision, records evidence, and writes or deletes the authorized GitOps XR. |
| Argo CD and Crossplane | Apply and reconcile the evaluator's desired state; expose observed readiness and cleanup signals. |

## Demonstration service

Build a polished **Incident Tracker** for a fictional operations team. A visitor can create incidents with title, description, affected service, severity, and owner; browse/search/filter the queue; inspect an incident's details and activity; and move it through `open`, `investigating`, and `resolved`. Show useful summary metrics and a clear health signal. Use a cohesive operations theme with responsive components and accessible forms, labels, keyboard behavior, and focus states. Seed representative demo data so a new preview is immediately meaningful. Keep the app focused enough that platform behavior remains the main story. Expose a health endpoint and a stable route. The default namespace preview needs only namespaced resources. A second demonstration PR declares and uses a lightweight cluster-scoped `IncidentPolicy` CRD (and a minimal corresponding capability) so the evaluator has a concrete reason to choose vCluster. Do not claim that the vCluster is necessary for all CRDs in every environment; it is the policy choice for this shared host.

The Backstage software template creates a repository that contains the app, container build, deployment package, catalog descriptor, API description where useful, CI workflow, and a checked-in preview contract. The catalog entity advertises ownership, repository link, API, and the preview action. The preview contract advertises allowable input fields and bounds; it is data for both UI and Codex discovery, not an instruction to grant arbitrary infrastructure.

## End-to-end journeys

### Service discovery

1. The user searches Backstage Catalog or asks Codex what can be requested.
2. Backstage returns the service identity, description, owner, API link, supported preview capability, accepted parameters, limits, and current preview status. For Codex, expose this through the Actions Registry/MCP surface; use a small custom action if stock catalog actions cannot return the complete capability schema.
3. The user supplies only declared inputs. Validation errors name the rejected field and allowed values.

### Manual request through Backstage

1. A Scaffolder template creates the Incident Tracker repository, or a service action creates a preview-request PR for an existing registered service. An ordinary developer code PR with a valid preview contract is also eligible.
2. The request UI captures service, PR or branch context, optional size/TTL within published bounds, and intended changes. It returns the GitHub PR URL and a request identifier. It does not directly deploy a preview.
3. The user sees `waiting-for-ci`, `evaluating`, `approved`, `rejected`, `provisioning`, `ready`, `degraded`, `cleaning`, or `deleted`, with a reason. A ready preview has its local URL and isolation mode.

### Request through Codex

1. Codex uses Backstage catalog and capability actions to answer “what services exist, what do they provide, and what can I ask for?” without requiring the user to guess action names or input fields.
2. After the user requests a concrete change, Codex uses the same registered template/action and schema as the UI. The resulting PR and evaluator flow are identical. Codex may inspect or modify the source repository only within the user's granted workspace and GitHub permissions; it does not directly write a privileged XR.
3. Codex can query status and report the decision explanation and preview URL. The Backstage view and Codex response should agree because both read the same status source.

### PR lifecycle

1. A PR is opened or updated in a configured source repository.
2. CI builds/tests the application and produces an immutable image reference or digest. The evaluator waits for the required checks and verifies the artifact belongs to the current PR head. For an offline/local-only demonstration, use a deterministic fixture artifact and mark the run as simulated.
3. The evaluator loads changed-file metadata plus the declared preview contract, applies deterministic policy, and records `namespace`, `vcluster`, or `rejected` with evidence. It never treats a natural-language request as authority for privileged resources.
4. For an approved PR, it commits one normalized `PreviewEnvironment` XR in a trusted GitOps folder. Argo CD's directory generator creates or updates an Application. Crossplane reconciles the selected Composition. The app becomes reachable at a local preview URL.
5. Further commits to the PR update the preview for the new head. Decisions and image references must be tied to that head so stale CI results cannot deploy the wrong revision.
6. Merge or close removes the GitOps folder; Argo CD prunes; Crossplane deletes the preview. The PR can be merged without any permanent deployment target in this project.

## What the preview shows

The local browser opens the actual Incident Tracker web app for that PR revision. The platform page also shows PR number, source commit, isolation mode, reason/evidence, phase, application health, preview URL, creation time, and cleanup result. A namespace preview and a vCluster preview must be visibly distinguishable, while both expose the same app use case.

## User-facing completion criteria

- A new user can create/discover a service and find its accepted request inputs in Backstage UI and in a Codex conversation.
- A normal app PR receives a live namespace preview after a successful build.
- The cluster-scoped demonstration PR receives a live vCluster preview, with an explanation linked to the CRD requirement.
- An unsupported or over-limit request is rejected with an actionable reason, without host resource creation.
- A new commit updates the matching preview to the current image digest.
- Merge and close both remove the preview and the UI/Codex status reports completion or an explicit cleanup failure.
