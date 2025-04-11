# Topomatik

Topomatik automatically reflects your underlying infrastructure in Kubernetes node topology annotations, because manually updating topology is about as fun as untangling holiday lights. 🎄

Learn more about topology in Kubernetes:

* https://kubernetes.io/docs/reference/labels-annotations-taints/#topologykubernetesiozone
* https://kubernetes.io/docs/concepts/scheduling-eviction/topology-spread-constraints/

## ✨ Features

* Automagically updates the `topology.kubernetes.io/zone` and `topology.kubernetes.io/region` node annotations based on autodiscovered infrastructure
* Multiple auto-discovery engines. Currently only LLDP is supported (more coming soon™)
* Works with both virtualized and bare-metal nodes
* Runs as a DaemonSet; updates topology even when nodes are live-migrated

## 📦 Installation

## Configuration

## Auto-discovery engines

### LLDP

LLDP (Link Layer Discovery Protocol) is a vendor-neutral Layer 2 protocol that enables network devices to automatically discover and share information about their identity, capabilities, and neighboring devices on a local network.

It can be used in both bare-metal and virtualized environments to inform nodes about the underlying topology. In bare-metal environments, it must be enabled at the network device level (e.g., switches). In virtualized environments, you'll need to install the lldpd service on your hypervisors.

#### Proxmox PVE setup

Here is an example of installation of the `lldpd` service on your Proxmox PVEs.

## 🤝 Contributing

Found a bug? Want to add support for another discovery engine? We welcome contributions! Just be sure your code is as clean as your commit messages.

## 📜 License

Topomatik is open-source software licensed under MIT. Use it, modify it, love it!
