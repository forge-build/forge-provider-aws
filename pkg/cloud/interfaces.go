/*
Copyright 2024 The Forge contributors.

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

package cloud

import (
	"context"

	"github.com/aws/aws-sdk-go/service/ec2"
	infrav1 "github.com/forge-build/forge-provider-aws/pkg/api/v1alpha1"
)

type Reconciler interface {
	Reconcile(ctx context.Context) error
	Delete(ctx context.Context) error
}

// Client is an interface which can get cloud client.
type Client interface {
	Cloud() *ec2.EC2
}

// BuildGetter is an interface which can get build information.
type BuildGetter interface {
	Client
	Region() string
	SubnetID() *string
	SecurityGroupID() *string
	Name() string
	InstanceType() string
	Namespace() string
	InstanceState() *infrav1.InstanceStatus
	GetInstanceID() *string
	AMI() string
	VPCID() *string
}

// BuildSetter is an interface which can set cluster information.
type BuildSetter interface {
	SetVPCName(name string)
	SetInstanceStatus(v infrav1.InstanceStatus)
	SetInstanceID(instanceID *string)
	SetVPCID(id *string)
	SetSubnet(id *string)
}

type Build interface {
	BuildGetter
	BuildSetter
}
