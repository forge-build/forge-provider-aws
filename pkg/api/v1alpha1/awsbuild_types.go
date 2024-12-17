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

import (
	buildv1 "github.com/forge-build/forge/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

const (
	// BuildFinalizer allows ReconcileAWSBuild to clean up AWS resources associated with AWSBuild before
	// removing it from the apiserver.
	BuildFinalizer = "awsbuild.infrastructure.forge.build"

	// AWSBuildKind is the kind of an AWSBuild Object.
	AWSBuildKind string = "AWSBuild"
)

// DiskType represents the type of disk in AWS.
type DiskType string

const (
	// GeneralPurposeSSD represents gp2/gp3 volumes.
	GeneralPurposeSSD DiskType = "gp2"
	// ProvisionedIOPS represents io1/io2 volumes.
	ProvisionedIOPS DiskType = "io1"
	// Magnetic represents st1/sc1 volumes.
	Magnetic DiskType = "st1"
)

// AttachedVolumeSpec defines AWS machine volumes.
type AttachedVolumeSpec struct {
	// VolumeType specifies the type of volume (e.g., gp2, io1).
	// +optional
	VolumeType *DiskType `json:"volumeType,omitempty"`

	// Size is the size of the volume in GB.
	// Defaults to 30GB.
	// +optional
	Size *int64 `json:"size,omitempty"`

	// IOPS specifies the IOPS for provisioned IOPS volumes.
	// +optional
	IOPS *int64 `json:"iops,omitempty"`

	// EncryptionKey defines the KMS key to be used to encrypt the volume.
	// +optional
	EncryptionKey *string `json:"encryptionKey,omitempty"`
}

// AWSBuildSpec defines the desired state of AWSBuild.
type AWSBuildSpec struct {
	// Embedded ConnectionSpec to define default connection credentials.
	buildv1.ConnectionSpec `json:",inline"`

	// Region is the AWS region for the build.
	Region string `json:"region"`

	// InstanceType is the EC2 instance type (e.g., t2.micro, m5.large).
	InstanceType string `json:"instanceType"`

	// VPCName encapsultes all the things related to AWS VPC
	// +optional
	Network NetworkSpec `json:"network"`

	// AMI is the Amazon Machine Image ID to use for the instance.
	// +optional
	AMI *string `json:"ami,omitempty"`

	// RootVolume specifies the root volume configuration.
	// +optional
	RootVolume *AttachedVolumeSpec `json:"rootVolume,omitempty"`

	// AdditionalVolumes defines additional volumes to attach to the instance.
	// +optional
	AdditionalVolumes []AttachedVolumeSpec `json:"additionalVolumes,omitempty"`

	// PublicIP specifies whether the instance should have a public IP.
	// +optional
	PublicIP *bool `json:"publicIP,omitempty"`

	// IAMRole specifies the IAM role to associate with the instance.
	// +optional
	IAMRole *string `json:"iamRole,omitempty"`

	// InstanceID is the unique identifier as specified by the cloud provider.
	// +optional
	InstanceID *string `json:"InstanceID,omitempty"`

	// CredentialsRef is a reference to a Secret that contains the credentials to use for provisioning this cluster. If not
	// supplied then the credentials of the controller will be used.
	// +optional
	CredentialsRef *corev1.SecretReference `json:"credentialsRef,omitempty"`
}

// AWSBuildStatus defines the observed state of AWSBuild.
type AWSBuildStatus struct {
	// Ready indicates that the GCPBuild is ready.
	// +optional
	// +kubebuilder:default=false
	Ready bool `json:"ready"`

	// MachineReady indicates that the associated machine is ready to accept connection.
	// +optional
	// +kubebuilder:default=false
	MachineReady bool `json:"machineReady"`

	// CleanUpReady indicates that the Infrastructure is cleaned up or not.
	// +optional
	// +kubebuilder:default=false
	CleanedUP bool `json:"cleanedUP,omitempty"`

	// InstanceStatus is the status of the GCP instance for this machine.
	// +optional
	InstanceStatus *InstanceStatus `json:"instanceState,omitempty"`

	// ArtifactRef is the reference to the built artifact.
	// +optional
	ArtifactRef *string `json:"artifactRef,omitempty"`

	// FailureReason describes why the build failed, if applicable.
	// +optional
	FailureReason *string `json:"failureReason,omitempty"`

	// FailureMessage provides additional information about a failure.
	// +optional
	FailureMessage *string `json:"failureMessage,omitempty"`

	// Conditions lists the current conditions of the AWSBuild.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=awsbuilds,scope=Namespaced,categories=forge;aws,singular=awsbuild
// +kubebuilder:printcolumn:name="Build",type="string",JSONPath=".metadata.labels['forge\\.build/build-name']",description="Build"
// +kubebuilder:printcolumn:name="Instance ID",type="string",JSONPath=".status.instanceID",description="Instance ID"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready",description="Build is ready"

// AWSBuild is the Schema for the awsbuilds API.
type AWSBuild struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AWSBuildSpec   `json:"spec,omitempty"`
	Status AWSBuildStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AWSBuildList contains a list of AWSBuilds.
type AWSBuildList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AWSBuild `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AWSBuild{}, &AWSBuildList{})
}
