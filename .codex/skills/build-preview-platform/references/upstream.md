# Upstream documentation to consult during implementation

These links are primary sources for the proposed integration. They are pointers, not frozen API guarantees. Check the docs for the versions pinned by the project before writing manifests or code.

## Crossplane

- [Crossplane v2 upgrade guide](https://docs.crossplane.io/latest/guides/upgrade-to-crossplane-v2/) — modern cluster-scoped XRs and namespaced composition behavior.
- [Composite resources](https://docs.crossplane.io/latest/composition/composite-resources/) and [Compositions](https://docs.crossplane.io/latest/composition/compositions/) — XRD, selection, pipeline functions, ownership, and deletion.
- [Write a Composition Function in Go](https://docs.crossplane.io/latest/guides/write-a-composition-function-in-go/) — SDK and packaging workflow.
- [Crossplane CLI command reference](https://docs.crossplane.io/cli/latest/command-reference/) — render and validation commands.
- [provider-helm](https://github.com/crossplane-contrib/provider-helm) — Helm Release and ProviderConfig behavior.

## Backstage and Codex request surface

- [MCP Actions Backend](https://backstage.io/docs/ai/mcp-actions/) — MCP endpoint, authentication, named server filters, and action exposure.
- [Well-known Actions](https://backstage.io/docs/ai/well-known-actions/) — current catalog and scaffolder actions.
- [Actions Registry](https://backstage.io/docs/backend-system/core-services/actions-registry/) — custom action schemas and registration.
- [Software Templates](https://backstage.io/docs/features/software-templates/writing-templates/) and [template parameter schema API](https://backstage.io/docs/features/software-templates/api/get-template-parameter-schema/) — UI schema and machine-readable accepted inputs.
- [Catalog descriptor format](https://backstage.io/docs/features/software-catalog/descriptor-format/) — Component and API entities.

## GitOps, virtual clusters, and PR source

- [Argo CD ApplicationSet Git generator](https://argo-cd.readthedocs.io/en/stable/operator-manual/applicationset/Generators-Git/) and [resource deletion controls](https://argo-cd.readthedocs.io/en/stable/operator-manual/applicationset/Controlling-Resource-Modification/) — directory lifecycle and pruning.
- [vCluster OSS versus Free](https://www.vcluster.com/docs/vcluster/introduction/oss-vs-free), [vcluster.yaml](https://www.vcluster.com/docs/vcluster/configure/vcluster-yaml), [ingress sync](https://www.vcluster.com/docs/vcluster/configure/vcluster-yaml/sync/to-host/networking/ingresses), and [shared nodes](https://www.vcluster.com/docs/vcluster/quick-start/shared-nodes/) — license tier, OSS image, workload deployment options, and routing limitations.
- [GitHub pull request API](https://docs.github.com/en/rest/pulls/pulls) — PR metadata and changed files. Use current GitHub docs for required checks and artifact retrieval when implementing those pieces.

## OpenAI skill format

- [OpenAI Skills guide](https://developers.openai.com/api/docs/guides/tools-skills) — `SKILL.md` manifest and supporting files.
- [Testing Agent Skills Systematically](https://developers.openai.com/blog/eval-skills) — repo-scoped location and behavioral evaluation.
- [Rethinking skills and prompts](https://developers.openai.com/blog/rethinking-skills-and-prompts-for-gpt-6-astra) — concise discovery metadata and progressive disclosure.
