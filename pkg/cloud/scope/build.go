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

package scope

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	infrav1 "github.com/forge-build/forge-provider-aws/pkg/api/v1alpha1"
	awsforge "github.com/forge-build/forge-provider-aws/pkg/aws"
	buildv1 "github.com/forge-build/forge/api/v1alpha1"
	"github.com/forge-build/forge/util"
	"github.com/pkg/errors"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// AWSBuildScope defines the basic context for an actuator to operate upon for AWS.
type AWSBuildScope struct {
	client      client.Client
	patchHelper *patch.Helper

	Build     *buildv1.Build
	AWSBuild  *infrav1.AWSBuild
	AWSClient *awsforge.AWSClient
	sshKEy    SSHKey
}

type SSHKey struct {
	MetadataSSHKeys string
	PrivateKey      string
	PublicKey       string
}

// AWSBuildScopeParams defines the input parameters to create an AWS BuildScope.
type AWSBuildScopeParams struct {
	Client    client.Client
	Build     *buildv1.Build
	AWSBuild  *infrav1.AWSBuild
	AWSClient *awsforge.AWSClient
}

// NewAWSBuildScope creates a new AWSBuildScope from the supplied parameters.
func NewAWSBuildScope(ctx context.Context, params AWSBuildScopeParams) (*AWSBuildScope, error) {
	if params.Build == nil {
		return nil, errors.New("failed to generate new scope from nil Build")
	}
	if params.AWSBuild == nil {
		return nil, errors.New("failed to generate new scope from nil AWSBuild")
	}

	if params.AWSClient == nil {
		awsSvc, err := awsforge.NewAWSClient(ctx, params.AWSBuild.Spec.Region, params.AWSBuild.Spec.CredentialsRef, params.Client)
		if err != nil {
			return nil, errors.Errorf("failed to create aws client: %v", err)
		}

		params.AWSClient = &awsSvc
	}

	helper, err := patch.NewHelper(params.AWSBuild, params.Client)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize patch helper")
	}

	return &AWSBuildScope{
		client:      params.Client,
		Build:       params.Build,
		AWSBuild:    params.AWSBuild,
		AWSClient:   params.AWSClient,
		patchHelper: helper,
	}, nil
}

func (s *AWSBuildScope) Cloud() *awsforge.AWSClient {
	return s.AWSClient
}

// GetSSHKey returns the ssh key.
func (s *AWSBuildScope) GetSSHKey() SSHKey {
	return s.sshKEy
}

// SetSSHKey sets ssh key.
func (s *AWSBuildScope) SetSSHKey(key SSHKey) {
	s.sshKEy = key
}

func (s *AWSBuildScope) VPCSpec() *ec2.CreateVpcInput {
	// Define a default CIDR block for new VPCs
	defaultCIDR := "10.0.0.0/16"

	// Define the input for creating a new VPC
	return &ec2.CreateVpcInput{
		CidrBlock: aws.String(defaultCIDR),
		TagSpecifications: []*ec2.TagSpecification{
			{
				ResourceType: aws.String(ec2.ResourceTypeVpc),
				Tags: []*ec2.Tag{
					{Key: aws.String("Name"), Value: s.VPCName()},
					{Key: aws.String("forge-managed"), Value: aws.String("true")}, // This tag is used to ensure if vpc is shared or not.
				},
			},
		},
	}
}

// SetVPCID sets AWS VPC ID.
func (s *AWSBuildScope) SetVPCID(id *string) {
	s.AWSBuild.Spec.Network.VPCID = id
}

func (s *AWSBuildScope) SetInstanceID(id *string) {
	s.AWSBuild.Spec.InstanceID = id
}

// SetInstanceStatus sets the EC2 Machine instance status.
func (s *AWSBuildScope) SetInstanceStatus(v infrav1.InstanceStatus) {
	s.AWSBuild.Status.InstanceStatus = &v
}

func (s *AWSBuildScope) SetSubnet(id *string) {
	s.AWSBuild.Spec.Network.SubnetID = id
}

func (s *AWSBuildScope) SetSecurityGroupID(id *string) {
	s.AWSBuild.Spec.Network.SecurityGroupID = id
}

func (s *AWSBuildScope) SetCleanedUP() {
	s.AWSBuild.Status.CleanedUP = true
}

func (s *AWSBuildScope) SetReady() {
	s.AWSBuild.Status.Ready = true
}

func (s *AWSBuildScope) SetMachineReady() {
	s.AWSBuild.Status.MachineReady = true
}

// SetVPCName sets AWS VPC Name.
func (s *AWSBuildScope) SetVPCName(name string) {
	s.AWSBuild.Spec.Network.Name = name
}

// Region returns the AWS region for the build.
func (s *AWSBuildScope) Region() string {
	return s.AWSBuild.Spec.Region
}

// Name returns the name of the build.
func (s *AWSBuildScope) Name() string {
	return s.Build.Name
}

func (s *AWSBuildScope) InstanceType() string {
	return s.AWSBuild.Spec.InstanceType
}

func (s *AWSBuildScope) InstanceState() *infrav1.InstanceStatus {
	return s.AWSBuild.Status.InstanceStatus
}

// Namespace returns the namespace of the build.
func (s *AWSBuildScope) Namespace() string {
	return s.Build.Namespace
}

// SubnetID returns the subnet ID for the AWS instance.
func (s *AWSBuildScope) SubnetID() *string {
	return s.AWSBuild.Spec.Network.SubnetID
}

func (s *AWSBuildScope) IsProvisionerReady() bool {
	return s.Build.Status.ProvisionersReady
}

func (s *AWSBuildScope) GetInstanceID() *string {
	return s.AWSBuild.Spec.InstanceID
}

// SecurityGroupID returns the security group for the instance.
func (s *AWSBuildScope) SecurityGroupID() *string {
	return s.AWSBuild.Spec.Network.SecurityGroupID
}

func (s *AWSBuildScope) PublicIP() *bool {
	return s.AWSBuild.Spec.PublicIP
}

// AMI returns the Amazon Machine Image (AMI) ID.
func (s *AWSBuildScope) AMI() string {
	return aws.StringValue(s.AWSBuild.Spec.AMI)
}

// IAMRole returns the IAM role for the instance.
func (s *AWSBuildScope) IAMRole() string {
	return aws.StringValue(s.AWSBuild.Spec.IAMRole)
}

// PatchObject persists the build configuration and status.
func (s *AWSBuildScope) PatchObject() error {
	return s.patchHelper.Patch(context.TODO(), s.AWSBuild)
}

// Close closes the current scope persisting the build configuration and status.
func (s *AWSBuildScope) Close() error {
	return s.PatchObject()
}

func (s *AWSBuildScope) VPCID() *string {
	return s.AWSBuild.Spec.Network.VPCID
}

func (s *AWSBuildScope) VPCName() *string {
	// Define a default name
	vpcName := aws.String(fmt.Sprintf("%s-%s-vpc", s.Name(), "forge"))
	// If the VPC ID is provided, no need to create a new VPC
	if s.AWSBuild.Spec.Network.Name != "" {
		vpcName = aws.String(s.AWSBuild.Spec.Network.Name)
	}
	return vpcName
}

func (s *AWSBuildScope) SecurityGroupName() *string {
	// Define a default name
	sgName := aws.String(fmt.Sprintf("%s-%s", s.Name(), "forge"))
	// If the VPC ID is provided, no need to generate new Name
	if s.AWSBuild.Spec.Network.Name != "" {
		sgName = aws.String(s.AWSBuild.Spec.Network.Name)
	}
	return sgName
}

func (s *AWSBuildScope) IsReady() bool {
	return s.AWSBuild.Status.Ready
}

func (s *AWSBuildScope) IsCleanedUP() bool {
	return s.AWSBuild.Status.CleanedUP
}

func (s *AWSBuildScope) CreationDate() string {
	return s.AWSBuild.CreationTimestamp.Time.Format(time.RFC3339)
}
func (s *AWSBuildScope) SetArtifactRef(reference string) {
	s.AWSBuild.Status.ArtifactRef = &reference
}

func (s *AWSBuildScope) EnsureCredentialsSecret(ctx context.Context, host string) error {
	err := util.EnsureCredentialsSecret(ctx, s.client, s.Build, util.SSHCredentials{
		Host:       host,
		Username:   s.AWSBuild.Spec.Username,
		PrivateKey: s.sshKEy.PrivateKey,
		PublicKey:  s.sshKEy.PublicKey,
	}, "aws")
	if err != nil {
		return err
	}
	return nil
}

// createUserData generates a cloud-init user data script that creates a specified user and installs an SSH key.
func (s *AWSBuildScope) UserData() *string {
	cloudConfigTemplate := `#cloud-config
users:
  - name: %s
    groups: sudo
    shell: /bin/bash
    sudo: ['ALL=(ALL) NOPASSWD:ALL']
    ssh_authorized_keys:
      - %s
`

	// Insert the username and SSH key into the template
	cloudConfig := fmt.Sprintf(cloudConfigTemplate, s.AWSBuild.Spec.Username, s.sshKEy.PublicKey)

	// User data must be Base64-encoded
	encodedUserData := base64.StdEncoding.EncodeToString([]byte(cloudConfig))
	return &encodedUserData
}
