# Topomatik

<p align="center">
    <a href="https://opensource.org/licenses/MIT" alt="MIT License">
        <img src="https://img.shields.io/badge/License-MIT-yellow.svg" /></a>
    <a href="https://enix.io/fr/blog/" alt="Brought to you by ENIX">
        <img src="https://img.shields.io/badge/Brought%20to%20you%20by-ENIX-%23377dff?labelColor=888&logo=data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAA4AAAAOCAQAAAC1QeVaAAAABGdBTUEAALGPC/xhBQAAACBjSFJNAAB6JgAAgIQAAPoAAACA6AAAdTAAAOpgAAA6mAAAF3CculE8AAAAAmJLR0QA/4ePzL8AAAAHdElNRQfkBAkQIg/iouK/AAABZ0lEQVQY0yXBPU8TYQDA8f/zcu1RSDltKliD0BKNECYZmpjgIAOLiYtubn4EJxI/AImzg3E1+AGcYDIMJA7lxQQQQRAiSSFG2l457+655x4Gfz8B45zwipWJ8rPCQ0g3+p9Pj+AlHxHjnLHAbvPW2+GmLoBN+9/+vNlfGeU2Auokd8Y+VeYk/zk6O2fP9fcO8hGpN/TUbxpiUhJiEorTgy+6hUlU5N1flK+9oIJHiKNCkb5wMyOFw3V9o+zN69o0Exg6ePh4/GKr6s0H72Tc67YsdXbZ5gENNjmigaXbMj0tzEWrZNtqigva5NxjhFP6Wfw1N1pjqpFaZQ7FAY6An6zxTzHs0BGqY/NQSnxSBD6WkDRTf3O0wG2Ztl/7jaQEnGNxZMdy2yET/B2xfGlDagQE1OgRRvL93UOHqhLnesPKqJ4NxLLn2unJgVka/HBpbiIARlHFq1n/cWlMZMne1ZfyD5M/Aa4BiyGSwP4Jl3UAAAAldEVYdGRhdGU6Y3JlYXRlADIwMjAtMDQtMDlUMTQ6MzQ6MTUrMDI6MDDBq8/nAAAAJXRFWHRkYXRlOm1vZGlmeQAyMDIwLTA0LTA5VDE0OjM0OjE1KzAyOjAwsPZ3WwAAAABJRU5ErkJggg==" /></a>
</p>

Topomatik automatically reflects your underlying infrastructure in Kubernetes node topology annotations, because manually updating topology is about as fun as untangling holiday lights 🎄

Learn more about topology in Kubernetes:

- https://kubernetes.io/docs/reference/labels-annotations-taints/#topologykubernetesiozone
- https://kubernetes.io/docs/concepts/scheduling-eviction/topology-spread-constraints/

## ✨ Features

- Automagically updates the `topology.kubernetes.io/zone` and `topology.kubernetes.io/region` node annotations based on autodiscovered infrastructure
- Multiple auto-discovery engines. Currently only LLDP is supported (more coming soon™)
- Works with both virtualized and bare-metal nodes
- Runs as a DaemonSet; updates topology even when nodes are live-migrated

## 📦 Installation

## TL;DR:

```
helm install my-release oci://quay.io/enix/charts/zfs-exporter
```

### Flux usage

When using [Flux](https://fluxcd.io), you can use this `HelmRepository` object as repository for all ENIX projects:

```
apiVersion: source.toolkit.fluxcd.io/v1
kind: HelmRepository
metadata:
  name: enix
  namespace: default
spec:
  interval: 24h
  type: oci
  url: oci://quay.io/enix/charts
```

Then, you can create a `HelmRelease` object pointing to this repository:

```
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: topomatik
  namespace: topomatik
spec:
  interval: 1m0s
  chart:
    spec:
      chart: topomatik
      sourceRef:
        kind: HelmRepository
        name: enix
        namespace: default
      version: 1.*
```

## Configuration

Topomatik is configured using a YAML file. Here's an example configuration:

```
annotationTemplates:
  topology.kubernetes.io/zone: "{{ .lldp.hostname }}"
  topology.kubernetes.io/region: "{{ .lldp.description | regexp.Find "location: [^ ]" }}

lldp:
  enabled: true
  interface: auto
```

### Annotation Templates

The `annotationTemplates` section defines which Kubernetes annotations Topomatik will manage. Each value is a Go template that will be rendered to determine the annotation value.

The [Sprig library](http://masterminds.github.io/sprig/) is available for advanced template operations, giving you access to string manipulation, regular expressions, and more.

### Auto-Discovery Engine Configuration

Each auto-discovery engine has its own configuration section.

All engines share a common enabled key that allows you to enable or disable the engine. The remaining configuration keys are specific to each engine.

Below you'll find detailed information about each supported engine

#### LLDP

LLDP (Link Layer Discovery Protocol) is a vendor-neutral Layer 2 protocol that enables network devices to automatically discover and share information about their identity, capabilities, and neighboring devices on a local network.

It can be used in both bare-metal and virtualized environments to inform nodes about the underlying topology (eg: Proxmox PVE). In bare-metal environments, it must be enabled at the network device level (e.g., switches). In virtualized environments, you'll need to install the lldpd service on your hypervisors.

##### Configuration

| Name      | Description                                                                                       | Default value |
| --------- | ------------------------------------------------------------------------------------------------- | ------------- |
| enabled   | Enable or disable this auto discovery engine                                                      | true          |
| interface | Interface name to use for listen to LLDP frames or auto to use any interface with default gateway | auto          |

##### Available template variables

| Name             | Description        |
| ---------------- | ------------------ |
| lldp.hostname    | The SysName TLV    |
| lldp.description | The SysDescription |

##### Proxmox PVE setup example

Topomatik can be used with Proxmox PVE using the lldpd. Just install it using `apt install lldpd`. The default configuration should be sufficient to advertise the TLV handled by Topomatik.

## 🤝 Contributing

Found a bug? Want to add support for another discovery engine? We welcome contributions! Just be sure your code is as clean as your commit messages.

## 📜 License

Topomatik is open-source software licensed under MIT. Use it, modify it, love it!
