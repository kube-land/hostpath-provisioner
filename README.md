## hostpath-provisioner

## Install

To install `hostpath-provisioner` controller:

```bash
kubectl apply -f install/.
```

The previous command will install the controller and it RBAC. Further it will install a storage class `hostpath` with `reclaimPolicy: Delete`.

Example of usage:

```yaml
kind: PersistentVolumeClaim
apiVersion: v1
metadata:
  name: hostpath-pvc
spec:
  storageClassName: "hostpath"
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
```

## Config

You can configure the host directory where the `hostpath-provisioner` will create the persistent volumes using the flag:

```
-pv-directory=/path/to/directory
```

## Build

To build `hostpath-provisioner`:

```bash
CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o hostpath-provisioner  .
```

To build the docker image:

```bash
docker build -t abdullahalmariah/hostpath-provisioner:latest -f Dockerfile.scratch .
```
