# Kubernetes Deployment Guide

> Production-ready Kubernetes manifests for deploying the LLM Gateway with high availability, monitoring, and security.

## Table of Contents

- [Quick Start](#quick-start)
- [Prerequisites](#prerequisites)
- [Deployment](#deployment)
- [Configuration](#configuration)
- [Secrets Management](#secrets-management)
- [Monitoring](#monitoring)
- [Storage Options](#storage-options)
- [Scaling](#scaling)
- [Updates and Rollbacks](#updates-and-rollbacks)
- [Security](#security)
- [Cloud Provider Guides](#cloud-provider-guides)
- [Troubleshooting](#troubleshooting)

## Quick Start

Deploy with default settings using pre-built images:

```bash
# Update kustomization.yaml with your image
cd k8s/
vim kustomization.yaml  # Set image to ghcr.io/yourusername/llm-gateway:v1.0.0

# Create secrets
kubectl create namespace llm-gateway
kubectl create secret generic llm-gateway-secrets \
  --from-literal=OPENAI_API_KEY="sk-your-key" \
  --from-literal=ANTHROPIC_API_KEY="sk-ant-your-key" \
  --from-literal=GOOGLE_API_KEY="your-key" \
  -n llm-gateway

# Deploy
kubectl apply -k .

# Verify
kubectl get pods -n llm-gateway
kubectl logs -n llm-gateway -l app=llm-gateway
```

## Prerequisites

- **Kubernetes**: v1.24+ cluster
- **kubectl**: Configured and authenticated
- **Container images**: Access to `ghcr.io/yourusername/llm-gateway`

**Optional but recommended:**
- **Prometheus Operator**: For metrics and alerting
- **cert-manager**: For automatic TLS certificates
- **Ingress Controller**: nginx, ALB, or GCE
- **External Secrets Operator**: For secrets management

## Deployment

### Using Kustomize (Recommended)

```bash
# Review and customize
cd k8s/
vim kustomization.yaml  # Update image, namespace, etc.
vim configmap.yaml      # Configure gateway settings
vim ingress.yaml        # Set your domain

# Deploy all resources
kubectl apply -k .

# Deploy with Kustomize overlays
kubectl apply -k overlays/production/
```

### Using kubectl

```bash
kubectl apply -f namespace.yaml
kubectl apply -f serviceaccount.yaml
kubectl apply -f secret.yaml
kubectl apply -f configmap.yaml
kubectl apply -f redis.yaml
kubectl apply -f deployment.yaml
kubectl apply -f service.yaml
kubectl apply -f ingress.yaml
kubectl apply -f hpa.yaml
kubectl apply -f pdb.yaml
kubectl apply -f networkpolicy.yaml
```

### With Monitoring

If Prometheus Operator is installed:

```bash
kubectl apply -f servicemonitor.yaml
kubectl apply -f prometheusrule.yaml
```

## Configuration

### Image Configuration

Update `kustomization.yaml`:

```yaml
images:
  - name: llm-gateway
    newName: ghcr.io/yourusername/llm-gateway
    newTag: v1.2.3  # Or 'latest', 'main', 'sha-abc123'
```

### Gateway Configuration

Edit `configmap.yaml` for gateway settings:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: llm-gateway-config
data:
  config.yaml: |
    server:
      address: ":8080"

    logging:
      level: info
      format: json

    rate_limit:
      enabled: true
      requests_per_second: 10
      burst: 20

    observability:
      enabled: true
      metrics:
        enabled: true
      tracing:
        enabled: true
        exporter:
          type: otlp
          endpoint: tempo:4317

    conversations:
      store: redis
      dsn: redis://redis:6379/0
      ttl: 1h
```

### Resource Limits

Default resources (adjust based on load testing):

```yaml
resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 1000m
    memory: 512Mi
```

### Ingress Configuration

Edit `ingress.yaml` for your domain:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: llm-gateway
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
spec:
  ingressClassName: nginx
  tls:
    - hosts:
        - llm-gateway.yourdomain.com
      secretName: llm-gateway-tls
  rules:
    - host: llm-gateway.yourdomain.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: llm-gateway
                port:
                  number: 80
```

## Secrets Management

### Option 1: kubectl (Development)

```bash
kubectl create secret generic llm-gateway-secrets \
  --from-literal=OPENAI_API_KEY="sk-..." \
  --from-literal=ANTHROPIC_API_KEY="sk-ant-..." \
  --from-literal=GOOGLE_API_KEY="..." \
  --from-literal=OIDC_AUDIENCE="your-client-id" \
  -n llm-gateway
```

### Option 2: External Secrets Operator (Production)

Install ESO, then create ExternalSecret:

```yaml
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: llm-gateway-secrets
  namespace: llm-gateway
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: aws-secretsmanager  # or vault, gcpsm, etc.
    kind: ClusterSecretStore
  target:
    name: llm-gateway-secrets
  data:
    - secretKey: OPENAI_API_KEY
      remoteRef:
        key: llm-gateway/openai-key
    - secretKey: ANTHROPIC_API_KEY
      remoteRef:
        key: llm-gateway/anthropic-key
    - secretKey: GOOGLE_API_KEY
      remoteRef:
        key: llm-gateway/google-key
```

### Option 3: Sealed Secrets

```bash
# Encrypt secrets
echo -n "sk-your-key" | kubectl create secret generic llm-gateway-secrets \
  --dry-run=client --from-file=OPENAI_API_KEY=/dev/stdin -o yaml | \
  kubeseal -o yaml > sealed-secret.yaml

# Commit sealed-secret.yaml to git
kubectl apply -f sealed-secret.yaml
```

## Monitoring

### Metrics

ServiceMonitor for Prometheus Operator:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: llm-gateway
spec:
  selector:
    matchLabels:
      app: llm-gateway
  endpoints:
    - port: http
      path: /metrics
      interval: 30s
```

**Available metrics:**
- `gateway_requests_total` - Total requests by provider/model
- `gateway_request_duration_seconds` - Request latency histogram
- `gateway_provider_errors_total` - Errors by provider
- `gateway_circuit_breaker_state` - Circuit breaker state changes
- `gateway_rate_limit_hits_total` - Rate limit violations

### Alerts

PrometheusRule with common alerts:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: llm-gateway-alerts
spec:
  groups:
    - name: llm-gateway
      interval: 30s
      rules:
        - alert: HighErrorRate
          expr: rate(gateway_requests_total{status=~"5.."}[5m]) > 0.05
          for: 5m
          annotations:
            summary: High error rate detected

        - alert: PodDown
          expr: kube_deployment_status_replicas_available{deployment="llm-gateway"} < 2
          for: 5m
          annotations:
            summary: Less than 2 gateway pods running
```

### Logging

View logs:

```bash
# Tail logs
kubectl logs -n llm-gateway -l app=llm-gateway -f

# Filter by level
kubectl logs -n llm-gateway -l app=llm-gateway | jq 'select(.level=="error")'

# Search logs
kubectl logs -n llm-gateway -l app=llm-gateway | grep "circuit.*open"
```

### Tracing

Configure OpenTelemetry collector:

```yaml
observability:
  tracing:
    enabled: true
    exporter:
      type: otlp
      endpoint: tempo:4317  # or jaeger-collector:4317
```

## Storage Options

### In-Memory (Default)

No persistence, lost on pod restart:

```yaml
conversations:
  store: memory
```

### Redis (Recommended)

Deploy Redis StatefulSet:

```bash
kubectl apply -f redis.yaml
```

Configure gateway:

```yaml
conversations:
  store: redis
  dsn: redis://redis:6379/0
  ttl: 1h
```

### External Redis

For production, use managed Redis:

```yaml
conversations:
  store: redis
  dsn: redis://:password@redis.example.com:6379/0
  ttl: 1h
```

**Cloud providers:**
- **AWS**: ElastiCache for Redis
- **GCP**: Memorystore for Redis
- **Azure**: Azure Cache for Redis

### PostgreSQL

```yaml
conversations:
  store: sql
  driver: pgx
  dsn: postgres://user:pass@postgres:5432/llm_gateway?sslmode=require
  ttl: 1h
```

## Scaling

### Horizontal Pod Autoscaler

Default HPA configuration:

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: llm-gateway
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: llm-gateway
  minReplicas: 3
  maxReplicas: 20
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
    - type: Resource
      resource:
        name: memory
        target:
          type: Utilization
          averageUtilization: 80
```

Monitor HPA:

```bash
kubectl get hpa -n llm-gateway
kubectl describe hpa llm-gateway -n llm-gateway
```

### Manual Scaling

```bash
# Scale to specific replica count
kubectl scale deployment/llm-gateway --replicas=10 -n llm-gateway

# Check status
kubectl get deployment llm-gateway -n llm-gateway
```

### Pod Disruption Budget

Ensures availability during disruptions:

```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: llm-gateway
spec:
  minAvailable: 2
  selector:
    matchLabels:
      app: llm-gateway
```

## Updates and Rollbacks

### Rolling Updates

```bash
# Update image
kubectl set image deployment/llm-gateway \
  gateway=ghcr.io/yourusername/llm-gateway:v1.2.3 \
  -n llm-gateway

# Watch rollout
kubectl rollout status deployment/llm-gateway -n llm-gateway

# Pause rollout if issues
kubectl rollout pause deployment/llm-gateway -n llm-gateway

# Resume rollout
kubectl rollout resume deployment/llm-gateway -n llm-gateway
```

### Rollback

```bash
# Rollback to previous version
kubectl rollout undo deployment/llm-gateway -n llm-gateway

# Rollback to specific revision
kubectl rollout history deployment/llm-gateway -n llm-gateway
kubectl rollout undo deployment/llm-gateway --to-revision=3 -n llm-gateway
```

### Blue-Green Deployment

```bash
# Deploy new version with different label
kubectl apply -f deployment-v2.yaml

# Test new version
kubectl port-forward -n llm-gateway deployment/llm-gateway-v2 8080:8080

# Switch service to new version
kubectl patch service llm-gateway -n llm-gateway \
  -p '{"spec":{"selector":{"version":"v2"}}}'

# Delete old version after verification
kubectl delete deployment llm-gateway-v1 -n llm-gateway
```

## Security

### Pod Security

Deployment includes security best practices:

```yaml
securityContext:
  runAsNonRoot: true
  runAsUser: 1000
  fsGroup: 1000
  seccompProfile:
    type: RuntimeDefault

containers:
  - name: gateway
    securityContext:
      allowPrivilegeEscalation: false
      readOnlyRootFilesystem: true
      capabilities:
        drop:
          - ALL
```

### Network Policies

Restrict traffic to/from gateway pods:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: llm-gateway
spec:
  podSelector:
    matchLabels:
      app: llm-gateway
  policyTypes:
    - Ingress
    - Egress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              name: ingress-nginx
      ports:
        - protocol: TCP
          port: 8080
  egress:
    - to:  # Allow DNS
        - namespaceSelector: {}
          podSelector:
            matchLabels:
              k8s-app: kube-dns
      ports:
        - protocol: UDP
          port: 53
    - to:  # Allow Redis
        - podSelector:
            matchLabels:
              app: redis
      ports:
        - protocol: TCP
          port: 6379
    - to:  # Allow external LLM providers (HTTPS)
        - namespaceSelector: {}
      ports:
        - protocol: TCP
          port: 443
```

### RBAC

ServiceAccount with minimal permissions:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: llm-gateway
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: llm-gateway
rules:
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: llm-gateway
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: llm-gateway
subjects:
  - kind: ServiceAccount
    name: llm-gateway
```

## Cloud Provider Guides

### AWS EKS

```bash
# Install AWS Load Balancer Controller
kubectl apply -k "github.com/aws/eks-charts/stable/aws-load-balancer-controller//crds?ref=master"
helm install aws-load-balancer-controller eks/aws-load-balancer-controller \
  -n kube-system \
  --set clusterName=my-cluster

# Update ingress for ALB
# Add annotations to ingress.yaml:
metadata:
  annotations:
    kubernetes.io/ingress.class: alb
    alb.ingress.kubernetes.io/scheme: internet-facing
    alb.ingress.kubernetes.io/target-type: ip
```

**IRSA for secrets:**

```bash
# Create IAM role and associate with ServiceAccount
eksctl create iamserviceaccount \
  --name llm-gateway \
  --namespace llm-gateway \
  --cluster my-cluster \
  --attach-policy-arn arn:aws:iam::aws:policy/SecretsManagerReadWrite \
  --approve
```

**ElastiCache Redis:**

```yaml
conversations:
  store: redis
  dsn: redis://my-cluster.cache.amazonaws.com:6379/0
```

### GCP GKE

```bash
# Enable Workload Identity
gcloud container clusters update my-cluster \
  --workload-pool=PROJECT_ID.svc.id.goog

# Create service account with Secret Manager access
gcloud iam service-accounts create llm-gateway

gcloud projects add-iam-policy-binding PROJECT_ID \
  --member "serviceAccount:llm-gateway@PROJECT_ID.iam.gserviceaccount.com" \
  --role "roles/secretmanager.secretAccessor"

# Bind K8s SA to GCP SA
kubectl annotate serviceaccount llm-gateway \
  -n llm-gateway \
  iam.gke.io/gcp-service-account=llm-gateway@PROJECT_ID.iam.gserviceaccount.com
```

**Memorystore Redis:**

```yaml
conversations:
  store: redis
  dsn: redis://10.0.0.3:6379/0  # Private IP from Memorystore
```

### Azure AKS

```bash
# Install Application Gateway Ingress Controller
az aks enable-addons \
  --resource-group myResourceGroup \
  --name myAKSCluster \
  --addons ingress-appgw \
  --appgw-name myApplicationGateway

# Configure Azure AD Workload Identity
az aks update \
  --resource-group myResourceGroup \
  --name myAKSCluster \
  --enable-oidc-issuer \
  --enable-workload-identity
```

**Azure Key Vault with ESO:**

```yaml
apiVersion: external-secrets.io/v1beta1
kind: SecretStore
metadata:
  name: azure-keyvault
spec:
  provider:
    azurekv:
      authType: WorkloadIdentity
      vaultUrl: https://my-vault.vault.azure.net
```

## Troubleshooting

### Pods Not Starting

```bash
# Check pod status
kubectl get pods -n llm-gateway

# Describe pod for events
kubectl describe pod llm-gateway-xxx -n llm-gateway

# Check logs
kubectl logs -n llm-gateway llm-gateway-xxx

# Check previous container logs (if crashed)
kubectl logs -n llm-gateway llm-gateway-xxx --previous
```

**Common issues:**
- Image pull errors: Check registry credentials
- CrashLoopBackOff: Check logs for startup errors
- Pending: Check resource quotas and node capacity

### Health Check Failures

```bash
# Port-forward to test locally
kubectl port-forward -n llm-gateway svc/llm-gateway 8080:80

# Test endpoints
curl http://localhost:8080/health
curl http://localhost:8080/ready

# Check from inside pod
kubectl exec -n llm-gateway deployment/llm-gateway -- wget -O- http://localhost:8080/health
```

### Provider Connection Issues

```bash
# Test egress from pod
kubectl exec -n llm-gateway deployment/llm-gateway -- wget -O- https://api.openai.com

# Check secrets
kubectl get secret llm-gateway-secrets -n llm-gateway -o jsonpath='{.data.OPENAI_API_KEY}' | base64 -d

# Verify network policies
kubectl get networkpolicy -n llm-gateway
kubectl describe networkpolicy llm-gateway -n llm-gateway
```

### Redis Connection Issues

```bash
# Test Redis connectivity
kubectl exec -n llm-gateway deployment/llm-gateway -- nc -zv redis 6379

# Connect to Redis
kubectl exec -it -n llm-gateway redis-0 -- redis-cli

# Check Redis logs
kubectl logs -n llm-gateway redis-0
```

### Performance Issues

```bash
# Check resource usage
kubectl top pods -n llm-gateway
kubectl top nodes

# Check HPA status
kubectl describe hpa llm-gateway -n llm-gateway

# Check for throttling
kubectl describe pod llm-gateway-xxx -n llm-gateway | grep -i throttl
```

### Debug Container

For distroless/minimal images:

```bash
# Use ephemeral debug container
kubectl debug -it -n llm-gateway llm-gateway-xxx --image=busybox --target=gateway

# Or use debug pod
kubectl run debug --rm -it --image=nicolaka/netshoot -n llm-gateway -- /bin/bash
```

## Useful Commands

```bash
# View all resources
kubectl get all -n llm-gateway

# Check deployment status
kubectl rollout status deployment/llm-gateway -n llm-gateway

# Tail logs from all pods
kubectl logs -n llm-gateway -l app=llm-gateway -f --max-log-requests=10

# Get events
kubectl get events -n llm-gateway --sort-by='.lastTimestamp'

# Check resource quotas
kubectl describe resourcequota -n llm-gateway

# Export current config
kubectl get deployment llm-gateway -n llm-gateway -o yaml > deployment-backup.yaml

# Force pod restart
kubectl rollout restart deployment/llm-gateway -n llm-gateway

# Delete and recreate deployment
kubectl delete deployment llm-gateway -n llm-gateway
kubectl apply -f deployment.yaml
```

## Architecture Overview

```
┌─────────────────────────────────────────────────┐
│           Internet / Load Balancer              │
└────────────────────┬────────────────────────────┘
                     │
                     ▼
          ┌──────────────────────┐
          │  Ingress Controller  │
          │    (TLS/SSL)         │
          └──────────┬───────────┘
                     │
                     ▼
          ┌──────────────────────┐
          │  Gateway Service     │
          │   (ClusterIP:80)     │
          └──────────┬───────────┘
                     │
        ┌────────────┼────────────┐
        ▼            ▼            ▼
    ┌─────┐      ┌─────┐      ┌─────┐
    │ Pod │      │ Pod │      │ Pod │
    │  1  │      │  2  │      │  3  │
    └──┬──┘      └──┬──┘      └──┬──┘
       │            │            │
       └────────────┼────────────┘
                    │
       ┌────────────┼────────────┐
       ▼            ▼            ▼
   ┌──────┐    ┌──────┐    ┌──────┐
   │Redis │    │Prom  │    │Tempo │
   └──────┘    └──────┘    └──────┘
```

## Additional Resources

- [Main Documentation](../README.md)
- [Docker Deployment](../docs/DOCKER_DEPLOYMENT.md)
- [Kubernetes Best Practices](https://kubernetes.io/docs/concepts/configuration/overview/)
- [Prometheus Operator](https://prometheus-operator.dev/)
- [External Secrets Operator](https://external-secrets.io/)
- [cert-manager](https://cert-manager.io/)
