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

package awserrors

import (
	"errors"
	"strings"

	"github.com/aws/aws-sdk-go/aws/awserr"
)

var ErrInstanceNotTerminated = errors.New("the Instance is not terminated yet, Waiting")

// IsNotFound checks if the error is a "not found" error for resources.
func IsNotFound(err error) bool {
	if awsErr, ok := err.(awserr.Error); ok {
		code := awsErr.Code()
		return strings.Contains(code, "NotFound")
	}
	return false
}

// IgnoreNotFound ignore AWS API not found error and return nil.
// Otherwise return the actual error.
func IgnoreNotFound(err error) error {
	if IsNotFound(err) {
		return nil
	}

	return err
}

func IsInstanceNotTerminated(err error) bool {
	return errors.Is(err, ErrInstanceNotTerminated)
}
