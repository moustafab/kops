/*
Copyright 2017 The Kubernetes Authors.

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

package mockec2

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/golang/glog"
)

func (m *MockEC2) FindVpnGateway(id string) *ec2.VpnGateway {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	VpnGateway := m.VpnGateways[id]
	if VpnGateway == nil {
		return nil
	}

	gatewayCopy := *VpnGateway
	gatewayCopy.Tags = m.getTags(ec2.ResourceTypeVpnGateway, id)
	return &gatewayCopy
}

func (m *MockEC2) VpnGatewayIds() []string {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	var ids []string
	for id := range m.VpnGateways {
		ids = append(ids, id)
	}
	return ids
}

func (m *MockEC2) CreateVpnGatewayRequest(input *ec2.CreateVpnGatewayInput) (*request.Request, *ec2.CreateVpnGatewayOutput) {
	panic("Not implemented")
}

func (m *MockEC2) CreateVpnGatewayWithContext(aws.Context, *ec2.CreateVpnGatewayInput, ...request.Option) (*ec2.CreateVpnGatewayOutput, error) {
	panic("Not implemented")
}

func (m *MockEC2) CreateVpnGateway(request *ec2.CreateVpnGatewayInput) (*ec2.CreateVpnGatewayOutput, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	glog.Infof("CreateVpnGateway: %v", request)

	id := m.allocateId("vgw")

	vgw := &ec2.VpnGateway{
		VpnGatewayId: s(id),
	}

	if m.VpnGateways == nil {
		m.VpnGateways = make(map[string]*ec2.VpnGateway)
	}
	m.VpnGateways[id] = vgw

	response := &ec2.CreateVpnGatewayOutput{
		VpnGateway: vgw,
	}
	return response, nil
}

func (m *MockEC2) DescribeVpnGatewaysRequest(*ec2.DescribeVpnGatewaysInput) (*request.Request, *ec2.DescribeVpnGatewaysOutput) {
	panic("Not implemented")
}

func (m *MockEC2) DescribeVpnGatewaysWithContext(aws.Context, *ec2.DescribeVpnGatewaysInput, ...request.Option) (*ec2.DescribeVpnGatewaysOutput, error) {
	panic("Not implemented")
}

func (m *MockEC2) DescribeVpnGateways(request *ec2.DescribeVpnGatewaysInput) (*ec2.DescribeVpnGatewaysOutput, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	glog.Infof("DescribeVpnGateways: %v", request)

	var VpnGateways []*ec2.VpnGateway

	if len(request.VpnGatewayIds) != 0 {
		request.Filters = append(request.Filters, &ec2.Filter{Name: s("internet-gateway-id"), Values: request.VpnGatewayIds})
	}

	for id, VpnGateway := range m.VpnGateways {
		allFiltersMatch := true
		for _, filter := range request.Filters {
			match := false
			switch *filter.Name {
			case "internet-gateway-id":
				for _, v := range filter.Values {
					if id == aws.StringValue(v) {
						match = true
					}
				}

			case "attachment.vpc-id":
				for _, v := range filter.Values {
					if VpnGateway.VpcAttachments != nil {
						for _, attachment := range VpnGateway.VpcAttachments {
							if *attachment.VpcId == *v {
								match = true
							}
						}
					}
				}

			default:
				if strings.HasPrefix(*filter.Name, "tag:") {
					match = m.hasTag(ec2.ResourceTypeVpnGateway, id, filter)
				} else {
					return nil, fmt.Errorf("unknown filter name: %q", *filter.Name)
				}
			}

			if !match {
				allFiltersMatch = false
				break
			}
		}

		if !allFiltersMatch {
			continue
		}

		gatewayCopy := *VpnGateway
		gatewayCopy.Tags = m.getTags(ec2.ResourceTypeVpnGateway, id)
		VpnGateways = append(VpnGateways, &gatewayCopy)
	}

	response := &ec2.DescribeVpnGatewaysOutput{
		VpnGateways: VpnGateways,
	}

	return response, nil
}

func (m *MockEC2) AttachVpnGateway(request *ec2.AttachVpnGatewayInput) (*ec2.AttachVpnGatewayOutput, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for id, VpnGateway := range m.VpnGateways {
		if id == *request.VpnGatewayId {
			VpnGateway.VpcAttachments = append(VpnGateway.VpcAttachments,
				&ec2.VpcAttachment{
					VpcId: request.VpcId,
				})
			return &ec2.AttachVpnGatewayOutput{}, nil
		}
	}

	return nil, fmt.Errorf("VpnGateway not found")

}

func (m *MockEC2) AttachVpnGatewayWithContext(aws.Context, *ec2.AttachVpnGatewayInput, ...request.Option) (*ec2.AttachVpnGatewayOutput, error) {
	panic("Not implemented")
}
func (m *MockEC2) AttachVpnGatewayRequest(*ec2.AttachVpnGatewayInput) (*request.Request, *ec2.AttachVpnGatewayOutput) {
	panic("Not implemented")
}

func (m *MockEC2) DetachVpnGateway(request *ec2.DetachVpnGatewayInput) (*ec2.DetachVpnGatewayOutput, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for id, vgw := range m.VpnGateways {
		if id == *request.VpnGatewayId {
			found := false
			var newAttachments []*ec2.VpcAttachment
			for _, a := range vgw.VpcAttachments {
				if aws.StringValue(a.VpcId) == aws.StringValue(request.VpcId) {
					found = true
					continue
				}
				newAttachments = append(newAttachments, a)
			}

			if !found {
				return nil, fmt.Errorf("Attachment to VPC not found")
			}
			vgw.VpcAttachments = newAttachments

			return &ec2.DetachVpnGatewayOutput{}, nil
		}
	}

	return nil, fmt.Errorf("VpnGateway not found")
}

func (m *MockEC2) DetachVpnGatewayWithContext(aws.Context, *ec2.DetachVpnGatewayInput, ...request.Option) (*ec2.DetachVpnGatewayOutput, error) {
	panic("Not implemented")
}
func (m *MockEC2) DetachVpnGatewayRequest(*ec2.DetachVpnGatewayInput) (*request.Request, *ec2.DetachVpnGatewayOutput) {
	panic("Not implemented")
}

func (m *MockEC2) DeleteVpnGateway(request *ec2.DeleteVpnGatewayInput) (*ec2.DeleteVpnGatewayOutput, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	glog.Infof("DeleteVpnGateway: %v", request)

	id := aws.StringValue(request.VpnGatewayId)
	o := m.VpnGateways[id]
	if o == nil {
		return nil, fmt.Errorf("VpnGateway %q not found", id)
	}
	delete(m.VpnGateways, id)

	return &ec2.DeleteVpnGatewayOutput{}, nil
}

func (m *MockEC2) DeleteVpnGatewayWithContext(aws.Context, *ec2.DeleteVpnGatewayInput, ...request.Option) (*ec2.DeleteVpnGatewayOutput, error) {
	panic("Not implemented")
}
func (m *MockEC2) DeleteVpnGatewayRequest(*ec2.DeleteVpnGatewayInput) (*request.Request, *ec2.DeleteVpnGatewayOutput) {
	panic("Not implemented")
}
