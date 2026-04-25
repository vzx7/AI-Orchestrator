# AI Orchestrator V5 — Kubernetes Deployment

## Prerequisites

- Kubernetes 1.26+
- kubectl configured
- Ingress controller (optional)

## Quick Start

### 1. Clone and configure

```bash
cd deploy/k8s

# Update image references
sed -i 's/ghcr.io\/your-org\/your-org/ghcr.io\/your-org\/your-org/g' *.yaml
```

### 2. Create namespace and deploy

```bash
# Apply all resources
kubectl apply -k .

# Or apply individually
kubectl apply -f 00-namespace.yaml
kubectl apply -f .
```

### 3. Check status

```bash
kubectl get pods -n ai-orchestrator
kubectl get svc -n ai-orchestrator
```

### 4. Access the API

```bash
# Get external IP (for LoadBalancer)
kubectl get svc orchestrator -n ai-orchestrator

# Or port-forward for local access
kubectl port-forward -n ai-orchestrator svc/orchestrator 8080:80
```

## Resources

| File | Resource | Description |
|------|----------|-------------|
| 00-namespace.yaml | Namespace | ai-orchestrator |
| 01-configmap.yaml | ConfigMap | Environment config |
| 02-secrets.yaml | Secret | API keys, passwords |
| 03-postgres.yaml | Deployment + Service | PostgreSQL |
| 04-redis.yaml | Deployment + Service | Redis |
| 05-orchestrator.yaml | Deployment + Service | Main API |
| 06-workers.yaml | Deployment + Service | Workers |
| 07-hpa.yaml | HorizontalPodAutoscaler | Auto-scaling |
| 08-pvc.yaml | PersistentVolumeClaim | Storage |
| 09-init-configmap.yaml | ConfigMap | DB init SQL |
| kustomization.yaml | Kustomization | Build config |

## Scaling

### Manual scaling

```bash
# Scale workers
kubectl scale deployment worker -n ai-orchestrator --replicas=5

# Scale orchestrator
kubectl scale deployment orchestrator -n ai-orchestrator --replicas=2
```

### Auto-scaling (HPA)

```bash
# Check HPA
kubectl get hpa -n ai-orchestrator

# Manual trigger for testing
kubectl autoscale deployment orchestrator -n ai-orchestrator --cpu-percent=70 --min=1 --max=5
```

## Monitoring

### Check pods

```bash
kubectl get pods -n ai-orchestrator -o wide
```

### Check logs

```bash
kubectl logs -n ai-orchestrator deployment/orchestrator -f
kubectl logs -n ai-orchestrator deployment/worker -f
```

### Check resources

```bash
kubectl top nodes
kubectl top pods -n ai-orchestrator
```

## Ingress (Optional)

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: orchestrator-ingress
  namespace: ai-orchestrator
  annotations:
    nginx.ingress.kubernetes.io/rewrite-target: /
spec:
  rules:
  - host: orchestrator.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: orchestrator
            port:
              number: 80
```

## Troubleshooting

### Pods not starting

```bash
# Check events
kubectl describe pod <pod-name> -n ai-orchestrator

# Check logs
kubectl logs <pod-name> -n ai-orchestrator
```

### Database connection issues

```bash
# Check PostgreSQL
kubectl exec -it postgres-0 -n ai-orchestrator -- psql -U admin -d orchestrator -c "SELECT 1"

# Check service
kubectl get svc postgres -n ai-orchestrator
kubectl describe svc postgres -n ai-orchestrator
```

### Out of memory

```bash
# Check resource limits
kubectl describe pod <pod-name> -n ai-orchestrator | grep -A 10 "Limits:"

# Adjust in deployment
kubectl edit deployment orchestrator -n ai-orchestrator
```

## Cleanup

```bash
# Delete all resources
kubectl delete -k .

# Or individually
kubectl delete -f . -n ai-orchestrator
kubectl delete namespace ai-orchestrator
```

## Production Considerations

1. **Secrets**: Use external secrets manager (Vault, AWS Secrets Manager)
2. **TLS**: Configure TLS termination in Ingress
3. **Monitoring**: Add Prometheus/Grafana
4. **Backup**: Schedule PostgreSQL backups
5. **NetworkPolicies**: Restrict pod-to-pod communication