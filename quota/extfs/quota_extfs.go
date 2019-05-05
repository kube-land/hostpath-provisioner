package extfs

/*
#include <stdlib.h>
#include <dirent.h>
#include <linux/fs.h>
#include <linux/quota.h>
#include <errno.h>
#ifndef PRJQUOTA
#define PRJQUOTA	2
#endif
#ifndef Q_SETPQUOTA
#define Q_SETPQUOTA (unsigned) QCMD(Q_SETQUOTA, PRJQUOTA)
#endif
#ifndef Q_GETPQUOTA
#define Q_GETPQUOTA (unsigned) QCMD(Q_GETQUOTA, PRJQUOTA)
#endif
*/
import "C"

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
	"k8s.io/klog"
	"github.com/appwavelets/hostpath-provisioner/quota/common"
)

//  ext4fs empirically has a maximum quota size of 2^48 - 1 1KiB blocks (256 petabytes)
const (
	linuxExtfsMagic       = 0xef53
	quotaBsize            = 1024                                   // extfs specific
	bitsPerWord           = 32 << (^uint(0) >> 63)                 // either 32 or 64
	maxQuota        int64 = (1<<(bitsPerWord-1) - 1) & (1<<58 - 1) // either 1<<31 - 1 or 1<<58 - 1
)

// VolumeProvider supplies a quota applier to the generic code.
type VolumeProvider struct {
}

// GetQuotaApplier -- does this backing device support quotas that
// can be applied to directories?
func (*VolumeProvider) GetQuotaApplier(mountpoint string, backingDev string) common.LinuxVolumeQuotaApplier {
	if common.IsFilesystemOfType(mountpoint, backingDev, linuxExtfsMagic) {
		return extfsVolumeQuota{backingDev}
	}
	return nil
}

type extfsVolumeQuota struct {
	backingDev string
}

// GetQuotaOnDir -- get the quota ID that applies to this directory.

func (v extfsVolumeQuota) GetQuotaOnDir(path string) (common.QuotaID, error) {
	return common.GetQuotaOnDir(path)
}

// SetQuotaOnDir -- apply the specified quota to the directory.  If
// bytes is not greater than zero, the quota should be applied in a
// way that is non-enforcing (either explicitly so or by setting a
// quota larger than anything the user may possibly create)
func (v extfsVolumeQuota) SetQuotaOnDir(path string, id common.QuotaID, bytes int64) error {
	klog.V(3).Infof("extfsSetQuotaOn %s ID %v bytes %v", path, id, bytes)
	if bytes < 0 || bytes > maxQuota {
		bytes = maxQuota
	}

	var d C.struct_if_dqblk

	d.dqb_bhardlimit = C.__u64(bytes / quotaBsize)
	d.dqb_bsoftlimit = d.dqb_bhardlimit
	d.dqb_ihardlimit = 0
	d.dqb_isoftlimit = 0
	d.dqb_valid = C.QIF_LIMITS

	var cs = C.CString(v.backingDev)
	defer C.free(unsafe.Pointer(cs))

	_, _, errno := unix.Syscall6(unix.SYS_QUOTACTL, C.Q_SETPQUOTA,
		uintptr(unsafe.Pointer(cs)), uintptr(id),
		uintptr(unsafe.Pointer(&d)), 0, 0)
	if errno != 0 {
		return fmt.Errorf("Failed to set quota limit for ID %d on %s: %v",
			id, path, errno.Error())
	}
	return common.ApplyProjectToDir(path, id)
}

func (v extfsVolumeQuota) getQuotaInfo(path string, id common.QuotaID) (C.struct_if_dqblk, syscall.Errno) {
	var d C.struct_if_dqblk

	var cs = C.CString(v.backingDev)
	defer C.free(unsafe.Pointer(cs))

	_, _, errno := unix.Syscall6(unix.SYS_QUOTACTL, C.Q_GETPQUOTA,
		uintptr(unsafe.Pointer(cs)), uintptr(C.__u32(id)),
		uintptr(unsafe.Pointer(&d)), 0, 0)
	return d, errno
}

// QuotaIDIsInUse -- determine whether the quota ID is already in use.
func (v extfsVolumeQuota) QuotaIDIsInUse(path string, id common.QuotaID) (bool, error) {
	d, errno := v.getQuotaInfo(path, id)
	isInUse := !(d.dqb_bhardlimit == 0 && d.dqb_bsoftlimit == 0 && d.dqb_curspace == 0 &&
		d.dqb_ihardlimit == 0 && d.dqb_isoftlimit == 0 && d.dqb_curinodes == 0 &&
		d.dqb_btime == 0 && d.dqb_itime == 0)
	return errno == 0 && isInUse, nil
}

// GetConsumption -- retrieve the consumption (in bytes) of the directory
// Note that with ext[[:digit:]]fs the quota consumption is in bytes
// per man quotactl
func (v extfsVolumeQuota) GetConsumption(path string, id common.QuotaID) (int64, error) {
	d, errno := v.getQuotaInfo(path, id)
	if errno != 0 {
		return 0, fmt.Errorf("Failed to get quota for %s: %s", path, errno.Error())
	}
	klog.V(3).Infof("Consumption for %s is %v", path, d.dqb_curspace)
	return int64(d.dqb_curspace), nil
}

// GetInodes -- retrieve the number of inodes in use under the directory
func (v extfsVolumeQuota) GetInodes(path string, id common.QuotaID) (int64, error) {
	d, errno := v.getQuotaInfo(path, id)
	if errno != 0 {
		return 0, fmt.Errorf("Failed to get quota for %s: %s", path, errno.Error())
	}
	klog.V(3).Infof("Inode consumption for %s is %v", path, d.dqb_curinodes)
	return int64(d.dqb_curinodes), nil
}

func (v extfsVolumeQuota) ClearQuotaOnDir(path string) error {
	projid, err := v.GetQuotaOnDir(path)
	if err == nil && projid != common.BadQuotaID {
		klog.V(3).Infof("clearQuotaOnDir clearing quota")
		err = v.SetQuotaOnDir(path, projid, 0)
		if err != nil {
			klog.V(3).Infof("Attempt to clear quota failed: %v", err)
		}
		return nil
	}
	klog.V(3).Infof("clearQuotaOnDir fails %v", err)
	return nil
}

func (v extfsVolumeQuota) FindAvailableQuota(path string) (common.QuotaID, error) {
	unusedQuotasSearched := 0
	for id := common.FirstQuota; id == id; id++ {
		isInUse, err := v.QuotaIDIsInUse(path, id)
		if err != nil {
			return common.BadQuotaID, err
		} else if !isInUse {
			return id, nil
		}
		unusedQuotasSearched++
		if unusedQuotasSearched > common.MaxUnusedQuotasToSearch {
			break
		}
	}
	return common.BadQuotaID, fmt.Errorf("Cannot find available quota ID")
}
