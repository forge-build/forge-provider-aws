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

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// AWSServices contains all AWS SDK clients used by the scope.
type AWSServices struct {
	EC2 *ec2.EC2
}

// NewAWSServices initializes AWS SDK clients based on the provided region.
func NewAWSServices(ctx context.Context, region string, credentialsRef *corev1.SecretReference, crClient client.Client) (AWSServices, error) {

	accessKey, secretKey, err := getAWSCredentialsFromSecret(ctx, credentialsRef, crClient)
	if err != nil {
		return AWSServices{}, err
	}
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(region),
		Credentials: credentials.NewStaticCredentials(accessKey, secretKey, ""),
	})
	if err != nil {
		return AWSServices{}, err
	}

	return AWSServices{
		EC2: ec2.New(sess),
	}, nil
}
