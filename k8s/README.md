# Kubernetes Deployment Guide

This directory contains Kubernetes manifests for deploying the LLM Gateway to production.

## Prerequisites

- Kubernetes cluster (v1.24+)
- `kubectl` configured
- Container registry access
- (Optional) Prometheus Operator for monitoring
- (Optional) cert-manager for TLS certificates
- (Optional) nginx-ingress-controller or cloud load balancer

## Quick Start

### 1. Build and Push Docker Image

```bash
# Build the image
docker build -t your-registry/llm-gateway:v1.0.0 .

# Push to registry
docker push your-registry/llm-gateway:v1.0.0
```

### 2. Configure Secrets

**Option A: Using kubectl**
```bash
kubectl create namespace llm-gateway

kubectl create secret generic llm-gateway-secrets \
  --from-literal=GOOGLE_API_KEY="your-key" \
  --from-literal=ANTHROPIC_API_KEY="your-key" \
  --from-literal=OPENAI_API_KEY="your-key" \
  --from-literal=OIDC_AUDIENCE="your-client-id" \
  -n llm-gateway
```

**Option B: Using External Secrets Operator (Recommended)**
- Uncomment the ExternalSecret in `secret.yaml`
- Configure your SecretStore (AWS Secrets Manager, Vault, etc.)

### 3. Update Configuration

Edit `configmap.yaml`:
- Update Redis connection string if using external Redis
- Configure observability endpoints (Tempo, Prometheus)
- Adjust rate limits as needed
- Set OIDC issuer and audience

Edit `ingress.yaml`:
- Replace `llm-gateway.example.com` with your domain
- Configure TLS certificate annotations

Edit `kustomization.yaml`:
- Update image registry and tag

### 4. Deploy

**Using Kustomize (Recommended):**
```bash
kubectl apply -k k8s/
```

**Using kubectl directly:**
```bash
kubectl apply -f k8s/namespace.yaml
kubectl apply -f k8s/serviceaccount.yaml
kubectl apply -f k8s/secret.yaml
kubectl apply -f k8s/configmap.yaml
kubectl apply -f k8s/redis.yaml
kubectl apply -f k8s/deployment.yaml
kubectl apply -f k8s/service.yaml
kubectl apply -f k8s/ingress.yaml
kubectl apply -f k8s/hpa.yaml
kubectl apply -f k8s/pdb.yaml
kubectl apply -f k8s/networkpolicy.yaml
```

**With Prometheus Operator:**
```bash
kubectl apply -f k8s/servicemonitor.yaml
kubectl apply -f k8s/prometheusrule.yaml
```

### 5. Verify Deployment

```bash
# Check pods
kubectl get pods -n llm-gateway

# Check services
kubectl get svc -n llm-gateway

# Check ingress
kubectl get ingress -n llm-gateway

# View logs
kubectl logs -n llm-gateway -l app=llm-gateway --tail=100 -f

# Check health
kubectl port-forward -n llm-gateway svc/llm-gateway 8080:80
curl http://localhost:8080/health
```

## Architecture Overview

```
┌─────────────────────────────────────────────────────────┐
│                    Internet/Clients                      │
└───────────────────────┬─────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────┐
│                  Ingress Controller                      │
│            (nginx/ALB/GCE with TLS)                     │
└───────────────────────┬─────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────┐
│                  LLM Gateway Service                     │
│                    (LoadBalancer)                        │
└───────────────────────┬─────────────────────────────────┘
                        │
        ┌───────────────┼───────────────┐
        ▼               ▼               ▼
┌──────────────┐ ┌──────────────┐ ┌──────────────┐
│   Gateway    │ │   Gateway    │ │   Gateway    │
│   Pod 1      │ │   Pod 2      │ │   Pod 3      │
└──────┬───────┘ └──────┬───────┘ └──────┬───────┘
       │                │                │
       └────────────────┼────────────────┘
                        │
        ┌───────────────┼───────────────┐
        ▼               ▼               ▼
┌──────────────┐ ┌──────────────┐ ┌──────────────┐
│    Redis     │ │  Prometheus  │ │    Tempo     │
│ (Persistent) │ │  (Metrics)   │ │  (Traces)    │
└──────────────┘ └──────────────┘ └──────────────┘
```

## Resource Specifications

### Default Resources
- **Requests**: 100m CPU, 128Mi memory
- **Limits**: 1000m CPU, 512Mi memory
- **Replicas**: 3 (min), 20 (max with HPA)

### Scaling
- HPA scales based on CPU (70%) and memory (80%)
- PodDisruptionBudget ensures minimum 2 replicas during disruptions

## Configuration Options

### Environment Variables (from Secret)
- `GOOGLE_API_KEY`: Google AI API key
- `ANTHROPIC_API_KEY`: Anthropic API key
- `OPENAI_API_KEY`: OpenAI API key
- `OIDC_AUDIENCE`: OIDC client ID for authentication

### ConfigMap Settings
See `configmap.yaml` for full configuration options:
- Server address
- Logging format and level
- Rate limiting
- Observability (metrics/tracing)
- Provider endpoints
- Conversation storage
- Authentication

## Security

### Security Features
- Non-root container execution (UID 1000)
- Read-only root filesystem
- No privilege escalation
- All capabilities dropped
- Network policies for ingress/egress control
- SeccompProfile: RuntimeDefault

### TLS/HTTPS
- Ingress configured with TLS
- Uses cert-manager for automatic certificate provisioning
- Force SSL redirect enabled

### Secrets Management
**Never commit secrets to git!**

Production options:
1. **External Secrets Operator** (Recommended)
   - AWS Secrets Manager
   - HashiCorp Vault
   - Google Secret Manager

2. **Sealed Secrets**
   - Encrypted secrets in git

3. **Manual kubectl secrets**
   - Created outside of git

## Monitoring

### Metrics
- Exposed on `/metrics` endpoint
- Scraped by Prometheus via ServiceMonitor
- Key metrics:
  - HTTP request rate, latency, errors
  - Provider request rate, latency, token usage
  - Conversation store operations
  - Rate limiting hits

### Alerts
See `prometheusrule.yaml` for configured alerts:
- High error rate
- High latency
- Provider failures
- Pod down
- High memory usage
- Rate limit threshold exceeded
- Conversation store errors

### Logs
Structured JSON logs with:
- Request IDs
- Trace context (trace_id, span_id)
- Log levels (debug/info/warn/error)

View logs:
```bash
kubectl logs -n llm-gateway -l app=llm-gateway --tail=100 -f
```

## Maintenance

### Rolling Updates
```bash
# Update image
kubectl set image deployment/llm-gateway gateway=your-registry/llm-gateway:v1.0.1 -n llm-gateway

# Check rollout status
kubectl rollout status deployment/llm-gateway -n llm-gateway

# Rollback if needed
kubectl rollout undo deployment/llm-gateway -n llm-gateway
```

### Scaling
```bash
# Manual scale
kubectl scale deployment/llm-gateway --replicas=5 -n llm-gateway

# HPA will auto-scale within min/max bounds (3-20)
```

### Configuration Updates
```bash
# Edit ConfigMap
kubectl edit configmap llm-gateway-config -n llm-gateway

# Restart pods to pick up changes
kubectl rollout restart deployment/llm-gateway -n llm-gateway
```

### Debugging
```bash
# Exec into pod
kubectl exec -it -n llm-gateway deployment/llm-gateway -- /bin/sh

# Port forward for local access
kubectl port-forward -n llm-gateway svc/llm-gateway 8080:80

# Check events
kubectl get events -n llm-gateway --sort-by='.lastTimestamp'
```

## Production Considerations

### High Availability
- Minimum 3 replicas across availability zones
- Pod anti-affinity rules spread pods across nodes
- PodDisruptionBudget ensures service availability during disruptions

### Performance
- Adjust resource limits based on load testing
- Configure HPA thresholds based on traffic patterns
- Use node affinity for GPU nodes if needed

### Cost Optimization
- Use spot/preemptible instances for non-critical workloads
- Set appropriate resource requests/limits
- Monitor token usage and implement quotas

### Disaster Recovery
- Redis persistence (if using StatefulSet)
- Regular backups of conversation data
- Multi-region deployment for geo-redundancy
- Document runbooks for incident response

## Cloud-Specific Notes

### AWS EKS
- Use AWS Load Balancer Controller for ALB
- Configure IRSA for service account
- Use ElastiCache for Redis
- Store secrets in AWS Secrets Manager

### GCP GKE
- Use GKE Ingress for GCLB
- Configure Workload Identity
- Use Memorystore for Redis
- Store secrets in Google Secret Manager

### Azure AKS
- Use Azure Application Gateway Ingress Controller
- Configure Azure AD Workload Identity
- Use Azure Cache for Redis
- Store secrets in Azure Key Vault

## Troubleshooting

### Common Issues

**Pods not starting:**
```bash
kubectl describe pod -n llm-gateway -l app=llm-gateway
kubectl logs -n llm-gateway -l app=llm-gateway --previous
```

**Health check failures:**
```bash
kubectl port-forward -n llm-gateway deployment/llm-gateway 8080:8080
curl http://localhost:8080/health
curl http://localhost:8080/ready
```

**Provider connection issues:**
- Verify API keys in secrets
- Check network policies allow egress
- Verify provider endpoints are accessible

**Redis connection issues:**
```bash
kubectl exec -it -n llm-gateway redis-0 -- redis-cli ping
```

## Additional Resources

- [Kubernetes Documentation](https://kubernetes.io/docs/)
- [Prometheus Operator](https://github.com/prometheus-operator/prometheus-operator)
- [cert-manager](https://cert-manager.io/)
- [External Secrets Operator](https://external-secrets.io/)
