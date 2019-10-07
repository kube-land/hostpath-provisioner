package quota

import (

	"github.com/appspero/hostpath-provisioner/quota/common"
	"github.com/appspero/hostpath-provisioner/quota/extfs"
	"github.com/appspero/hostpath-provisioner/quota/xfs"

	"fmt"
	"bufio"
	"os"
	"regexp"
	"k8s.io/klog"

)

var mountsFile = "/proc/self/mounts"
var mountParseRegexp *regexp.Regexp = regexp.MustCompilePOSIX("^([^ ]*)[ \t]*([^ ]*)[ \t]*([^ ]*)") // Ignore options etc.

var providers = []common.LinuxVolumeQuotaProvider{
	&extfs.VolumeProvider{},
	&xfs.VolumeProvider{},
}

func GetQuotaApplier(mountpoint string) common.LinuxVolumeQuotaApplier {

	backingDev, err := detectBackingDevInternal(mountpoint, mountsFile)
	if err != nil {
		klog.Infof(err.Error())
		return nil
	}

	var applier common.LinuxVolumeQuotaApplier

	for _, provider := range providers {
	  if applier = provider.GetQuotaApplier(mountpoint, backingDev); applier != nil {
	    break
	  }
	}

	return applier

}

func detectBackingDevInternal(mountpoint string, mounts string) (string, error) {
	file, err := os.Open(mounts)
	if err != nil {
		return "", err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		match := mountParseRegexp.FindStringSubmatch(scanner.Text())
		if match != nil {
			device := match[1]
			mount := match[2]
			if mount == mountpoint {
				return device, nil
			}
		}
	}
	return "", fmt.Errorf("couldn't find backing device for %s", mountpoint)
}
