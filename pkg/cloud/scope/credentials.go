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

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func getAWSCredentialsFromSecret(ctx context.Context, credentialsRef *corev1.SecretReference, kubeClient client.Client) (string, string, error) {
	secretRef := types.NamespacedName{
		Name:      credentialsRef.Name,
		Namespace: credentialsRef.Namespace,
	}

	secret := &corev1.Secret{}
	if err := kubeClient.Get(ctx, secretRef, secret); err != nil {
		return "", "", errors.Wrapf(err, "failed to fetch AWS credentials secret %s/%s", secretRef.Namespace, secretRef.Name)
	}

	accessKey, ok := secret.Data["aws_access_key_id"]
	if !ok {
		return "", "", errors.New("aws_access_key_id key missing in secret")
	}

	secretKey, ok := secret.Data["aws_secret_access_key"]
	if !ok {
		return "", "", errors.New("aws_secret_access_key key missing in secret")
	}

	return string(accessKey), string(secretKey), nil
}
