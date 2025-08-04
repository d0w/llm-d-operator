# LLMD Operator Samples

This directory contains sample configurations for the **llmd-operator**, a Kubernetes operator that deploys and manages the [llm-d](https://github.com/llm-d/llm-d-deployer) distributed inference infrastructure.

## 🎯 Architecture Overview

The **llmd-operator** is a **meta-operator** that deploys the real llm-d components rather than emulating them:

```
┌─────────────────┐    ┌─────────────────────────────────────────┐
│                 │    │           llm-d Infrastructure           │
│  llmd-operator  │───▶│  ┌─────────────────────────────────────┐ │
│  (meta-operator)│    │  │ ModelService Controller              │ │
│                 │    │  │ (ghcr.io/llm-d/llm-d-model-service) │ │
└─────────────────┘    │  └─────────────────────────────────────┘ │
                       │  ┌─────────────────────────────────────┐ │
                       │  │ Inference Scheduler (EPP)           │ │
                       │  │ (ghcr.io/llm-d/llm-d-inference-    │ │
                       │  │  scheduler)                         │ │
                       │  └─────────────────────────────────────┘ │
                       │  ┌─────────────────────────────────────┐ │
                       │  │ Redis (KV Cache & Smart Routing)    │ │
                       │  └─────────────────────────────────────┘ │
                       │  ┌─────────────────────────────────────┐ │
                       │  │ Gateway API (Inference Extensions)  │ │
                       │  └─────────────────────────────────────┘ │
                       └─────────────────────────────────────────┘
```

## 📁 Sample Files

### **1. `llmd_v1alpha1_llmd.yaml`** - Meta-Operator Configuration
**Purpose**: Deploys the llm-d infrastructure components

**What it creates**:
- **ModelService Controller**: Manages `ModelService` CRs
- **Redis**: For KV cache indexing and smart routing  
- **Gateway API**: For inference-aware routing
- **ConfigMap Presets**: For common ModelService configurations
- **RBAC**: Required permissions for the ModelService controller

```bash
# Deploy llm-d infrastructure
kubectl apply -f config/samples/llmd_v1alpha1_llmd.yaml
```

### **2. `modelservice_v1alpha1_simple.yaml`** - Simple Model Deployment
**Purpose**: Deploys a single model without disaggregated serving

**Configuration**:
- **Image**: `ghcr.io/llm-d/llm-d:0.0.8` (real llm-d runtime)
- **Model**: `meta-llama/Llama-2-7b-chat-hf`
- **Scaling**: Standard serving (prefill + decode in same container)
- **Resources**: 1 GPU, 8Gi memory

```bash
# Deploy simple model (after llm-d infrastructure is ready)
kubectl apply -f config/samples/modelservice_v1alpha1_simple.yaml
```

### **3. `modelservice_v1alpha1_sample.yaml`** - Distributed Model Deployment  
**Purpose**: Demonstrates disaggregated serving with separate prefill/decode

**Configuration**:
- **Images**: Real llm-d components (`ghcr.io/llm-d/*`)
- **Prefill**: 2 replicas (compute-bound)
- **Decode**: 4 replicas (memory bandwidth-bound)  
- **Endpoint Picker**: `ghcr.io/llm-d/llm-d-inference-scheduler:0.0.4`
- **Smart Routing**: Cache-aware load balancing

```bash
# Deploy distributed model (after llm-d infrastructure is ready)
kubectl apply -f config/samples/modelservice_v1alpha1_sample.yaml
```

## 🚀 Quick Start

### **Step 1: Install CRDs**
```bash
# Install both LLMD and ModelService CRDs
kubectl apply -k config/crd
```

### **Step 2: Deploy llm-d Infrastructure**  
```bash
# Deploy the meta-operator configuration
kubectl apply -f config/samples/llmd_v1alpha1_llmd.yaml

# Wait for infrastructure to be ready
kubectl get llmds
kubectl get pods
```

### **Step 3: Deploy a Model**
```bash
# Start with simple deployment
kubectl apply -f config/samples/modelservice_v1alpha1_simple.yaml

# Check model status
kubectl get modelservices
kubectl describe modelservice llama2-7b-simple
```

### **Step 4: Test Inference**
```bash
# Port forward to the model service
kubectl port-forward svc/llama2-7b-simple-prefill 8000:8000

# Test inference
curl -X POST http://localhost:8000/v1/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "meta-llama/Llama-2-7b-chat-hf",
    "prompt": "The capital of France is",
    "max_tokens": 50
  }'
```

## 🔧 Key Differences from Emulation

| **Aspect** | **Previous (Emulation)** | **Current (Real llm-d)** |
|------------|---------------------------|---------------------------|
| **Images** | Custom/fake images | `ghcr.io/llm-d/*` real images |
| **Controller** | Custom Go controller | Real ModelService controller |
| **Scheduler** | Basic Kubernetes scheduler | llm-d inference scheduler (EPP) |
| **Routing** | Standard K8s Services | Gateway API with inference extensions |
| **KV Cache** | ConfigMap | Redis with cache-aware routing |
| **Commands** | Emulated vLLM flags | Real llm-d optimized commands |

## 📚 Real llm-d Components

### **ModelService Controller** 
- **Image**: `ghcr.io/llm-d/llm-d-model-service:0.0.10`
- **Purpose**: Manages ModelService CRs and deploys inference workloads
- **Repository**: https://github.com/llm-d/llm-d-model-service

### **Inference Scheduler (EPP)**
- **Image**: `ghcr.io/llm-d/llm-d-inference-scheduler:0.0.4`  
- **Purpose**: Intelligent routing with cache/load/prefix awareness
- **Features**: KV cache aware scoring, session affinity, prefill/decode routing

### **llm-d Runtime**
- **Image**: `ghcr.io/llm-d/llm-d:0.0.8`
- **Purpose**: Enhanced vLLM with llm-d optimizations
- **Features**: Disaggregated serving, prefix caching, smart batching

## 🎛️ Configuration Options

### **ConfigMap Presets**
The LLMD operator creates preset configurations for common deployments:

- **`basic-gpu-preset`**: Standard GPU deployment
- **`basic-gpu-with-nixl-preset`**: GPU with NIXL enabled  
- **`basic-gpu-with-nixl-and-redis-lookup-preset`**: Full features enabled

### **Monitoring**  
ServiceMonitors are created for:
- ModelService controller metrics
- Endpoint picker metrics  
- Model inference metrics
- Redis metrics

## 🔍 Troubleshooting

### **Common Issues**

1. **ModelService CRD not found**
   ```bash
   # Ensure ModelService CRD is installed
   kubectl get crd modelservices.llm-d.ai
   ```

2. **Infrastructure not ready**
   ```bash
   # Check LLMD status
   kubectl describe llmd llmd-sample
   
   # Check controller logs
   kubectl logs -l control-plane=controller-manager
   ```

3. **Model deployment issues**
   ```bash
   # Check ModelService status
   kubectl describe modelservice <model-name>
   
   # Check pod logs
   kubectl logs -l app.kubernetes.io/name=<model-name>
   ```

## 📖 Further Reading

- [llm-d Documentation](https://github.com/llm-d/llm-d-deployer)
- [Gateway API Inference Extension](https://kubernetes.io/blog/2025/06/05/introducing-gateway-api-inference-extension/)
- [ModelService CRD Reference](https://github.com/llm-d/llm-d-model-service)
- [Inference Scheduler Architecture](https://github.com/llm-d/llm-d-inference-scheduler) 