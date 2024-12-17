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

package aws

import (
	"bytes"
	"fmt"
	"net"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/pkg/errors"
)

func findAvailableCIDR(vpcCIDR string, usedCIDRs []string, subnetMask int) (string, error) {
	_, vpcIPNet, err := net.ParseCIDR(vpcCIDR)
	if err != nil {
		return "", fmt.Errorf("failed to parse VPC CIDR %s: %w", vpcCIDR, err)
	}

	// Iterate through potential subnets in the VPC CIDR
	for ip := vpcIPNet.IP.Mask(vpcIPNet.Mask); vpcIPNet.Contains(ip); incrementIP(ip) {
		// Create a candidate CIDR block
		candidateCIDR := fmt.Sprintf("%s/%d", ip.String(), subnetMask)

		// Check if the candidate overlaps with any used CIDRs
		inUse, err := isCIDRInUse(candidateCIDR, usedCIDRs)
		if err != nil {
			return "", err
		}
		if !inUse {
			// Found an available CIDR
			return candidateCIDR, nil
		}
	}

	return "", errors.New("no available CIDR block found")
}

// incrementIP increments an IP address in-place.
func incrementIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

// parseCIDR parses a CIDR block into an IP range.
func parseCIDR(cidr string) (net.IP, net.IP, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse CIDR %s: %w", cidr, err)
	}
	firstIP := ipNet.IP
	lastIP := make(net.IP, len(firstIP))
	copy(lastIP, firstIP)
	for i := range ipNet.Mask {
		lastIP[i] |= ^ipNet.Mask[i]
	}
	return firstIP, lastIP, nil
}

// isCIDRInUse checks if a candidate CIDR overlaps with any used CIDR.
func isCIDRInUse(candidateCIDR string, usedCIDRs []string) (bool, error) {
	candidateFirst, candidateLast, err := parseCIDR(candidateCIDR)
	if err != nil {
		return false, err
	}

	for _, usedCIDR := range usedCIDRs {
		usedFirst, usedLast, err := parseCIDR(usedCIDR)
		if err != nil {
			return false, err
		}

		if bytesCompare(candidateFirst, usedLast) <= 0 && bytesCompare(candidateLast, usedFirst) >= 0 {
			// Candidate overlaps with a used CIDR
			return true, nil
		}
	}
	return false, nil
}

// bytesCompare compares two byte slices lexicographically.
func bytesCompare(a, b net.IP) int {
	return bytes.Compare(a, b)
}

func GetNameFromTags(tags []*ec2.Tag) string {
	for _, tag := range tags {
		if aws.StringValue(tag.Key) == "Name" {
			return *tag.Value
		}
	}
	return ""
}
