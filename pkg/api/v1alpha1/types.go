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

import "fmt"

// NetworkSpec encapsulates all things related to an AWS network.
type NetworkSpec struct {
	// VPCID specifies the ID of the Virtual Private Cloud (VPC) for the instance.
	// +optional
	VPCID *string `json:"vpcID,omitempty"`

	// SubnetID specifies the ID of the subnet for the instance.
	// +optional
	SubnetID *string `json:"subnetID,omitempty"`

	// SecurityGroupIDs lists the security groups to associate with the instance.
	// +optional
	SecurityGroupIDs []string `json:"securityGroupIDs,omitempty"`

	// AssignPublicIP specifies whether to assign a public IP to the instance.
	// +optional
	AssignPublicIP *bool `json:"assignPublicIP,omitempty"`
}

// LoadBalancerSpec defines configuration for AWS load balancers.
type LoadBalancerSpec struct {
	// Type specifies the type of load balancer (e.g., ALB, NLB).
	// +optional
	Type *string `json:"type,omitempty"`

	// Name specifies the name of the load balancer.
	// +optional
	Name *string `json:"name,omitempty"`

	// SubnetIDs lists the subnets to use for the load balancer.
	// +optional
	SubnetIDs []string `json:"subnetIDs,omitempty"`

	// SecurityGroupIDs lists the security groups for the load balancer.
	// +optional
	SecurityGroupIDs []string `json:"securityGroupIDs,omitempty"`
}

// SubnetSpec configures an AWS Subnet.
type SubnetSpec struct {
	// SubnetID specifies the ID of the subnet.
	SubnetID string `json:"subnetID"`

	// CIDRBlock is the range of internal addresses that are owned by this subnet.
	CIDRBlock string `json:"cidrBlock"`

	// AvailabilityZone specifies the AWS Availability Zone of the subnet.
	AvailabilityZone string `json:"availabilityZone"`

	// Tags are additional tags to assign to the subnet.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// String returns a string representation of the subnet.
func (s *SubnetSpec) String() string {
	return fmt.Sprintf("subnetID=%s, cidrBlock=%s, availabilityZone=%s", s.SubnetID, s.CIDRBlock, s.AvailabilityZone)
}

// Subnets is a slice of SubnetSpec.
type Subnets []SubnetSpec

// ToMap returns a map from subnet ID to subnet spec.
func (s Subnets) ToMap() map[string]*SubnetSpec {
	res := make(map[string]*SubnetSpec)
	for i := range s {
		x := s[i]
		res[x.SubnetID] = &x
	}
	return res
}

// FindBySubnetID returns a single subnet matching the given ID or nil.
func (s Subnets) FindBySubnetID(subnetID string) *SubnetSpec {
	for _, x := range s {
		if x.SubnetID == subnetID {
			return &x
		}
	}
	return nil
}

// FilterByAvailabilityZone filters subnets by a given availability zone.
func (s Subnets) FilterByAvailabilityZone(az string) (res Subnets) {
	for _, x := range s {
		if x.AvailabilityZone == az {
			res = append(res, x)
		}
	}
	return
}

// InstanceStatus describes the state of an GCP instance.
type InstanceStatus string

var (
	// InstanceStatusProvisioning is the string representing an instance in a provisioning state.
	InstanceStatusProvisioning = InstanceStatus("PROVISIONING")

	// InstanceStatusRepairing is the string representing an instance in a repairing state.
	InstanceStatusRepairing = InstanceStatus("REPAIRING")

	// InstanceStatusRunning is the string representing an instance in a pending state.
	InstanceStatusRunning = InstanceStatus("RUNNING")

	// InstanceStatusStaging is the string representing an instance in a staging state.
	InstanceStatusStaging = InstanceStatus("STAGING")

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
	InstanceStatusSuspending = InstanceStatus("SUSPENDING")

	// InstanceStatusTerminated is the string representing an instance that has been terminated.
	InstanceStatusTerminated = InstanceStatus("TERMINATED")
)
