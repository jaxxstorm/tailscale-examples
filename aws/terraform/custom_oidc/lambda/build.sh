#!/bin/bash
set -e

echo "Building Lambda function..."

# Navigate to lambda directory
cd lambda

# Build the Go binary
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bootstrap main.go

echo "✅ Lambda binary built successfully at lambda/bootstrap"
echo "Run 'terraform apply' to deploy"
