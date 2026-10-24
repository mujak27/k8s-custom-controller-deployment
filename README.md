# CustomDeployment: safe workload updates

Learning slices for [issue #2](https://github.com/mujak27/k8s-custom-controller-deployment/issues/2)
and [issue #3](https://github.com/mujak27/k8s-custom-controller-deployment/issues/3): a
namespaced `workloads.example.com/v1alpha1` CustomDeployment creates one same-named
native Deployment with one container. Kubernetes manages the ReplicaSet and Pods.

Issue #2 created and owned that Deployment. Issue #3 adds validated replica bounds,
an immutable initial replica count, image/resource updates, status, and visible
ownership conflicts. There are still no metrics reads, autoscaling, HPA, Services,
webhooks, or in-cluster manager.

## Issue #3 local behavior

- `spec.image` is required and nonempty.
- `spec.replicas` is the creation-only **initial replica count**: default `1`,
  minimum `1`, immutable after creation. Omitting it on an update defaults it to
  `1`, so an update that omits a stored value other than `1` is rejected.
- `spec.minReplicas` defaults to `1` and must be at least `1`.
  `spec.maxReplicas` defaults to `5` and must be at least `1` and at least
  `minReplicas`.
- On first Deployment creation only, the initial count is clamped into
  `[minReplicas, maxReplicas]`. Later reconciliation never writes the Deployment
  replica count, so a count changed by a later scaler or a bounds-only edit is
  preserved.
- `resources.requests.cpu` and `resources.requests.memory` are required and must
  be strictly positive. Limits are optional; when a CPU or memory limit is
  present it must be positive and at least the corresponding request. Generated
  quantity schema validation checks syntax, and CEL checks presence, positivity,
  and request/limit ordering for both numeric and string quantity encodings. No
  admission webhook was added.
- `spec.image`, `spec.resources`, and the replica bounds are editable. An image or
  resource edit updates only the managed container fields of the owned Deployment;
  the Deployment selector, defaulted fields, and replica count are retained.
  Bounds apply at Deployment creation and, later, to autoscaling.
- `status.observedGeneration` records the processed CustomDeployment generation.
  `status.desiredReplicas` and `status.readyReplicas` report the owned Deployment's
  current desired and ready counts. Exactly one `Ready` condition uses
  `RolloutInProgress`, `RolloutComplete`, or `OwnershipConflict`.
- A new CustomDeployment generation is not reported Ready from an older
  Deployment rollout. Readiness requires the Deployment to have observed its
  current generation and to have all desired replicas updated, ready, and
  available.
- Status is written only when it changes; unchanged Ready conditions keep their
  transition time. A status update does not trigger a write loop.
- An unrelated same-name Deployment is never adopted, modified, or deleted.
  Reconciliation sets `Ready=False` with reason `OwnershipConflict`, returns an
  ownership-conflict error, and leaves the existing object unchanged.

Deleting a CustomDeployment relies on Kubernetes owner-reference garbage
collection for its Deployment, ReplicaSets, and Pods. No external-resource
finalizer is needed because there is no resource outside Kubernetes to clean up;
the owner chain already gives Kubernetes the deletion responsibility.

Manually editing the owned Deployment or attaching an HPA is unsupported. This
controller owns the workload declaration and is intended to be the sole scaling
writer once issue #4/#5 land; issue #3 does not implement metrics, scaling, or HPA
behavior.

## Issue #3 live acceptance is pending

No live Kubernetes command, CRD installation, local manager run, rollout,
ownership-conflict check, manual Deployment edit, HPA attachment, or
garbage-collection verification was performed for issue #3. The local suite uses
envtest, which runs only an API server and etcd: it does not run the Deployment
controller, kubelets, or garbage collector. Envtest therefore cannot prove live
rollout or deletion behavior. Those checks remain pending explicit cluster
approval.

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
**Handwritten within scaffold entry points:** API fields/markers (including replica
bounds, resource CEL rules, and status), creation/update/status reconciler and owned
Deployment watch, required `WATCH_NAMESPACE` manager configuration, envtest behavior
tests, sample settings, and this README. The automatic Kind E2E workflow was removed;
remaining generated in-cluster/Docker/Kind targets and fixtures are not this project's
tested deployment path.

## Build and tests

Issue #3 reused the pinned controller-gen v0.21.0, golangci-lint, and envtest
Kubernetes 1.36.2 assets already installed in the issue #2 worktree, so no tools
were installed for this slice. The actual commands were:

```sh
make manifests generate fmt vet CONTROLLER_GEN=/Users/violet/codes/violet/custom-controller-deployment-worktrees/create-first-customdeployment/bin/controller-gen
KUBEBUILDER_ASSETS=/Users/violet/codes/violet/custom-controller-deployment-worktrees/create-first-customdeployment/bin/k8s/1.36.2-darwin-arm64 go test $(go list ./... | grep -v /e2e) -count=1
KUBEBUILDER_ASSETS=/Users/violet/codes/violet/custom-controller-deployment-worktrees/create-first-customdeployment/bin/k8s/1.36.2-darwin-arm64 go test -race ./internal/controller -count=1
GOLANGCI_LINT_CACHE=/private/var/folders/71/7fmmmmsd3nzbsbgdhfgdp5rh0000gn/T/opencode/customdeployment-issue3-lint-cache /Users/violet/codes/violet/custom-controller-deployment-worktrees/create-first-customdeployment/bin/golangci-lint run --timeout 5m
go build -o /private/var/folders/71/7fmmmmsd3nzbsbgdhfgdp5rh0000gn/T/opencode/customdeployment-issue3-manager ./cmd/main.go
```

Observed: **30/30 specs passed** on envtest Kubernetes **1.36.2 darwin/arm64**;
controller statement coverage **87.3%**; race-enabled controller suite passed;
lint **0 issues**; manager build passed. A clean checkout can use `make build test`
and `make lint`, which install the same pinned tool versions into ignored `bin/`.

The issue #2 red → green history was:

- Creation failed with Deployment `NotFound` before implementing reconciliation, then
  passed with exact image/resources, controller ownership, and preserved replicas.
- The unrelated-object test initially failed because reconciliation returned nil;
  the ownership check now reports the conflict and preserves the existing object.
- Empty image and zero/negative replicas were initially accepted, and omitted replicas
  were rejected. Generated schema validation/defaulting now rejects/defaults correctly.
- Initial lint found repeated fixture text and a capitalized error; both were corrected.

The issue #3 red → green checks demonstrated:

- Omitted bounds initially came back as `0/0`; schema defaults now produce `1/5`.
- Nonpositive minimums and `maxReplicas < minReplicas` were initially accepted; schema
  minimums plus a CEL rule now reject them. Explicit zero is tested through an
  unstructured object because the typed client omits zero-valued `omitempty` fields.
- Missing, zero, and negative CPU/memory requests, malformed quantities, nonpositive
  limits, and limits below requests were initially accepted; quantity schema/CEL now
  rejects them. Limits may still be omitted. An integer-form quantity (`cpu: 1`) was
  initially rejected by an overly narrow CEL conversion; CEL now converts both numeric
  and string encodings before comparing.
- Explicit and omitted edits to the initial replica count were initially accepted; a
  transition CEL rule now rejects both.
- Deployment creation initially used the raw initial count (`9` and `1` in the
  above-max/below-min cases); it now clamps to `5` and `2`.
- Image/resource edits initially left the Deployment unchanged; reconciliation now
  updates only managed container fields and preserves a later replica count of four.
- A bounds-only edit was checked not to rewrite the Deployment at all.
- Status was initially absent; it now reports generation, actual counts, and a Ready
  condition. Pending, ready, and stale-rollout transitions are covered.
- The conflict test initially had no visible condition; it now checks
  `Ready=False`, reason `OwnershipConflict`, zero owned counts, and an unchanged
  unrelated Deployment.
- The stale-readiness test exposed a shallow status copy: `SetStatusCondition`
  mutated the original slice, so the equality check skipped the write. Deep-copying
  status before editing fixed condition transitions while keeping unchanged writes out.

Tests use the real Kubernetes API with a namespace-limited running controller, not an
in-memory client. **Envtest runs only the API server and etcd**: no Deployment controller,
kubelets, scheduler, garbage collector, or Metrics Server. It cannot establish Pod
readiness or owner-based cleanup; the issue #2 real-cluster checks below did that, and
issue #3 live rollout/GC checks remain pending.

## Existing-cluster walkthrough (issue #2, historical)

This section records the completed issue #2 live demo; it is not issue #3
acceptance. Reverify cluster identity and collisions before any repeat.

Shared prerequisites belong to the Argo CD GitOps workflow in
`mujak27/k8s-crossplane`; do not install or disrupt them here. Only the experimental
CRD, dedicated namespace/workload, and test permissions if necessary may be applied
directly. The issue #2 demo needed no added permissions or infrastructure changes.
Issue #3's generated manager role now includes Deployment `update` and `patch`, so a
future live run must use a kubeconfig authorized for those writes.

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

## Safe cleanup (issue #2, performed)

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
