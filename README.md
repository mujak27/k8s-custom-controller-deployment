# CustomDeployment: first workload

Learning slice for [issue #2](https://github.com/mujak27/k8s-custom-controller-deployment/issues/2):
a namespaced `workloads.example.com/v1alpha1` CustomDeployment creates one same-named
native Deployment with one container. Kubernetes manages the ReplicaSet and Pods.
There are no metrics reads, autoscaling, HPA, Services, webhooks, or in-cluster manager.

`spec.image` must be nonempty; `spec.replicas` is a positive **initial replica count**
(default 1), used only at Deployment creation. `spec.resources` supplies native
container requests/limits. Existing owned Deployments are left unchanged in this slice,
including their replica count. Editable workload reconciliation, immutable initial
replicas, complete resource validation, and status belong to issue #3.
An unrelated same-name Deployment is never adopted or overwritten: reconciliation
returns an ownership-conflict error, which controller-runtime logs and retries.

## Tooling and generation (observed 2026-10-16)

All commands below run from the repository root. The actual implementation directory was
`/Users/violet/codes/violet/custom-controller-deployment-worktrees/issue-2-first-customdeployment`.

Go was already installed (`go version go1.27.1 darwin/arm64`); Kubebuilder was missing.
The [compatibility guidance](https://book.kubebuilder.io/versions_compatibility_supportability.html)
says to use a release's generated dependency/tool versions. The pinned
[v4.15.0 go.mod](https://github.com/kubernetes-sigs/kubebuilder/blob/v4.15.0/testdata/project-v4/go.mod)
requires Go 1.26 and uses Kubernetes libraries v0.36.0 / controller-runtime v0.24.1,
matching the existing cluster's Kubernetes 1.36 minor. No dependency minor was upgraded.
The generic [quick start](https://book.kubebuilder.io/quick-start.html) describes the
generation commands; the pinned release's files, not its generic minimums, govern this project.

The actual host installation/generation commands were:

```sh
command -v go kubebuilder kubectl; go version
curl -fL https://github.com/kubernetes-sigs/kubebuilder/releases/download/v4.15.0/kubebuilder_darwin_arm64 -o /private/var/folders/71/7fmmmmsd3nzbsbgdhfgdp5rh0000gn/T/opencode/kubebuilder-v4.15.0
curl -fL https://github.com/kubernetes-sigs/kubebuilder/releases/download/v4.15.0/checksums.txt
shasum -a 256 /private/var/folders/71/7fmmmmsd3nzbsbgdhfgdp5rh0000gn/T/opencode/kubebuilder-v4.15.0
chmod +x /private/var/folders/71/7fmmmmsd3nzbsbgdhfgdp5rh0000gn/T/opencode/kubebuilder-v4.15.0
/private/var/folders/71/7fmmmmsd3nzbsbgdhfgdp5rh0000gn/T/opencode/kubebuilder-v4.15.0 init --domain example.com --repo example.com/custom-controller-deployment --project-name custom-controller-deployment
/private/var/folders/71/7fmmmmsd3nzbsbgdhfgdp5rh0000gn/T/opencode/kubebuilder-v4.15.0 create api --group workloads --version v1alpha1 --kind CustomDeployment --plural customdeployments --resource --controller --namespaced
```

The SHA-256 matched the published value
`8a8b2806c352f3e0749d315ecea91a352b2ce9321af927fca823a1604bd3530a`.
Initialization warned about the existing instructions/glossary; those files were preserved.
Host generators and test binaries are local development tools, not cluster add-ons.
Docker and Kind were not needed or run.

**Generated:** `PROJECT`, module/build/tooling scaffolding, API registration,
DeepCopy code, CRD/RBAC manifests, Kustomize configuration, Docker/E2E boilerplate.
**Handwritten within scaffold entry points:** API fields/markers, creation/ownership
reconciler and owned Deployment watch, required `WATCH_NAMESPACE` manager configuration,
envtest behavior tests, sample settings, and this README. The automatic Kind E2E
workflow was removed; remaining generated in-cluster/Docker/Kind targets and fixtures
are not this project's tested deployment path.

## Build and tests

```sh
go mod tidy
make build test
make lint
KUBEBUILDER_ASSETS="$PWD/bin/k8s/1.36.2-darwin-arm64" go test -race ./internal/controller -count=1
env -u WATCH_NAMESPACE ./bin/manager
```

`make build test` generates CRD/RBAC and DeepCopy files, formats/vets, builds the host
manager, downloads missing pinned tools into ignored `bin/`, and runs envtest.
Observed: six passing specs on envtest Kubernetes **1.36.2 darwin/arm64**,
controller statement coverage **85.7%**; lint **0 issues**. The last command deliberately
exits 1 with `WATCH_NAMESPACE must name one dedicated test namespace`, before API access.
The race-enabled controller suite also passed. Optional attempts with `go test -count=3`
and then `-ginkgo.repeat=2` were rejected by the runner before executing specs:
Ginkgo repetition requires its CLI, not either `go test` invocation. No repeat CLI
was installed just for this extra check.

The incremental red → green checks demonstrated:

- Creation failed with Deployment `NotFound` before implementing reconciliation, then
  passed with exact image/resources, two requested replicas, and controller ownership.
  A regression assertion also verifies reconciliation preserves a later count of four.
- The unrelated-object test initially failed because reconciliation returned nil;
  the ownership check now reports the conflict and preserves the entire existing object.
- Empty image and zero/negative replicas were initially accepted, and omitted replicas
  were rejected. Generated schema validation/defaulting now rejects/defaults correctly.
- Initial lint found repeated fixture text and a capitalized error; both were corrected.

Tests use the real Kubernetes API with a namespace-limited running controller, not an
in-memory client. **Envtest runs only the API server and etcd**: no Deployment controller,
kubelets, scheduler, garbage collector, or Metrics Server. It cannot establish Pod
readiness or owner-based cleanup; the real-cluster checks below do that.

## Existing-cluster walkthrough

Shared prerequisites belong to the Argo CD GitOps workflow in
`mujak27/k8s-crossplane`; do not install or disrupt them here. Only the experimental
CRD, dedicated namespace/workload, and test permissions if necessary may be applied
directly. This slice needed no added permissions or infrastructure changes.

Read-only identity and collision checks used:

```sh
kubectl config view --minify -o json
kubectl --context orbstack get nodes -o wide
kubectl --context orbstack get crd customdeployments.workloads.example.com --ignore-not-found -o yaml
kubectl --context orbstack get namespace customdeployment-issue-2 --ignore-not-found -o yaml
```

Observed: context/cluster **orbstack**, API **https://127.0.0.1:26443**, Ready node
`orbstack`, **v1.36.5+orb1**, arm64. CRD and namespace were absent. Reverify these
facts before repeating; if the CRD exists, inspect **all** instances with
`kubectl --context orbstack get customdeployments.workloads.example.com -A` and do not
overwrite or delete someone else's API/resources.

**CRD installation is cluster-scoped**, despite `spec.scope: Namespaced` for instances.
To pin generated make targets to the verified context, a private temporary kubeconfig
was created (contains credentials; never commit it):

```sh
umask 077
kubectl --context orbstack config view --minify --flatten --raw > /private/var/folders/71/7fmmmmsd3nzbsbgdhfgdp5rh0000gn/T/opencode/customdeployment-issue2.kubeconfig
KUBECONFIG=/private/var/folders/71/7fmmmmsd3nzbsbgdhfgdp5rh0000gn/T/opencode/customdeployment-issue2.kubeconfig make install
kubectl --context orbstack create namespace customdeployment-issue-2
```

Observed: CRD `customdeployments.workloads.example.com` and namespace created.
`make install` applies **only** `config/crd`, not the generated in-cluster manager/RBAC.

Run the manager **on the host** in another terminal:

```sh
KUBECONFIG=/private/var/folders/71/7fmmmmsd3nzbsbgdhfgdp5rh0000gn/T/opencode/customdeployment-issue2.kubeconfig WATCH_NAMESPACE=customdeployment-issue-2 make run
```

The actual run was backgrounded with output in ignored `bin/issue2-manager.log`.
Observed log: `Starting manager {"namespace":"customdeployment-issue-2"}` and event
sources for both `*v1alpha1.CustomDeployment` and `*v1.Deployment`.
`WATCH_NAMESPACE` requires one valid namespace and restricts the manager cache and
watches. It does **not** reduce the existing kubeconfig user's permissions; namespace
scoping is not an RBAC security boundary. Do not run a second scaling writer or HPA.

```sh
kubectl --context orbstack -n customdeployment-issue-2 apply -f config/samples/workloads_v1alpha1_customdeployment.yaml
kubectl --context orbstack -n customdeployment-issue-2 wait --for=create deployment/customdeployment-sample --timeout=30s
kubectl --context orbstack -n customdeployment-issue-2 rollout status deployment/customdeployment-sample --timeout=120s
kubectl --context orbstack -n customdeployment-issue-2 get customdeployments,deployments,replicasets,pods -o json
```

Observed: sample created; Deployment creation wait succeeded; rollout completed.
The final JSON was also checked with Python assertions: exactly one of each kind,
each controller owner UID matched its parent, one requested/ready Deployment replica,
Pod `Ready=True`, one `nginx:1.28.0` container, requests **100m CPU / 128Mi memory**,
limits **500m CPU / 256Mi memory**.

```text
CustomDeployment/customdeployment-sample
  → Deployment/customdeployment-sample
  → ReplicaSet/customdeployment-sample-554f9999bf
  → Pod/customdeployment-sample-554f9999bf-vlxvc (Ready=True)
```

For a quick readable owner inspection on a repeat run (child names may differ):

```sh
kubectl --context orbstack -n customdeployment-issue-2 get deployments,replicasets,pods -o 'custom-columns=KIND:.kind,NAME:.metadata.name,OWNER_KIND:.metadata.ownerReferences[0].kind,OWNER:.metadata.ownerReferences[0].name'
```

## Safe cleanup (performed)

Delete only the experiment CR; Kubernetes garbage-collects its children. Use the
actual names from inspection when waiting, not stale names from a previous run.

```sh
kubectl --context orbstack -n customdeployment-issue-2 delete customdeployment customdeployment-sample
kubectl --context orbstack -n customdeployment-issue-2 wait --for=delete deployment/customdeployment-sample replicaset/customdeployment-sample-554f9999bf pod/customdeployment-sample-554f9999bf-vlxvc --timeout=90s
```

Observed: CR deleted; deletion wait exited 0 (children already absent). No child
was manually deleted. Stop the local manager with Ctrl-C; for the background run,
the exact make → go run → manager process tree was inspected, then manager PID 58158
received `kill -TERM 58158`. Do **not** reuse that PID on another run.

Before removing a CRD, check **every namespace**: removing it deletes all its instances.
Remove it only if this experiment created it and no unrelated instances now exist.
Likewise, delete the dedicated namespace only if experiment-owned and no longer needed.

```sh
kubectl --context orbstack get customdeployments.workloads.example.com -A -o wide
kubectl --context orbstack -n customdeployment-issue-2 get customdeployments,deployments,replicasets,pods
KUBECONFIG=/private/var/folders/71/7fmmmmsd3nzbsbgdhfgdp5rh0000gn/T/opencode/customdeployment-issue2.kubeconfig make uninstall
kubectl --context orbstack delete namespace customdeployment-issue-2
rm /private/var/folders/71/7fmmmmsd3nzbsbgdhfgdp5rh0000gn/T/opencode/customdeployment-issue2.kubeconfig
```

Both inventories were empty. CRD and namespace deletion succeeded; the host manager
exited and the temporary credential copy was removed. Shared infrastructure and
unrelated workloads were untouched. No controller image was built/pushed, no other
cluster was created, and no `make deploy`, `make undeploy`, or `make test-e2e` was run.
