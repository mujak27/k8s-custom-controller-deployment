# Project instructions

## Agent skills

### Issue tracker

Track work in GitHub Issues for mujak27/k8s-custom-controller-deployment. Before reading, creating, updating, or selecting tickets, read `docs/agents/issue-tracker.md`.

### Triage labels

Use the five canonical triage labels. Before triaging or labelling work, read `docs/agents/triage-labels.md`.

### Domain docs

Use the single-context glossary and ADR layout. Before exploring or designing, read `docs/agents/domain.md`.

## Learning workflow

- Before each command, show its exact text, working directory, purpose, write effects, and Kubernetes context when applicable. Afterwards explain the relevant result. Distinguish generated Kubebuilder files from handwritten code.
- Use the existing test cluster associated with mujak27/k8s-crossplane; verify its actual context before writes. Manage shared cluster prerequisites through that repository's Argo CD GitOps workflow.
- Direct CLI application is reserved for the custom-controller experiment: its CRD, test permissions, and isolated workload resources. Run the manager locally, restricted to the dedicated test namespace.
- Keep unrelated workloads and shared infrastructure untouched. Use isolated fixtures for failure tests and clean up only experiment-owned resources.
