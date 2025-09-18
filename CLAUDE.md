# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is the Governance Policy Framework Addon for Open Cluster Management, written in Go. It's a Kubernetes controller that runs on managed clusters and provides several synchronization controllers for policy management between hub and managed clusters.

## Common Development Commands

### Build and Test
- `make build` - Build the binary to `build/_output/bin/governance-policy-framework-addon`
- `make test` - Run unit tests (requires `make test-dependencies` first)
- `make test-coverage` - Run unit tests with coverage
- `make e2e-test` - Run end-to-end tests (requires `make e2e-dependencies` first)

### Local Development with KinD
- `make kind-bootstrap-cluster-dev` - Create KinD clusters and install CRDs
- `make build-images` - Build Docker images
- `make kind-deploy-controller-dev` - Deploy controller to KinD cluster
- `make kind-delete-cluster` - Clean up KinD clusters

### Code Generation
- `make generate` - Generate DeepCopy methods
- `make manifests` - Generate CRDs and RBAC manifests
- `make generate-operator-yaml` - Generate final deployment manifest

### Running Locally
- `make run` - Run controller locally (requires hub and managed cluster configs)

## Architecture

The addon consists of four main controllers in the `controllers/` directory:

### 1. Secret Sync Controller (`controllers/secretsync/`)
- Syncs the `policy-encryption-key` Secret from hub to managed cluster
- Runs on managed clusters
- Requires Secret access in the managed cluster namespace

### 2. Spec Sync Controller (`controllers/specsync/`)
- Updates local Policy specs to match Policies from the hub cluster
- Watches for Policy changes in the cluster's namespace on the hub
- Creates/updates/deletes replicated policies on managed cluster

### 3. Status Sync Controller (`controllers/statussync/`)
- Updates Policy statuses on both hub and managed clusters
- Watches policy changes and events in the managed cluster
- Includes event mapping and predicates for status updates

### 4. Template Sync Controller (`controllers/templatesync/`)
- Updates objects defined in Policy templates
- Watches Policy changes in cluster namespace on managed cluster
- Creates/updates/deletes objects from `spec.policy-templates`

### 5. Gatekeeper Sync Controller (`controllers/gatekeepersync/`)
- Handles Gatekeeper constraint synchronization
- Includes policy predicates for Gatekeeper-specific logic

### 6. Uninstall Controller (`controllers/uninstall/`)
- Handles cleanup when the addon is uninstalled

## Key Files

- `main.go` - Main entry point, sets up all controllers and managers
- `controllers/utils/` - Shared utilities for controllers
- `test/e2e/` - End-to-end test suites
- `test/resources/` - Test resource manifests organized by test cases
- `deploy/` - Kubernetes deployment manifests and RBAC configurations

## Dependencies

- Go 1.23+
- Kubernetes controller-runtime framework
- Open Cluster Management addon framework
- Gatekeeper frameworks for constraint handling

## Testing

The project uses Ginkgo for testing with extensive e2e test coverage. Test resources are organized by case numbers in `test/resources/case*/`.

For coverage testing:
- Unit test coverage minimum: 69%
- Use `make coverage-merge` and `make coverage-verify` for coverage analysis

## Development Notes

- The controller can run in both normal and hosted modes (controlled by `HOSTED` env var)
- KinD clusters are used for local development with separate hub and managed clusters
- The project uses controller-gen for code generation and Kustomize for manifest generation
- Branch-based dependency management in Makefile for different environments