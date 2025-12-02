# Device Config Manager (DCM)

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Version](https://img.shields.io/badge/Go-1.23.4+-00ADD8.svg)](https://golang.org/)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-v1.24+-326CE5.svg)](https://kubernetes.io/)
[![Ubuntu](https://img.shields.io/badge/Ubuntu-22.04%20|%2024.04-E95420.svg)](https://ubuntu.com/)
[![RHEL](https://img.shields.io/badge/RHEL-9-EE0000.svg)](https://www.redhat.com/en/technologies/linux-platforms/enterprise-linux)

Device Config Manager (DCM) is a unified component that handles both AMD GPU and AINIC (AI NIC) device configurations. Originally designed for GPU partitioning in the GPU Operator, DCM has evolved into a comprehensive device configuration solution that supports:

- **GPU Configuration**: GPU partitioning and hardware management
- **AINIC Configuration**: AI network card profile and performance management  
- **Unified Architecture**: Single binary and container supporting both device types
- **Flexible Deployment**: Kubernetes and standalone/Debian deployment modes

## Key Features

- **Unified Binary**: One executable handles both GPU and AINIC workloads automatically
- **Auto-Detection**: Automatically discovers and configures available devices
- **Concurrent Management**: Can manage GPU and AINIC devices simultaneously
- **Environment Aware**: Adapts behavior based on Kubernetes vs Debian deployment
- **Profile-Based**: Uses ConfigMaps (K8s) or JSON files (Debian) for configuration

## Supported Platforms
- Ubuntu 22.04, 24.04
- RHEL 9 (Kubernetes/OpenShift)

## Device Support
- **GPU**: AMD GPUs with ROCM 6.3, 6.4, 7.0
- **AINIC**: AMD AI network cards with SR-IOV and RoCE capabilities

## Quick Start

### Build Unified Binary
```bash
# For Kubernetes deployment
make dcm-binary

# For Debian/standalone deployment  
make dcm-binary ENV=debian
```

### Build Docker Image
```bash
make dcm-docker
```

### Available Targets
- `dcm-binary` - Build unified DCM binary (ENV: k8s|debian)
- `dcm-docker` - Build Docker image with unified binary
- `pkg` - Create Debian package
- Legacy targets: `dcm`, `dcm-st` for backward compatibility

## Architecture

DCM uses a unified architecture where a single binary automatically:

1. **Environment Detection**: Determines if running in Kubernetes or standalone mode
2. **Device Discovery**: Scans for available GPU and AINIC configurations
3. **Concurrent Management**: Initializes appropriate managers for detected devices
4. **Configuration Monitoring**: Watches for configuration changes and applies updates

### Deployment Modes

| Mode | Environment | GPU Support | AINIC Support | Configuration Source |
|------|-------------|-------------|---------------|---------------------|
| Kubernetes | K8s/OpenShift | ✅ Full | ✅ Full | ConfigMaps + Node Labels |
| Debian | Standalone | ❌ Not Yet | ✅ Full | JSON Files |

## Configuration

DCM supports two configuration sources:
- **Kubernetes**: ConfigMaps with node-based profile selection via labels
- **Debian**: Direct JSON configuration files

For detailed AINIC configuration examples and advanced usage, see [README_AINIC.md](README_AINIC.md).

## Documentation

For detailed documentation including installation guides, configuration options, and partition descriptions, see the [documentation](https://instinct.docs.amd.com/projects/gpu-operator/en/latest/dcm/device-config-manager.html).

## License

This project is licensed under the Apache 2.0 License - see the [LICENSE](LICENSE) file for details.