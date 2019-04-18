// Copyright © 2019 Banzai Cloud
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package workflow

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2018-10-01/compute"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/goph/emperror"
	"go.uber.org/cadence/activity"
)

// CreateVMSSActivityName is the default registration name of the activity
const CreateVMSSActivityName = "pke-azure-create-vmss"

// CreateVMSSActivity represents an activity for creating an Azure virtual machine scale set
type CreateVMSSActivity struct {
	azureClientFactory *AzureClientFactory
}

// MakeCreateVMSSActivity returns a new CreateVMSSActivity
func MakeCreateVMSSActivity(azureClientFactory *AzureClientFactory) CreateVMSSActivity {
	return CreateVMSSActivity{
		azureClientFactory: azureClientFactory,
	}
}

// CreateVMSSActivityInput represents the input needed for executing a CreateVMSSActivity
type CreateVMSSActivityInput struct {
	OrganizationID    uint
	SecretID          string
	ClusterName       string
	ResourceGroupName string

	ScaleSet VirtualMachineScaleSet
}

// VirtualMachineScaleSet represents an Azure virtual machine scale set
type VirtualMachineScaleSet struct {
	AdminUsername          string
	Image                  Image
	InstanceCount          int64
	InstanceType           string
	LBBackendAddressPoolID string
	LBInboundNATPoolID     string
	Location               string
	Name                   string
	NetworkSecurityGroupID string
	SSHPublicKey           string
	SubnetID               string
	UserDataScript         string
	Zones                  []string
}

type Image struct {
	Offer     string
	Publisher string
	SKU       string
	Version   string
}

type CreateVMSSActivityOutput struct {
	PrincipalID string
}

// Execute performs the activity
func (a CreateVMSSActivity) Execute(ctx context.Context, input CreateVMSSActivityInput) (output CreateVMSSActivityOutput, err error) {
	logger := activity.GetLogger(ctx).Sugar().With(
		"organization", input.OrganizationID,
		"cluster", input.ClusterName,
		"secret", input.SecretID,
		"resourceGroup", input.ResourceGroupName,
		"vmssName", input.ScaleSet.Name,
	)

	keyvals := []interface{}{
		"resourceGroup", input.ResourceGroupName,
		"vmssName", input.ScaleSet.Name,
	}

	logger.Info("create virtual machine scale set")

	cc, err := a.azureClientFactory.New(input.OrganizationID, input.SecretID)
	if err = emperror.Wrap(err, "failed to create cloud connection"); err != nil {
		return
	}

	params := input.getCreateOrUpdateVirtualMachineScaleSetParams()

	client := cc.GetVirtualMachineScaleSetsClient()

	logger.Debug("sending request to create or update virtual machine scale set")

	future, err := client.CreateOrUpdate(ctx, input.ResourceGroupName, input.ScaleSet.Name, params)
	if err = emperror.WrapWith(err, "sending request to create or update virtual machine scale set failed", keyvals...); err != nil {
		return
	}

	logger.Debug("waiting for the completion of create or update virtual machine scale set operation")

	err = future.WaitForCompletionRef(ctx, client.Client)
	if err = emperror.WrapWith(err, "waiting for the completion of create or update virtual machine scale set operation failed", keyvals...); err != nil {
		return
	}

	vmss, err := future.Result(client.VirtualMachineScaleSetsClient)
	if err = emperror.WrapWith(err, "getting virtual machine scale set create or update result failed", keyvals...); err != nil {
		return
	}

	if vmss.Identity != nil {
		output.PrincipalID = to.String(vmss.Identity.PrincipalID)
	}

	return
}

func (input CreateVMSSActivityInput) getCreateOrUpdateVirtualMachineScaleSetParams() compute.VirtualMachineScaleSet {
	var bapRefs *[]compute.SubResource
	if input.ScaleSet.LBBackendAddressPoolID != "" {
		bapRefs = &[]compute.SubResource{
			{
				ID: to.StringPtr(input.ScaleSet.LBBackendAddressPoolID),
			},
		}
	}
	var inpRefs *[]compute.SubResource
	if input.ScaleSet.LBInboundNATPoolID != "" {
		inpRefs = &[]compute.SubResource{
			{
				ID: to.StringPtr(input.ScaleSet.LBInboundNATPoolID),
			},
		}
	}
	return compute.VirtualMachineScaleSet{
		Identity: &compute.VirtualMachineScaleSetIdentity{
			Type: compute.ResourceIdentityTypeSystemAssigned,
		},
		Location: to.StringPtr(input.ScaleSet.Location),
		Sku: &compute.Sku{
			Capacity: to.Int64Ptr(input.ScaleSet.InstanceCount),
			Name:     to.StringPtr(input.ScaleSet.InstanceType),
		},
		Tags: *to.StringMapPtr(tagsFrom(getOwnedTag(input.ClusterName))),
		VirtualMachineScaleSetProperties: &compute.VirtualMachineScaleSetProperties{
			VirtualMachineProfile: &compute.VirtualMachineScaleSetVMProfile{
				NetworkProfile: &compute.VirtualMachineScaleSetNetworkProfile{
					NetworkInterfaceConfigurations: &[]compute.VirtualMachineScaleSetNetworkConfiguration{
						{
							VirtualMachineScaleSetNetworkConfigurationProperties: &compute.VirtualMachineScaleSetNetworkConfigurationProperties{
								IPConfigurations: &[]compute.VirtualMachineScaleSetIPConfiguration{
									{
										VirtualMachineScaleSetIPConfigurationProperties: &compute.VirtualMachineScaleSetIPConfigurationProperties{
											LoadBalancerBackendAddressPools: bapRefs,
											LoadBalancerInboundNatPools:     inpRefs,
											Subnet: &compute.APIEntityReference{
												ID: to.StringPtr(input.ScaleSet.SubnetID),
											},
										},
									},
								},
								NetworkSecurityGroup: &compute.SubResource{
									ID: to.StringPtr(input.ScaleSet.NetworkSecurityGroupID),
								},
							},
						},
					},
				},
				OsProfile: &compute.VirtualMachineScaleSetOSProfile{
					AdminUsername: to.StringPtr(input.ScaleSet.AdminUsername),
					CustomData:    to.StringPtr(input.ScaleSet.UserDataScript),
					LinuxConfiguration: &compute.LinuxConfiguration{
						DisablePasswordAuthentication: to.BoolPtr(true),
						SSH: &compute.SSHConfiguration{
							PublicKeys: &[]compute.SSHPublicKey{
								{
									KeyData: to.StringPtr(input.ScaleSet.SSHPublicKey),
								},
							},
						},
					},
				},
				StorageProfile: &compute.VirtualMachineScaleSetStorageProfile{
					ImageReference: &compute.ImageReference{
						Offer:     to.StringPtr(input.ScaleSet.Image.Offer),
						Publisher: to.StringPtr(input.ScaleSet.Image.Publisher),
						Sku:       to.StringPtr(input.ScaleSet.Image.SKU),
						Version:   to.StringPtr(input.ScaleSet.Image.Version),
					},
					OsDisk: &compute.VirtualMachineScaleSetOSDisk{
						CreateOption: compute.DiskCreateOptionTypesFromImage,
						ManagedDisk: &compute.VirtualMachineScaleSetManagedDiskParameters{
							StorageAccountType: compute.StorageAccountTypesStandardLRS,
						},
					},
				},
			},
		},
		Zones: to.StringSlicePtr(input.ScaleSet.Zones),
	}
}
