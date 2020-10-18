![containerd banner](https://raw.githubusercontent.com/cncf/artwork/master/projects/containerd/horizontal/color/containerd-horizontal-color.png)

[![GoDoc](https://godoc.org/github.com/containerd/containerd?status.svg)](https://godoc.org/github.com/containerd/containerd)
[![Build Status](https://travis-ci.org/containerd/containerd.svg?branch=master)](https://travis-ci.org/containerd/containerd)
[![Windows Build Status](https://ci.appveyor.com/api/projects/status/github/containerd/containerd?branch=master&svg=true)](https://ci.appveyor.com/project/mlaventure/containerd-3g73f?branch=master)
[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bhttps%3A%2F%2Fgithub.com%2Fcontainerd%2Fcontainerd.svg?type=shield)](https://app.fossa.io/projects/git%2Bhttps%3A%2F%2Fgithub.com%2Fcontainerd%2Fcontainerd?ref=badge_shield)
[![Go Report Card](https://goreportcard.com/badge/github.com/containerd/containerd)](https://goreportcard.com/report/github.com/containerd/containerd)
[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/1271/badge)](https://bestpractices.coreinfrastructure.org/projects/1271)

containerd is an industry-standard container runtime with an emphasis on simplicity, robustness and portability. It is available as a daemon for Linux and Windows, which can manage the complete container lifecycle of its host system: image transfer and storage, container execution and supervision, low-level storage and network attachments, etc.

containerd is designed to be embedded into a larger system, rather than being used directly by developers or end-users.

![architecture](design/architecture.png)

## Evaluation on Container Rootfs on top of SPDK vhost

This section will give a guide on how to run SPDK vhost-target to provide container rootfs to Kata containers by containerd ctr cmd.

Currently, the POC demo can only do very limited operations, like: pull several container images into SPDK vhost, and then run some containers with SPDK serving container rootfs.

### Required Specific Softwares

* Kata Containers POC version
* Containerd POC version
* SPDK formal release like v20.0
* or SPDK Interrupt Mode POC version for further exploration

>  **Note: SPDK Interrupt Mode POC version is a minimal set of SPDK vhost target. It runs in interrupt mode and non-hugepage mode; vhost-blk device is its frontend, and Linux AIO block device is its backend.**

### Evaluation Enviroment Setup

* Open 5 SSH Terminals from A to E for operating

* Prepare SPDK on TermA
  
  - Download POC version SPDK
  
  ```bash
  go get github.com/spdk/spdk
  cd $GOPATH/src/github.com/spdk/spdk
  ```
  
  - Configure and compile it
  
  ```bash
  ./configure && make -j18
  ```
  
  - SPDK start and device assignment
  
  ```bash
  # Environment Prepare
  HUGEMEM=4096 PCI_WHITELIST="none" scripts/setup.sh
  mkdir -p /var/run/kata-containers/vhost-user/block/sockets/
  mkdir -p /var/run/kata-containers/vhost-user/block/devices/
  rm -f /var/run/kata-containers/vhost-user/block/devices
  ./app/vhost/vhost -S /var/run/kata-containers/vhost-user/block/sockets/ &
  ```
  
  - >  **Or SPDK further exploration with interrupt mode and non-hugepage**
 
  ```bash
  go get github.com/spdk/spdk
  cd $GOPATH/src/github.com/spdk/spdk

  # Pull and checkout poc branch from https://review.spdk.io/gerrit/c/spdk/spdk/+/4584
  # Replace <latest> with its newest version from the patch link
  git fetch "https://review.spdk.io/gerrit/spdk/spdk" refs/changes/84/4584/<latest> && git checkout FETCH_HEAD
  ./configure && make -j18
  ```
  
  ```bash
  # Environment Prepare
  # hugepage is not required yet.
  mkdir -p /var/run/kata-containers/vhost-user/block/sockets/
  mkdir -p /var/run/kata-containers/vhost-user/block/devices/
  rm -f /var/run/kata-containers/vhost-user/block/devices
  ./app/vhost/vhost -E -s 2048 -S /var/run/kata-containers/vhost-user/block/sockets/ &
  ```
  
  ```bash
  #Use nvme1n1 as the pool device
  dd if=/dev/zero of=/dev/nvme1n1 count=10 bs=4k
  ./scripts/rpc.py  "bdev_aio_create" /dev/nvme1n1 devpool
  ./scripts/rpc.py bdev_lvol_create_lvstore devpool devpool
  ```

* Prepare Kata-container on TermB
  
  - Download POC version Kata-runtime
  
  ```bash
  go get github.com/kata-containers/runtime
  
  #pull and checkout poc branch from:
  #repo: https://github.com/dong-liuliu/runtime
  #branch: xliu2/vhost-rootfs
  cd $GOPATH/src/github.com/kata-containers/runtime/
  git remote add rootfs-poc https://github.com/dong-liuliu/runtime
  git pull rootfs-poc
  git checkout rootfs-poc/xliu2/vhost-rootfs
  ```
  
  - Compile and Install
  
  ```bash
  make -j18 && make install
  ```
  
  - Vhost-user-blk enablement configure for Kata containers (/etc/kata-containers/configuration.toml)
  
  ```text
  #In kata configuration file, enable following options:
  enable_vhost_user_store = true
  enable_hugepages = true
  ```
  
  - >  **SPDK further exploratin configure for kata containers (/etc/kata-containers/configuration.toml)**

```text
  #In kata configuration file, enable following options:
  enable_vhost_user_store = true
  file_mem_backend = "/dev/shm"
```

* Prepare POC version Containerd on TermC
  
  - Download POC version Containerd
  
  ```bash
  go get github.com/containerd/containerd
  
  #pull and checkout poc branch from:
  #repo: https://github.com/dong-liuliu/containerd
  #branch: xliu2/spdk-rootfs
  cd $GOPATH/src/github.com/containerd/containerd
  git remote add rootfs-poc https://github.com/dong-liuliu/containerd
  git pull rootfs-poc
  git checkout rootfs-poc/xliu2/spdk-rootfs
  ```
  
  - Compile and install
  
  ```bash
  make -j18 && make install
  ```
  
  - SPDK vhost configure in containerd config (/etc/containerd/config.toml)
  
  ```text
  [plugins]
        [plugins.devmapper]
    pool_name = "devpool"
    root_path = "/var/lib/containerd/devmapper"
    base_image_size = "10GB"
    block_provider = "spdkvhost"
    pool_path = ""
       [plugins.linux]
                runtime = "kata"
  ```
  
  - Run containerd
  
  ```bash
  modprobe nbd
  mkdir -p /var/lib/containerd/devmapper
  service containerd start

  # Check and confirm devmapper is enabled
  # Currently, spdkvhost snapshotter is one option for devmapper
  ctr plugins ls
  ```

### Demo details

- Pull Images Demo
  
  - Download 2 short container images on TermD
  
  ```bash
  ctr images pull --snapshotter devmapper  docker.io/library/busybox:latest
  ctr images pull --snapshotter devmapper  docker.io/library/hello-world:latest  
  ```
  
  - Check SPDK’s Bdev status on TermA
  
  ```bash
  ./scripts/rpc.py  "bdev_get_bdevs " 
  # Several snapshot/clone bdevs have been created for images
  ```

- Run Containers Demo 
  
  - Run busybox container on TermD
  
  ```bash
  ctr run --rm -t  --runtime "io.containerd.kata.v2" --snapshotter devmapper  docker.io/library/busybox:latest test1
  
  # directly input some cmds on the started container
  hostname  #kata-container is returned
  cat /proc/vmstat
  ```
  
  - Run Hello-world container on TermE
  
  ```bash
  ctr run  --rm -t  --runtime "io.containerd.kata.v2" --snapshotter devmapper docker.io/library/hello-world:latest test2
  # Messages returned should be: Hello from Docker! ….
  ```
  
  - Check SPDK’s vhost controller status on TermA
  
  ```bash
  ./scripts/rpc.py "vhost_get_controllers"
  # There are related vhost-blk devices generated for kata container to use as rootfs
  ```






## Getting Started

See our documentation on [containerd.io](https://containerd.io):
* [for ops and admins](docs/ops.md)
* [namespaces](docs/namespaces.md)
* [client options](docs/client-opts.md)

See how to build containerd from source at [BUILDING](BUILDING.md).

If you are interested in trying out containerd see our example at [Getting Started](docs/getting-started.md).


## Runtime Requirements

Runtime requirements for containerd are very minimal. Most interactions with
the Linux and Windows container feature sets are handled via [runc](https://github.com/opencontainers/runc) and/or
OS-specific libraries (e.g. [hcsshim](https://github.com/Microsoft/hcsshim) for Microsoft). The current required version of `runc` is always listed in [RUNC.md](/RUNC.md).

There are specific features
used by containerd core code and snapshotters that will require a minimum kernel
version on Linux. With the understood caveat of distro kernel versioning, a
reasonable starting point for Linux is a minimum 4.x kernel version.

The overlay filesystem snapshotter, used by default, uses features that were
finalized in the 4.x kernel series. If you choose to use btrfs, there may
be more flexibility in kernel version (minimum recommended is 3.18), but will
require the btrfs kernel module and btrfs tools to be installed on your Linux
distribution.

To use Linux checkpoint and restore features, you will need `criu` installed on
your system. See more details in [Checkpoint and Restore](#checkpoint-and-restore).

Build requirements for developers are listed in [BUILDING](BUILDING.md).

## Features

### Client

containerd offers a full client package to help you integrate containerd into your platform.

```go

import (
  "github.com/containerd/containerd"
  "github.com/containerd/containerd/cio"
)


func main() {
	client, err := containerd.New("/run/containerd/containerd.sock")
	defer client.Close()
}

```

### Namespaces

Namespaces allow multiple consumers to use the same containerd without conflicting with each other.  It has the benefit of sharing content but still having separation with containers and images.

To set a namespace for requests to the API:

```go
context = context.Background()
// create a context for docker
docker = namespaces.WithNamespace(context, "docker")

containerd, err := client.NewContainer(docker, "id")
```

To set a default namespace on the client:

```go
client, err := containerd.New(address, containerd.WithDefaultNamespace("docker"))
```

### Distribution

```go
// pull an image
image, err := client.Pull(context, "docker.io/library/redis:latest")

// push an image
err := client.Push(context, "docker.io/library/redis:latest", image.Target())
```

### Containers

In containerd, a container is a metadata object.  Resources such as an OCI runtime specification, image, root filesystem, and other metadata can be attached to a container.

```go
redis, err := client.NewContainer(context, "redis-master")
defer redis.Delete(context)
```

### OCI Runtime Specification

containerd fully supports the OCI runtime specification for running containers.  We have built in functions to help you generate runtime specifications based on images as well as custom parameters.

You can specify options when creating a container about how to modify the specification.

```go
redis, err := client.NewContainer(context, "redis-master", containerd.WithNewSpec(oci.WithImageConfig(image)))
```

### Root Filesystems

containerd allows you to use overlay or snapshot filesystems with your containers.  It comes with builtin support for overlayfs and btrfs.

```go
// pull an image and unpack it into the configured snapshotter
image, err := client.Pull(context, "docker.io/library/redis:latest", containerd.WithPullUnpack)

// allocate a new RW root filesystem for a container based on the image
redis, err := client.NewContainer(context, "redis-master",
	containerd.WithNewSnapshot("redis-rootfs", image),
	containerd.WithNewSpec(oci.WithImageConfig(image)),
)

// use a readonly filesystem with multiple containers
for i := 0; i < 10; i++ {
	id := fmt.Sprintf("id-%s", i)
	container, err := client.NewContainer(ctx, id,
		containerd.WithNewSnapshotView(id, image),
		containerd.WithNewSpec(oci.WithImageConfig(image)),
	)
}
```

### Tasks

Taking a container object and turning it into a runnable process on a system is done by creating a new `Task` from the container.  A task represents the runnable object within containerd.

```go
// create a new task
task, err := redis.NewTask(context, cio.Stdio)
defer task.Delete(context)

// the task is now running and has a pid that can be use to setup networking
// or other runtime settings outside of containerd
pid := task.Pid()

// start the redis-server process inside the container
err := task.Start(context)

// wait for the task to exit and get the exit status
status, err := task.Wait(context)
```

### Checkpoint and Restore

If you have [criu](https://criu.org/Main_Page) installed on your machine you can checkpoint and restore containers and their tasks.  This allow you to clone and/or live migrate containers to other machines.

```go
// checkpoint the task then push it to a registry
checkpoint, err := task.Checkpoint(context)

err := client.Push(context, "myregistry/checkpoints/redis:master", checkpoint)

// on a new machine pull the checkpoint and restore the redis container
checkpoint, err := client.Pull(context, "myregistry/checkpoints/redis:master")

redis, err = client.NewContainer(context, "redis-master", containerd.WithNewSnapshot("redis-rootfs", checkpoint))
defer container.Delete(context)

task, err = redis.NewTask(context, cio.Stdio, containerd.WithTaskCheckpoint(checkpoint))
defer task.Delete(context)

err := task.Start(context)
```

### Snapshot Plugins

In addition to the built-in Snapshot plugins in containerd, additional external
plugins can be configured using GRPC. An external plugin is made available using
the configured name and appears as a plugin alongside the built-in ones.

To add an external snapshot plugin, add the plugin to containerd's config file
(by default at `/etc/containerd/config.toml`). The string following
`proxy_plugin.` will be used as the name of the snapshotter and the address
should refer to a socket with a GRPC listener serving containerd's Snapshot
GRPC API. Remember to restart containerd for any configuration changes to take
effect.

```
[proxy_plugins]
  [proxy_plugins.customsnapshot]
    type = "snapshot"
    address =  "/var/run/mysnapshotter.sock"
```

See [PLUGINS.md](PLUGINS.md) for how to create plugins

### Releases and API Stability

Please see [RELEASES.md](RELEASES.md) for details on versioning and stability
of containerd components.

### Communication

For async communication and long running discussions please use issues and pull requests on the github repo.
This will be the best place to discuss design and implementation.

For sync communication we have a community slack with a #containerd channel that everyone is welcome to join and chat about development.

**Slack:** Catch us in the #containerd and #containerd-dev channels on dockercommunity.slack.com.
[Click here for an invite to docker community slack.](https://dockr.ly/slack)

### Security audit

A third party security audit was performed by Cure53 in 4Q2018; the [full report](docs/SECURITY_AUDIT.pdf) is available in our docs/ directory.

### Reporting security issues

__If you are reporting a security issue, please reach out discreetly at security@containerd.io__.

## Licenses

The containerd codebase is released under the [Apache 2.0 license](LICENSE.code).
The README.md file, and files in the "docs" folder are licensed under the
Creative Commons Attribution 4.0 International License. You may obtain a
copy of the license, titled CC-BY-4.0, at http://creativecommons.org/licenses/by/4.0/.

## Project details

**containerd** is the primary open source project within the broader containerd GitHub repository.
However, all projects within the repo have common maintainership, governance, and contributing
guidelines which are stored in a `project` repository commonly for all containerd projects.

Please find all these core project documents, including the:
 * [Project governance](https://github.com/containerd/project/blob/master/GOVERNANCE.md),
 * [Maintainers](https://github.com/containerd/project/blob/master/MAINTAINERS),
 * and [Contributing guidelines](https://github.com/containerd/project/blob/master/CONTRIBUTING.md)

information in our [`containerd/project`](https://github.com/containerd/project) repository.

## Adoption

Interested to see who is using containerd? Are you using containerd in a project?
Please add yourself via pull request to our [ADOPTERS.md](./ADOPTERS.md) file.
