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
	"fmt"
	"reflect"
	"strings"
)

// Labels defines a map of AWS tags.
type Labels map[string]string

// Equals returns true if the labels are equal.
func (in Labels) Equals(other Labels) bool {
	return reflect.DeepEqual(in, other)
}

// HasOwned checks if the labels contain a tag marking the resource as owned by the build.
func (in Labels) HasOwned(build string) bool {
	value, ok := in[BuildTagKey(build)]
	return ok && ResourceLifecycle(value) == ResourceLifecycleOwned
}

// ToFilterString returns the string representation of the labels as a filter for AWS SDK calls.
func (in Labels) ToFilterString() string {
	var builder strings.Builder
	for k, v := range in {
		builder.WriteString(fmt.Sprintf("tag:%s=%s ", k, v))
	}
	return builder.String()
}

// Difference returns the difference between this map of tags and the other map of tags.
func (in Labels) Difference(other Labels) Labels {
	res := make(Labels, len(in))
	for key, value := range in {
		if otherValue, ok := other[key]; ok && value == otherValue {
			continue
		}
		res[key] = value
	}
	return res
}

// AddLabels merges the current labels with another set of labels.
func (in Labels) AddLabels(other Labels) Labels {
	for key, value := range other {
		if in == nil {
			in = make(map[string]string, len(other))
		}
		in[key] = value
	}
	return in
}

// ResourceLifecycle defines the lifecycle of a resource.
type ResourceLifecycle string

const (
	// ResourceLifecycleOwned indicates that the resource is owned and managed by the build.
	ResourceLifecycleOwned = ResourceLifecycle("owned")

	// NameAWSProviderPrefix is the tag prefix for Forge AWS provider resources.
	NameAWSProviderPrefix = "forge-aws"

	// NameAWSProviderOwned is the tag name for Forge AWS provider-owned resources.
	NameAWSProviderOwned = NameAWSProviderPrefix + "-build-"
)

// BuildTagKey generates the key for resources associated with a build.
func BuildTagKey(name string) string {
	return fmt.Sprintf("%s%s", NameAWSProviderOwned, name)
}

// BuildParams is used to build tags for AWS resources.
type BuildParams struct {
	// Lifecycle determines the resource lifecycle.
	Lifecycle ResourceLifecycle

	// BuildName is the name of the build associated with the resource.
	BuildName string

	// ResourceID is the unique identifier of the resource to be tagged.
	ResourceID string

	// Additional tags to apply to the resource.
	// +optional
	Additional Labels
}

// Build generates tags including the build tag and returns them in map form.
func Build(params BuildParams) Labels {
	tags := make(Labels)
	for k, v := range params.Additional {
		tags[strings.ToLower(k)] = strings.ToLower(v)
	}
	tags[BuildTagKey(params.BuildName)] = string(params.Lifecycle)
	return tags
}
