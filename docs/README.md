# Documentation

Welcome to the latticelm documentation. This directory contains detailed guides and documentation for various aspects of the LLM Gateway.

## User Guides

### [Docker Deployment Guide](./DOCKER_DEPLOYMENT.md)
Complete guide to deploying the LLM Gateway using Docker with pre-built images or building from source.

**Topics covered:**
- Using pre-built container images from CI/CD
- Configuration with environment variables and config files
- Docker Compose setup with Redis and monitoring
- Production considerations (security, resources, networking)
- Multi-platform builds
- Troubleshooting and debugging

### [Admin Web UI](./ADMIN_UI.md)
Documentation for the built-in admin dashboard.

**Topics covered:**
- Accessing the Admin UI
- Features and capabilities
- System information dashboard
- Provider status monitoring
- Configuration management

## Developer Documentation

### [Admin UI Specification](./admin-ui-spec.md)
Technical specification and design document for the Admin UI component.

**Topics covered:**
- Component architecture
- API endpoints
- UI mockups and wireframes
- Implementation details

### [Implementation Summary](./IMPLEMENTATION_SUMMARY.md)
Overview of the implementation details and architecture decisions.

**Topics covered:**
- System architecture
- Provider implementations
- Key features and their implementations
- Technology stack

## Additional Resources

## Deployment Guides

### [Kubernetes Deployment Guide](../k8s/README.md)
Production-grade Kubernetes deployment with high availability, monitoring, and security.

**Topics covered:**
- Deploying with Kustomize and kubectl
- Secrets management (External Secrets Operator, Sealed Secrets)
- Monitoring with Prometheus and OpenTelemetry
- Horizontal Pod Autoscaling and PodDisruptionBudgets
- Security best practices (RBAC, NetworkPolicies, Pod Security)
- Cloud-specific guides (AWS EKS, GCP GKE, Azure AKS)
- Rolling updates and rollback strategies

For more documentation, see:

- **[Main README](../README.md)** - Overview, quick start, and feature documentation
- **[Configuration Example](../config.example.yaml)** - Detailed configuration options with comments

## Need Help?

- **Issues**: Check the [GitHub Issues](https://github.com/yourusername/latticelm/issues)
- **Discussions**: Use [GitHub Discussions](https://github.com/yourusername/latticelm/discussions) for questions
- **Contributing**: See [Contributing Guidelines](../README.md#contributing) in the main README
