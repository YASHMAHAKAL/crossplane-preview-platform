# Backstage source-change template verification

On 2026-10-11, Backstage loaded three `Template` entities from public `main`. Each template was executed through Backstage's Scaffolder HTTP API using the configured local token. Its completed task linked to an ordinary GitHub PR. No template wrote `preview.request.json`, selected an isolation mode, or created a `PreviewEnvironment` directly.

| PR | Generated source diff | Current-head CI and evaluator | Live verification | Close result |
| --- | --- | --- | --- | --- |
| [#25](https://github.com/YASHMAHAKAL/crossplane-preview-platform/pull/25) | Only `app/incident-tracker/public/appearance.json` | `build` passed after the test correction; `namespace`, `namespaced-app-change` | `ready`; live `/appearance.json` served the new headline, subtitle, and violet palette | `deleted`, `cleanup-verified`; Application, XR, namespace, volumes, and route absent |
| [#26](https://github.com/YASHMAHAKAL/crossplane-preview-platform/pull/26) | Only `deploy/incident-tracker/preview.json` | `build` passed; `namespace`, `namespaced-deployment-change`, `replicas=2` | `ready`; two ready app replicas with `250m`/`256Mi` requests and `1000m`/`1024Mi` limits; `/healthz` returned 200 | `deleted`, `cleanup-verified`; Application, XR, namespace, volumes, and route absent |
| [#27](https://github.com/YASHMAHAKAL/crossplane-preview-platform/pull/27) | Only `deploy/cluster/incident-policy.json` | `build` passed; `vcluster`, `cluster-api-required`, `allowCritical=true` | `ready`; `/api/context` reported `vcluster`, `/healthz` returned 200; guest CRD had `low`, `high`, `critical`, while host CRD was absent | `deleted`, `cleanup-verified`; Application, XR, namespace, volumes, and route absent |

The first CI run for #25 exposed a test that asserted the baseline copy in a file the template is intended to change. The test now checks valid appearance shape and palette instead. PR #25's updated head passed CI and reached a live preview.

Argo CD's [Git generator](https://argo-cd.readthedocs.io/en/stable/operator-manual/applicationset/Generators-Git/) uses a three-minute default poll. For final cleanup verification, the operator used its documented one-shot `argocd.argoproj.io/application-set-refresh` annotation after the watcher removed GitOps directories. This bypassed the repository revision cache. It did not change the source PR or trusted GitOps contents.

The portal was exercised through Scaffolder's backend API, which is the same execution path used by its UI and MCP action. This run did not automate a browser form submission or a separate Codex session.
