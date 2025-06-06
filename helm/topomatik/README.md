Topomatik automatically reflects your underlying infrastructure in Kubernetes node topology labels, because manually updating topology is about as fun as untangling holiday lights 🎄

Learn more about topology in Kubernetes:

- https://kubernetes.io/docs/reference/labels-annotations-taints/#topologykubernetesiozone
- https://kubernetes.io/docs/concepts/scheduling-eviction/topology-spread-constraints/

Learn more about this project on Github: https://github.com/enix/topomatik

## TL;DR:

```
helm install my-release oci://quay.io/enix/charts/topomatik
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

Topomatik is configured to enable the LLDP discovery engine by default and set the LLDP hostname as the `topology.kubernetes.io/zone` label.

More about its configuration can be found in the [project README](https://github.com/enix/topomatik?tab=readme-ov-file#configuration).

Default values are available [here](https://github.com/enix/topomatik/blob/main/helm/topomatik/values.yaml).
