# KDP workspace operator v1

## Context

We are building a CNCF self-service portal based on the Kubermatic Developer Platform.

## Objective

You should prepare a multi-stage plan for building a KDP workspace operator. The operator creates workspaces based on the CRs created by maintainer-d (retrieve them with `kubectl get crd | grep maintainer`).

1. You must follow the specification described in [CLAUDE_20251223_kdp_organiztion_op_design_doc.md](CLAUDE_20251223_kdp_organiztion_op_design_doc.md).

2. You MUST use https://github.com/kcp-dev/client-go to connect to kcp.dev.

3. Use kubebuilder - https://github.com/kubernetes-sigs/kubebuilder.

Ask questions if you miss any infromation.
