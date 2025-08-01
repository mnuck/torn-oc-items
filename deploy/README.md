# Kubernetes Deployment Guide

This directory contains Kubernetes manifests for deploying the torn-oc-items application.

## Files Overview

- `deployment.yaml`: Main application deployment with security contexts and resource limits
- `service.yaml`: Headless service for DNS resolution (batch application, no HTTP endpoints)
- `configmap.yaml`: Non-sensitive configuration values
- `torn-secret.yaml`: Template for sensitive configuration (API keys, credentials)
- `env.template`: Template for creating the .env file

## Quick Start

### 1. Prepare Configuration Files

First, create your `.env` file based on `env.template`:

```bash
cp env.template .env
# Edit .env with your actual values
```

Ensure you have your Google Sheets `credentials.json` file ready.

### 2. Create Kubernetes Secret

Option A - From files (recommended):
```bash
kubectl create secret generic torn-oc-secrets \
  --from-file=.env \
  --from-file=credentials.json
```

Option B - Using the template:
```bash
# Base64 encode your files
cat .env | base64 -w 0
cat credentials.json | base64 -w 0

# Edit torn-secret.yaml with the base64 values
kubectl apply -f torn-secret.yaml
```

### 3. Deploy Application

```bash
# Apply all manifests
kubectl apply -f .

# Or apply in order
kubectl apply -f configmap.yaml
kubectl apply -f torn-secret.yaml  # Only if using Option B above
kubectl apply -f deployment.yaml
kubectl apply -f service.yaml
```

### 4. Verify Deployment

```bash
# Check pod status
kubectl get pods -l app=torn-oc-items

# View logs
kubectl logs -l app=torn-oc-items -f

# Check resource usage
kubectl top pods -l app=torn-oc-items
```

## Security Features

- **Non-root execution**: Runs as UID 65532 (distroless nonroot user)
- **Read-only root filesystem**: Prevents runtime modifications
- **No privileged escalation**: Security constraints enforced
- **Minimal capabilities**: All Linux capabilities dropped
- **Distroless base image**: No shell, package manager, or unnecessary tools
- **Rolling updates**: Zero-downtime deployments with health checks
- **Resource limits**: Prevents resource exhaustion

## Configuration Management

- **Secrets**: API keys and credentials stored securely in Kubernetes secrets
- **ConfigMap**: Non-sensitive configuration in configmap for easy updates
- **Environment-specific**: Override values for different environments

## Monitoring & Troubleshooting

### Common Commands

```bash
# View application logs
kubectl logs -l app=torn-oc-items --tail=100

# Get pod details
kubectl describe pods -l app=torn-oc-items

# Execute into pod (limited - distroless image)
kubectl exec -it deployment/torn-oc-items -- sh  # Will fail - no shell

# Port forward for debugging (if app had HTTP endpoints)
# kubectl port-forward deployment/torn-oc-items 8080:8080
```

### Health Monitoring

Since this is a batch application without HTTP endpoints, monitor through:

- **Logs**: Application logs for errors and API call summaries
- **Pod status**: Kubernetes pod restart count and status
- **Resource usage**: Memory and CPU consumption patterns
- **Application metrics**: API call counts and processing statistics in logs

### Scaling

```bash
# Scale replicas (usually keep at 1 for batch processing)
kubectl scale deployment torn-oc-items --replicas=1

# View resource usage for scaling decisions
kubectl top pods -l app=torn-oc-items
```

## CI/CD Integration

This deployment integrates with GitHub Actions workflows:

1. **CI Pipeline**: Builds, tests, and scans the application
2. **CD Pipeline**: Creates and signs container images  
3. **Deploy Pipeline**: Updates deployment manifests and validates Kubernetes configs

The deploy workflow will automatically update the image tag in `deployment.yaml` when triggered.

## Legacy Instructions (for reference)

### Manual Secret Creation Process

If you need to manually update secrets after deployment:

1. Get current .env from secret:
   ```bash
   kubectl get secret torn-oc-secrets -o json | jq -r '.data[".env"]' | base64 -d > .env
   ```

2. Edit the `.env` file with new values

3. Update secret:
   ```bash
   ENV_CONTENT=$(cat .env | base64 -w 0)
   sed -i "s/your_base64_encoded_env_file_content_here/$ENV_CONTENT/" torn-secret.yaml
   kubectl apply -f torn-secret.yaml
   ```

4. Restart deployment:
   ```bash
   kubectl rollout restart deployment torn-oc-items
   ```
