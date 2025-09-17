# CSM SharedNFS for Dell CSI drivers

[![Go Report Card](https://goreportcard.com/badge/github.com/dell/csm-sharednfs?style=flat-square)](https://goreportcard.com/report/github.com/dell/csm-sharednfs)
[![License](https://img.shields.io/github/license/dell/csm-sharednfs?style=flat-square&color=blue&label=License)](https://github.com/dell/csm-sharednfs/blob/master/LICENSE)
[![Last Release](https://img.shields.io/github/v/release/dell/csm-sharednfs?label=Latest&style=flat-square&logo=go)](https://github.com/dell/csm-sharednfs/releases)


## Description
The csm-sharednfs (Container Storage Modules - SharedNFS) is a component designed to export block volumes from a CSI driver via NFS. It enhances container orchestrator environments by providing NFS access to block storage volumes, allowing multiple pods to share storage efficiently.

## Features
- Converts block storage into NFS shares.
- Enables multiple pods to access the same volume.
- Integrates seamlessly with Dell Storage solutions.
- Supports dynamic volume provisioning. Optimized for Kubernetes environments.

## Usage
Once deployed, csm-sharednfs allows users to:
- Create and manage NFS shares backed by block storage. 
- Mount NFS volumes on multiple pods. 
- Ensure high availability and performance in storage workloads. 

Note: This project can be compiled with CSI Powerstore driver only. Other platforms are not yet supported at this time.

## Table of Contents

* [Code of Conduct](https://github.com/dell/csm/blob/main/docs/CODE_OF_CONDUCT.md)
* [Maintainer Guide](https://github.com/dell/csm/blob/main/docs/MAINTAINER_GUIDE.md)
* [Committer Guide](https://github.com/dell/csm/blob/main/docs/COMMITTER_GUIDE.md)
* [Contributing Guide](https://github.com/dell/csm/blob/main/docs/CONTRIBUTING.md)
* [List of Adopters](https://github.com/dell/csm/blob/main/docs/ADOPTERS.md)
* [Support](#support)
* [Security](https://github.com/dell/csm/blob/main/docs/SECURITY.md)
* [Building](#building)
* [Prerequisites](#prerequisites)
* [Documentation](#documentation)

## Support
For any issues, questions or feedback, please follow our [support process](https://github.com/dell/csm/blob/main/docs/SUPPORT.md)

## Building
This project is imported as a [Go module](https://go.dev/ref/mod) from within the CSI drivers.
The dependencies for this project are listed in the go.mod file.

To run unit tests, execute `make unit-test`.

## Prerequisites

NFS server services are required to be active and running on the nodes prior to using this module. 
Please refer to respective host platform documentation on installing nfs packages and how to enable nfs services. 

## Documentation
For more detailed information on the driver, please refer to [Container Storage Modules documentation](https://dell.github.io/csm-docs/).
