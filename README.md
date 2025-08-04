# llmd-operator

[![Go Report Card](https://goreportcard.com/badge/github.com/d0w/llmd-operator)](https://goreportcard.com/report/github.com/d0w/llmd-operator)

A Kubernetes operator that deploys and manages [llm-d](https://llm-d.ai/) infrastructure for high-performance distributed LLM inference.

## Overview

The **llmd-operator** is a **meta-operator** that deploys and manages the complete llm-d ecosystem on Kubernetes/OpenShift. Instead of reimplementing llm-d functionality, it deploys the real llm-d components and manages their lifecycle.

### Architecture

```
┌─────────────────┐    ┌─────────────────────────────────────────┐
│                 │    │           llm-d Infrastructure           │
│  llmd-operator  │───▶│  ┌─────────────────────────────────────┐ │
│  (meta-operator)│    │  │     ModelService Controller        │ │
│                 │    │  │   (manages ModelService CRs)       │ │
└─────────────────┘    │  └─────────────────────────────────────┘ │
                      │  ┌─────────────────────────────────────┐ │
       ┌───────────────┤  │             Redis                  │ │
       │               │  │      (KV cache & routing)          │ │
       │               │  └─────────────────────────────────────┘ │
       │               │  ┌─────────────────────────────────────┐ │
       ▼               │  │         Gateway Resources           │ │
┌─────────────────┐    │  │     (Gateway API routing)          │ │
│ Users create    │    │  └─────────────────────────────────────┘ │
│ ModelService    │    │  ┌─────────────────────────────────────┐ │
│ resources       │    │  │        ConfigMap Presets           │ │
└─────────────────┘    │  │    (deployment templates)          │ │
                      │  └─────────────────────────────────────┘ │
                      └─────────────────────────────────────────┘
```

## What gets deployed

When you create an `LLMD` resource, the operator deploys:

### Core Infrastructure
- **ModelService Controller**: The llm-d controller that manages `ModelService` CRs
- **Redis**: For KV cache and intelligent routing (optional)
- **Gateway Resources**: Gateway API resources for external access (optional)
- **ConfigMap Presets**: Pre-configured templates for different deployment types
- **RBAC**: Service accounts, roles, and bindings for proper permissions
- **Monitoring**: ServiceMonitors for metrics collection (optional)

### Model Deployments
Users then create `ModelService` resources (managed by the llm-d controller) which deploy:
- **Prefill Services**: Compute-bound prefill phase handling
- **Decode Services**: Memory bandwidth-bound decode phase handling  
- **Endpoint Picker**: Smart routing between prefill/decode services
- **Routing Proxy**: Request routing sidecars

## Quick Start

1. **Install the operator**:
   ```bash
   kubectl apply -f https://github.com/d0w/llmd-operator/releases/latest/download/install.yaml
   ```

2. **Deploy llm-d infrastructure**:
   ```yaml
   apiVersion: llmd.opendatahub.io/v1alpha1
   kind: LLMD
   metadata:
     name: llmd-sample
   spec:
     modelServiceController:
       enabled: true
     redis:
       enabled: true
     gateway:
       enabled: true
     configMapPresets:
       - "basic-gpu-preset"
       - "basic-gpu-with-nixl-and-redis-lookup-preset"
     monitoring:
       enabled: true
   ```

3. **Deploy a model**:
   ```yaml
   apiVersion: llm-d.ai/v1alpha1
   kind: ModelService
   metadata:
     name: llama-model
   spec:
     baseConfigMapRef:
       name: basic-gpu-preset
     routing:
       modelName: "meta-llama/Llama-3.2-3B-Instruct"
     modelArtifacts:
       uri: "hf://meta-llama/Llama-3.2-3B-Instruct"
     prefill:
       replicas: 1
       containers:
       - name: "vllm"
         args:
         - "--served-model-name"
         - "llama-3.2-3b-instruct"
         resources:
           requests:
             nvidia.com/gpu: 1
           limits:
             nvidia.com/gpu: 1
     decode:
       replicas: 2
       containers:
       - name: "vllm"
         args:
         - "--served-model-name" 
         - "llama-3.2-3b-instruct"
         resources:
           requests:
             nvidia.com/gpu: 1
           limits:
             nvidia.com/gpu: 1
   ```

## Configuration

### LLMD Resource Spec

| Field | Description | Default |
|-------|-------------|---------|
| `modelServiceController.enabled` | Deploy the ModelService controller | `true` |
| `modelServiceController.image` | Override controller image | `ghcr.io/llm-d/llm-d-model-service:0.0.10` |
| `redis.enabled` | Deploy Redis for KV cache | `true` |
| `redis.internal.persistence.enabled` | Enable Redis persistence | `true` |
| `redis.internal.persistence.size` | Redis storage size | `8Gi` |
| `gateway.enabled` | Create Gateway resources | `true` |
| `gateway.className` | Gateway class name | `kgateway` |
| `configMapPresets` | List of preset names to create | `[]` |
| `monitoring.enabled` | Enable metrics collection | `true` |
| `namespace` | Target namespace for components | Same as LLMD resource |

### Available ConfigMap Presets

- `basic-gpu-preset`: Basic GPU configuration
- `basic-gpu-with-nixl-preset`: GPU with NIXL networking
- `basic-gpu-with-nixl-and-redis-lookup-preset`: GPU with NIXL and Redis lookup

## Container Images

The operator deploys these official llm-d images:

| Component | Image | Purpose |
|-----------|-------|---------|
| ModelService Controller | `ghcr.io/llm-d/llm-d-model-service:0.0.10` | Manages ModelService resources |
| vLLM Inference | `ghcr.io/llm-d/llm-d:0.0.8` | Prefill/Decode services |
| Endpoint Picker | `ghcr.io/llm-d/llm-d-inference-scheduler:0.0.4` | Smart routing |
| Routing Proxy | `ghcr.io/llm-d/llm-d-routing-sidecar:0.0.6` | Request routing |
| Redis | `redis:7-alpine` | KV cache and routing |

## Development

### Prerequisites

- Go 1.22+
- Kubernetes 1.30+
- kubectl
- kustomize

### Building

```bash
make build
```

### Testing

```bash
make test
```

### Deploying to cluster

```bash
make install    # Install CRDs
make run        # Run controller locally
```

Or deploy to cluster:

```bash
make deploy
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) for details.

## Related Projects

- [llm-d](https://llm-d.ai/) - The distributed LLM inference framework
- [llm-d-deployer](https://github.com/llm-d/llm-d-deployer) - Helm charts for llm-d
- [vLLM](https://docs.vllm.ai/) - High-throughput LLM inference engine

