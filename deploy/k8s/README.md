# Agent Exchange (AEX) - Kubernetes Deployment

Production-ready Kubernetes manifests for deploying the Agent Exchange code review demo, including all AEX core microservices, code review agents, payment agents, and the NiceGUI dashboard.

## Architecture Overview

```
                         Ingress (nginx)
                        /              \
                       /                \
              /api/* -->              / -->
          aex-gateway:8080      demo-ui-nicegui:8502
               |                       |
    +----------+----------+            |
    |    AEX Core Services |           |
    |  (11 Go microservices)|          |
    +----------+----------+            |
               |                       |
         +-----+-----+         +------+------+
         |  MongoDB   |         | Code Review |
         | StatefulSet|         |   Agents    |
         +------------+         |  + Payment  |
                                |   Agents    |
                                +-------------+
```

### Services

| Category | Services | Count |
|----------|----------|-------|
| Database | MongoDB 7 (StatefulSet) | 1 |
| AEX Core | gateway, work-publisher, bid-gateway, bid-evaluator, contract-engine, provider-registry, trust-broker, identity, settlement, telemetry, credentials-provider | 11 |
| Code Review Agents | code-reviewer-a (QuickReview), code-reviewer-b (CodeGuard), code-reviewer-c (ArchitectAI), orchestrator | 4 |
| Payment Agents | payment-devpay, payment-codeauditpay, payment-securitypay | 3 |
| UI | demo-ui-nicegui (NiceGUI WebSocket dashboard) | 1 |
| **Total** | | **20** |

## Prerequisites

- Kubernetes cluster (v1.25+)
- `kubectl` (v1.25+)
- `kustomize` (v5.0+) or `kubectl` with kustomize support
- Nginx Ingress Controller (for ingress routing)
- Container images built and pushed to a registry

### For Local Development

- [minikube](https://minikube.sigs.k8s.io/) or [kind](https://kind.sigs.k8s.io/)
- Docker (for building images locally)

## Quick Start (Local Development)

### 1. Start a Local Cluster

**Using minikube:**

```bash
minikube start --memory 8192 --cpus 4
minikube addons enable ingress
```

**Using kind:**

```bash
kind create cluster --name aex
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/kind/deploy.yaml
```

### 2. Set Up Secrets

Before deploying, create the secrets with your actual values:

```bash
# Create namespace first
kubectl apply -f deploy/k8s/namespace.yaml

# Create the secret (replace placeholder values)
kubectl create secret generic aex-secrets \
  --namespace aex \
  --from-literal=ANTHROPIC_API_KEY="sk-ant-your-key-here" \
  --from-literal=JWT_SIGNING_KEY="your-jwt-signing-key" \
  --from-literal=MONGO_URI="mongodb://root:root@mongodb.aex.svc.cluster.local:27017/?authSource=admin"
```

### 3. Deploy with Kustomize

**Development (local images, 1 replica, small resources):**

```bash
kubectl apply -k deploy/k8s/overlays/dev/
```

**Staging (2 replicas, moderate resources):**

```bash
kubectl apply -k deploy/k8s/overlays/staging/
```

**Production (HPA, PDB, NetworkPolicy, larger resources):**

```bash
kubectl apply -k deploy/k8s/overlays/production/
```

### 4. Verify Deployment

```bash
# Check all pods are running
kubectl get pods -n aex

# Check all services
kubectl get svc -n aex

# Watch pod status
kubectl get pods -n aex -w

# Check deployment rollout status
kubectl rollout status deployment/aex-gateway -n aex
```

### 5. Access the Application

**With minikube:**

```bash
# Get the UI URL
minikube service demo-ui-nicegui -n aex --url

# Or use port-forward
kubectl port-forward svc/demo-ui-nicegui 8502:8502 -n aex
```

**With Ingress:**

```bash
# Get the ingress address
kubectl get ingress -n aex

# Access:
# UI:  http://<INGRESS_IP>/
# API: http://<INGRESS_IP>/api/
```

## Directory Structure

```
deploy/k8s/
├── README.md                              # This file
├── namespace.yaml                         # aex namespace
├── base/                                  # Base Kustomize configuration
│   ├── kustomization.yaml                 # Assembles all resources
│   ├── namespace.yaml                     # Namespace definition
│   ├── configmap.yaml                     # Shared env vars and service URLs
│   └── secrets.yaml                       # Secret template (DO NOT commit real values)
├── services/                              # AEX Core Services (Go microservices)
│   ├── mongodb/
│   │   ├── statefulset.yaml               # MongoDB StatefulSet with PVC
│   │   └── service.yaml                   # ClusterIP service
│   ├── aex-gateway/
│   │   ├── deployment.yaml                # API gateway deployment
│   │   └── service.yaml
│   ├── aex-work-publisher/
│   │   ├── deployment.yaml
│   │   └── service.yaml
│   ├── aex-bid-gateway/
│   │   ├── deployment.yaml
│   │   └── service.yaml
│   ├── aex-bid-evaluator/
│   │   ├── deployment.yaml
│   │   └── service.yaml
│   ├── aex-contract-engine/
│   │   ├── deployment.yaml
│   │   └── service.yaml
│   ├── aex-provider-registry/
│   │   ├── deployment.yaml
│   │   └── service.yaml
│   ├── aex-trust-broker/
│   │   ├── deployment.yaml
│   │   └── service.yaml
│   ├── aex-identity/
│   │   ├── deployment.yaml
│   │   └── service.yaml
│   ├── aex-settlement/
│   │   ├── deployment.yaml
│   │   └── service.yaml
│   ├── aex-telemetry/
│   │   ├── deployment.yaml
│   │   └── service.yaml
│   └── aex-credentials-provider/
│       ├── deployment.yaml
│       └── service.yaml
├── agents/                                # Code Review Demo Agents (Python)
│   ├── code-reviewer-a/
│   │   ├── deployment.yaml                # QuickReview - Budget reviews
│   │   └── service.yaml
│   ├── code-reviewer-b/
│   │   ├── deployment.yaml                # CodeGuard - Security-focused
│   │   └── service.yaml
│   ├── code-reviewer-c/
│   │   ├── deployment.yaml                # ArchitectAI - Architecture review
│   │   └── service.yaml
│   ├── orchestrator/
│   │   ├── deployment.yaml                # Workflow coordinator
│   │   └── service.yaml
│   ├── payment-devpay/
│   │   ├── deployment.yaml                # General dev payments
│   │   └── service.yaml
│   ├── payment-codeauditpay/
│   │   ├── deployment.yaml                # Code audit payments
│   │   └── service.yaml
│   └── payment-securitypay/
│       ├── deployment.yaml                # Security payments
│       └── service.yaml
├── ui/
│   ├── deployment.yaml                    # NiceGUI real-time dashboard
│   └── service.yaml                       # LoadBalancer service
├── ingress/
│   └── ingress.yaml                       # Nginx Ingress routing
└── overlays/
    ├── dev/
    │   └── kustomization.yaml             # Local dev (1 replica, NodePort, small resources)
    ├── staging/
    │   └── kustomization.yaml             # Staging (2 replicas, moderate resources)
    └── production/
        ├── kustomization.yaml             # Production config
        ├── hpa.yaml                       # HorizontalPodAutoscalers (2-10 replicas)
        ├── pdb.yaml                       # PodDisruptionBudgets
        └── networkpolicy.yaml             # Network policies (default deny + allow rules)
```

## Configuration

### Environment Variables

All shared configuration is managed through the `aex-config` ConfigMap. Service URLs use Kubernetes DNS:

```
http://<service-name>.aex.svc.cluster.local:<port>
```

### Secrets Management

The `base/secrets.yaml` is a template. **Never commit real secret values.**

For production, use one of:
- **Sealed Secrets**: Encrypt secrets in the repository
- **External Secrets Operator**: Sync from AWS Secrets Manager, HashiCorp Vault, etc.
- **SOPS**: Mozilla SOPS for encrypted secrets in git

### Image Configuration

Base manifests use placeholder image names (`${REGISTRY}/image-name:${TAG}`). Each overlay sets the actual registry and tag via the Kustomize `images` transformer.

To update image tags for a deployment:

```bash
# Update a specific image tag
cd deploy/k8s/overlays/production/
kustomize edit set image YOUR_ACCOUNT.dkr.ecr.YOUR_REGION.amazonaws.com/aex-gateway=YOUR_ACCOUNT.dkr.ecr.YOUR_REGION.amazonaws.com/aex-gateway:v1.2.3
```

## Overlay Details

### Dev Overlay

- 1 replica for all services
- NodePort for UI (port 30502)
- Local image names (`agent-exchange/*:local`)
- 1Gi MongoDB PVC
- Small resource requests/limits

### Staging Overlay

- 2 replicas for core services
- Moderate resources (256Mi-512Mi request, 512Mi-1Gi limit)
- 5Gi MongoDB PVC
- ECR image placeholders

### Production Overlay

- 2-3 base replicas + HPA (scales to 6-10)
- HorizontalPodAutoscalers on key services
- PodDisruptionBudgets (minAvailable: 1)
- NetworkPolicies (default deny + allow rules)
- Node affinity for workload isolation
- Pod anti-affinity for spread across nodes
- TLS on Ingress (cert-manager integration)
- 10Gi MongoDB PVC
- Large resource limits (512Mi-2Gi)

## Monitoring and Troubleshooting

### Health Checks

All services expose `/health` endpoints. Kubernetes uses these for liveness and readiness probes.

```bash
# Check health of a specific service
kubectl exec -n aex deploy/aex-gateway -- wget -qO- http://localhost:8080/health

# Check all pod health
kubectl get pods -n aex -o wide
```

### Logs

```bash
# View logs for a specific service
kubectl logs -n aex deploy/aex-gateway -f

# View logs for all pods with a label
kubectl logs -n aex -l app.kubernetes.io/component=code-review-agent -f

# View previous container logs (if crashed)
kubectl logs -n aex deploy/code-reviewer-a --previous
```

### Common Issues

**Pods stuck in Pending:**
```bash
kubectl describe pod -n aex <pod-name>
# Check for resource constraints or PVC binding issues
```

**Pods in CrashLoopBackOff:**
```bash
kubectl logs -n aex <pod-name> --previous
# Check for missing env vars, connection issues, or startup failures
```

**MongoDB connection failures:**
```bash
# Verify MongoDB is running and ready
kubectl get pods -n aex -l app.kubernetes.io/name=mongodb
kubectl exec -n aex mongodb-0 -- mongosh --eval "db.adminCommand('ping')"
```

**Services not discovering each other:**
```bash
# Test DNS resolution
kubectl exec -n aex deploy/aex-gateway -- nslookup aex-bid-gateway.aex.svc.cluster.local

# Verify ConfigMap values
kubectl get configmap aex-config -n aex -o yaml
```

### Scaling

```bash
# Manual scaling
kubectl scale deployment aex-gateway -n aex --replicas=5

# Check HPA status (production)
kubectl get hpa -n aex

# View HPA details
kubectl describe hpa aex-gateway-hpa -n aex
```

### Resource Usage

```bash
# Pod resource usage (requires metrics-server)
kubectl top pods -n aex

# Node resource usage
kubectl top nodes
```

## Cleanup

```bash
# Delete all resources in the namespace
kubectl delete namespace aex

# Or delete specific overlay
kubectl delete -k deploy/k8s/overlays/dev/
```
