# hostpath-provisioner

The `hostpath-provisioner` will provision HostPath Persistent Volumes using Persistent Volumes Claims. It support setting size using storage requests. It uses disk project quota for both XFS and EXT4.

## Install

To install `hostpath-provisioner` controller:

```bash
kubectl apply -k install/
```

or by reference in your overlay as:

```yaml
# kustomization.yaml
bases:
  - github.com/appspero/hostpath-provisioner//install?ref=master
```

The previous command will install the controller and it RBAC.

### Enabling quota on non-root partition

To enable quota on the non-root partition edit the configuration file `/etc/fstab`. For example:

```
# /etc/fstab

/dev/mapper/centos-disk /mnt/disk               xfs     defaults,pquota        0 0
```

To check current quota:

```bash
xfs_quota -xc 'report -h' /mnt/disk
```

### Enabling quota on the `/` (root) partition

To enable quota on the `/` (root) partition edit the grub configuration file `/etc/default/grub`. Search for the line that starts with `GRUB_CMDLINE_LINUX` and add `rootflags=uquota,gquota` to the command line parameters. Then

```bash
cp /boot/grub2/grub.cfg /boot/grub2/grub.cfg_bak
grub2-mkconfig -o /boot/grub2/grub.cfg
reboot
```

## Configuration

You can configure the host directory where the `hostpath-provisioner` will create the persistent volumes using the flag:

```
-pv-directory=/path/to/directory
```

Then create a storage class with `provisioner: appspero.com/hostpath` (check examples).


**Warning:** The default value of `-pv-directory` is `/tmp/hostpath-provisioner`. Using `/tmp/...` directory could be risky for persisting volume; it will be auto-cleaned by the OS.

## Build

To build `hostpath-provisioner`:

```bash
docker build -t abdullahalmariah/hostpath-provisioner:latest .
docker push abdullahalmariah/hostpath-provisioner:latest
```
