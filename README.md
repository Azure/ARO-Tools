# Project

> This repo has been populated by an initial template to help get you started. Please
> make sure to update the content to build a great experience for community-building.

As the maintainer of this project, please make a few updates:

- Improving this README.MD file to provide a great experience
- Updating SUPPORT.MD with content about this project's support experience
- Understanding the security reporting process in SECURITY.MD
- Remove this section from the README

## Merge automation (Prow / Tide)

This repository is onboarded onto [OpenShift CI (Prow)](https://docs.ci.openshift.org/). Configuration lives in
[openshift/release](https://github.com/openshift/release) under
`core-services/prow/02_config/Azure/ARO-Tools/`.

Merges are handled by Prow's Tide component, not by pressing the GitHub merge button. A PR merges once it has both labels:

- `lgtm` — added when a reviewer comments `/lgtm`.
- `approved` — added when an [OWNERS](./OWNERS) approver comments `/approve`.

Approvers and reviewers are defined in [`OWNERS`](./OWNERS). Use `/hold` to block a merge and `/hold cancel` to release
it. Comment `/retest` to re-run failed checks.

Note: only merge commits are allowed on `main` (squash and rebase merges are disabled), matching
[Azure/ARO-HCP](https://github.com/Azure/ARO-HCP)'s repo settings.

## Helm deployment

The `helmdeploy` tool removes legacy `helm` field-manager entries before upgrading
a release with server-side apply. Resources without a legacy entry are not written.
Resource-version conflicts are retried with bounded backoff and a fresh read on
each attempt, preserving concurrent controller updates. Other API errors and
conflicts that exhaust the retry budget fail the deployment.

## Istio upgrade timeouts

The `tools/istio-upgrade` ARM client polls each managed-cluster update for up to
20 minutes, bounded by the caller's context. AKS updates can complete successfully
after more than 15 minutes even when the new Istio pods have already started.
The separate post-operation provisioning check has a five-minute timeout; the
overall upgrade remains bounded by its 60-minute timeout.

## Istio bootstrap discovery

`istio discover-bootstrap --subscription-id <subscription> --resource-group <group>
--cluster-name <cluster> --versions asm-1-29` lists all managed-cluster pages in
the resource group without GETting the target cluster or loading a kubeconfig.
It emits `{"istioVersions":"asm-1-29"}` only when the cluster is absent; every
existing cluster, including Failed clusters and active canaries, produces
`{"istioVersions":""}` so consumers omit creation-only revisions. Listing errors
fail the command rather than imply absence.

`--versions` accepts ordered CSV revisions.
`--output-file <path>` additionally writes the same JSON emitted on stdout.
The `IstioBootstrapDiscovery` pipeline action takes `aksCluster` and `versions`
as `Value` bindings and an optional `identityFrom` input; its output is named
`istioVersions`. Runners must checkpoint this live discovery within the deployment
execution, preserving its output for ARM retries and supported resume without
reusing it for an unrelated execution.

## Contributing

This project welcomes contributions and suggestions.  Most contributions require you to agree to a
Contributor License Agreement (CLA) declaring that you have the right to, and actually do, grant us
the rights to use your contribution. For details, visit https://cla.opensource.microsoft.com.

When you submit a pull request, a CLA bot will automatically determine whether you need to provide
a CLA and decorate the PR appropriately (e.g., status check, comment). Simply follow the instructions
provided by the bot. You will only need to do this once across all repos using our CLA.

This project has adopted the [Microsoft Open Source Code of Conduct](https://opensource.microsoft.com/codeofconduct/).
For more information see the [Code of Conduct FAQ](https://opensource.microsoft.com/codeofconduct/faq/) or
contact [opencode@microsoft.com](mailto:opencode@microsoft.com) with any additional questions or comments.

## Trademarks

This project may contain trademarks or logos for projects, products, or services. Authorized use of Microsoft 
trademarks or logos is subject to and must follow 
[Microsoft's Trademark & Brand Guidelines](https://www.microsoft.com/en-us/legal/intellectualproperty/trademarks/usage/general).
Use of Microsoft trademarks or logos in modified versions of this project must not cause confusion or imply Microsoft sponsorship.
Any use of third-party trademarks or logos are subject to those third-party's policies.
