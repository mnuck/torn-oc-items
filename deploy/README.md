# Kubernetes Deployment

This directory contains Kubernetes manifests for deploying the Torn OC Items application.

## Setup

### 1. Prepare your .env file

Copy `env.template` to create your actual `.env` file:

```bash
cp env.template .env
```

Edit the `.env` file with your actual values:

- `TORN_API_KEY`: Your Torn API key with log access
- `TORN_FACTION_API_KEY`: Your Torn Faction API key
- `SPREADSHEET_ID`: Your Google Spreadsheet ID
- Other configuration as needed

### 2. Prepare Google Sheets credentials

Download your Google Sheets API credentials JSON file and save it as `credentials.json`.

### 3. Build and Push Docker Image

Build the Docker image and push it to the local registry:

```bash
# Build the image
docker build -t localhost:32000/torn-oc-items:0.0.1 .

# Push to local registry
docker push localhost:32000/torn-oc-items:0.0.1
```

Note: Make sure your local registry at localhost:32000 is running and accessible.

### 4. Create the secret

Create the Kubernetes secret with your `.env` file and credentials:

```bash
# Base64 encode your .env file
ENV_CONTENT=$(cat .env | base64 -w 0)

# Base64 encode your credentials file
CREDS_CONTENT=$(cat credentials.json | base64 -w 0)

# Update the secret file
sed -i "s/your_base64_encoded_env_file_content_here/$ENV_CONTENT/" torn-secret.yaml
sed -i "s/your_base64_encoded_credentials_json_content_here/$CREDS_CONTENT/" torn-secret.yaml
```

### 5. Deploy

Apply the manifests:

```bash
kubectl apply -f torn-secret.yaml
kubectl apply -f deployment.yaml
```

## File Structure

- `env.template`: Template for environment variables
- `torn-secret.yaml`: Kubernetes secret containing .env file and credentials
- `deployment.yaml`: Kubernetes deployment manifest

## Security Notes

- The .env file and credentials are stored as Kubernetes secrets
- Files are mounted read-only into the container
- Container runs as non-root user (UID 1001)
- Security contexts prevent privilege escalation
