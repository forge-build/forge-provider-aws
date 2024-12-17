/*
Copyright 2024 The Forge Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

// NetworkSpec encapsulates all things related to an AWS network.
type NetworkSpec struct {

	// Name specifies the Name of the Virtual Private Cloud (VPC) for the instance.
	// +optional
	Name string `json:"name,omitempty"`

	// VPCID specifies the ID of the Virtual Private Cloud (VPC) for the instance.
	// +optional
	VPCID *string `json:"vpcID,omitempty"`

	// SubnetID specifies the ID of the subnet for the instance.
	// +optional
	SubnetID *string `json:"subnetID,omitempty"`

	// SecurityGroupID list the security group to associate with the instance.
	// +optional
	SecurityGroupID *string `json:"securityGroup,omitempty"`

	// AssignPublicIP specifies whether to assign a public IP to the instance.
	// +optional
	AssignPublicIP *bool `json:"assignPublicIP,omitempty"`
}

// InstanceStatus describes the state of an EC2 instance.
type InstanceStatus string

var (
	// InstanceStatusProvisioning is the string representing an instance in a provisioning state.
	InstanceStatusProvisioning = InstanceStatus("PROVISIONING")

	// InstanceStatusRepairing is the string representing an instance in a repairing state.
	InstanceStatusRepairing = InstanceStatus("REPAIRING")

	// InstanceStatusRunning is the string representing an instance in a pending state.
	InstanceStatusRunning = InstanceStatus("RUNNING")

	// InstanceStatusStaging is the string representing an instance in a staging state.
	InstanceStatusStaging = InstanceStatus("PENDING")

	// InstanceStatusStopped is the string representing an instance
	// that has been stopped and can be restarted.
	InstanceStatusStopped = InstanceStatus("STOPPED")

	// InstanceStatusStopping is the string representing an instance
	// that is in the process of being stopped and can be restarted.
	InstanceStatusStopping = InstanceStatus("STOPPING")

	// InstanceStatusSuspended is the string representing an instance
	// that is suspended.
	InstanceStatusSuspended = InstanceStatus("SUSPENDED")

	// InstanceStatusSuspending is the string representing an instance
	// that is in the process of being suspended.
	InstanceStatusShuttingDown = InstanceStatus("SHUTTING-DOWN")

	// InstanceStatusTerminated is the string representing an instance that has been terminated.
	InstanceStatusTerminated = InstanceStatus("TERMINATED")
)
