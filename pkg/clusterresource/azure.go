package clusterresource

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	hivev1azure "github.com/openshift/hive/pkg/apis/hive/v1/azure"

	installertypes "github.com/openshift/installer/pkg/types"
	azureinstallertypes "github.com/openshift/installer/pkg/types/azure"
)

const (
	azureCredFile     = "osServicePrincipal.json"
	azureRegion       = "centralus"
	azureInstanceType = "Standard_D2s_v3"
)

var _ CloudBuilder = (*AzureCloudBuilder)(nil)

// AzureCloudBuilder encapsulates cluster artifact generation logic specific to Azure.
type AzureCloudBuilder struct {
	// ServicePrincipal is the bytes from a service principal file, typically ~/.azure/osServicePrincipal.json.
	ServicePrincipal []byte

	// BaseDomainResourceGroupName is the resource group where the base domain for this cluster is configured.
	BaseDomainResourceGroupName string
}

func (p *AzureCloudBuilder) generateCredentialsSecret(o *Builder) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      p.credsSecretName(o),
			Namespace: o.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			azureCredFile: p.ServicePrincipal,
		},
	}
}

func (p *AzureCloudBuilder) addClusterDeploymentPlatform(o *Builder, cd *hivev1.ClusterDeployment) {
	cd.Spec.Platform = hivev1.Platform{
		Azure: &hivev1azure.Platform{
			CredentialsSecretRef: corev1.LocalObjectReference{
				Name: p.credsSecretName(o),
			},
			Region:                      azureRegion,
			BaseDomainResourceGroupName: p.BaseDomainResourceGroupName,
		},
	}
}

func (p *AzureCloudBuilder) addMachinePoolPlatform(o *Builder, mp *hivev1.MachinePool) {
	mp.Spec.Platform.Azure = &hivev1azure.MachinePool{
		InstanceType: azureInstanceType,
		OSDisk: hivev1azure.OSDisk{
			DiskSizeGB: 128,
		},
	}

}

func (p *AzureCloudBuilder) addInstallConfigPlatform(o *Builder, ic *installertypes.InstallConfig) {
	// Inject platform details into InstallConfig:
	ic.Platform = installertypes.Platform{
		Azure: &azureinstallertypes.Platform{
			Region:                      azureRegion,
			BaseDomainResourceGroupName: p.BaseDomainResourceGroupName,
		},
	}

	// Used for both control plane and workers.
	mpp := &azureinstallertypes.MachinePool{}
	ic.ControlPlane.Platform.Azure = mpp
	ic.Compute[0].Platform.Azure = mpp
}

func (p *AzureCloudBuilder) credsSecretName(o *Builder) string {
	return fmt.Sprintf("%s-azure-creds", o.Name)
}
