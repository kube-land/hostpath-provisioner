package main

import (
	"fmt"
	"errors"
	"flag"
	"os"
	"path"
	"syscall"

	"sigs.k8s.io/sig-storage-lib-external-provisioner/controller"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog"

	"github.com/appwavelets/hostpath-provisioner/quota/common"
	"github.com/appwavelets/hostpath-provisioner/quota"
)

const (
	provisionerName = "appwavelets.com/hostpath"
)

type hostPathProvisioner struct {
	// The directory to create PV-backing directories in
	pvDir string

	// Identity of this hostPathProvisioner, set to node's name. Used to identify
	// "this" provisioner's PVs.
	identity string

	quotaApplier common.LinuxVolumeQuotaApplier
}

// NewHostPathProvisioner creates a new hostpath provisioner
func NewHostPathProvisioner(pvDir string) controller.Provisioner {
	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		klog.Fatal("env variable NODE_NAME must be set so that this provisioner can identify itself")
	}
	return &hostPathProvisioner{
		pvDir:    pvDir,
		identity: nodeName,
		quotaApplier: quota.GetQuotaApplier(pvDir),
	}
}

var _ controller.Provisioner = &hostPathProvisioner{}

// Provision creates a storage asset and returns a PV object representing it.
func (p *hostPathProvisioner) Provision(options controller.VolumeOptions) (*v1.PersistentVolume, error) {

	selectedNode := options.SelectedNode
	nodeValue := selectedNode.Labels[v1.LabelHostname]

	if nodeValue != p.identity {
		return nil, &controller.IgnoredError{Reason: "node does not match persistent volume selected node"}
	}

	path := path.Join(p.pvDir, options.PVName)

	if err := os.MkdirAll(path, 0777); err != nil {
		return nil, err
	}

	size := options.PVC.Spec.Resources.Requests[v1.ResourceStorage]

	if p.quotaApplier != nil {
		quotaID, err := p.quotaApplier.FindAvailableQuota(path)
		if err == nil {
			err = p.quotaApplier.SetQuotaOnDir(path, quotaID, size.Value())
			if err != nil {
				fmt.Println(err)
			}
		} else {
			fmt.Println(err)
		}
	}

	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: options.PVName,
			Annotations: map[string]string{
				"hostPathProvisionerIdentity": p.identity,
			},
		},
		Spec: v1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: options.PersistentVolumeReclaimPolicy,
			AccessModes:                   options.PVC.Spec.AccessModes,
			Capacity: v1.ResourceList{
				v1.ResourceName(v1.ResourceStorage): options.PVC.Spec.Resources.Requests[v1.ResourceName(v1.ResourceStorage)],
			},
			PersistentVolumeSource: v1.PersistentVolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: path,
				},
			},
			NodeAffinity: &v1.VolumeNodeAffinity{
				Required: &v1.NodeSelector{
					NodeSelectorTerms: []v1.NodeSelectorTerm{
						{
							MatchExpressions: []v1.NodeSelectorRequirement{
								{
									Key:      v1.LabelHostname,
									Operator: v1.NodeSelectorOpIn,
									Values:   []string{nodeValue},
								},
							},
						},
					},
				},
			},
		},
	}

	return pv, nil
}

// Delete removes the storage asset that was created by Provision represented
// by the given PV.
func (p *hostPathProvisioner) Delete(volume *v1.PersistentVolume) error {
	ann, ok := volume.Annotations["hostPathProvisionerIdentity"]
	if !ok {
		return errors.New("identity annotation not found on PV")
	}
	if ann != p.identity {
		return &controller.IgnoredError{Reason: "identity annotation on PV does not match ours"}
	}

	path := path.Join(p.pvDir, volume.Name)

	if p.quotaApplier != nil {
		p.quotaApplier.ClearQuotaOnDir(path)
	}

	if err := os.RemoveAll(path); err != nil {
		return err
	}

	return nil
}

func main() {
	syscall.Umask(0)

	var pvDir string
  flag.StringVar(&pvDir, "pv-directory", "/tmp/hostpath-provisioner", "host directory where the `hostpath-provisioner` will create the persistent volumes")

	klog.InitFlags(nil)

	flag.Set("logtostderr", "true")
	flag.Parse()

	// Create an InClusterConfig and use it to create a client for the controller
	// to use to communicate with Kubernetes
	config, err := rest.InClusterConfig()
	if err != nil {
		klog.Fatalf("Failed to create config: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatalf("Failed to create client: %v", err)
	}

	// The controller needs to know what the server version is because out-of-tree
	// provisioners aren't officially supported until 1.5
	serverVersion, err := clientset.Discovery().ServerVersion()
	if err != nil {
		klog.Fatalf("Error getting server version: %v", err)
	}

	// Create the provisioner: it implements the Provisioner interface expected by
	// the controller
	hostPathProvisioner := NewHostPathProvisioner(pvDir)

	// Start the provision controller which will dynamically provision hostPath
	// PVs
	pc := controller.NewProvisionController(clientset, provisionerName, hostPathProvisioner, serverVersion.GitVersion, controller.LeaderElection(false))
	pc.Run(wait.NeverStop)
}
