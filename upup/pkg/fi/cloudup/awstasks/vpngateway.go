/*
Copyright 2016 The Kubernetes Authors.

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

package awstasks

import (
	"fmt"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/golang/glog"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup/awsup"
	"k8s.io/kops/upup/pkg/fi/cloudup/cloudformation"
	"k8s.io/kops/upup/pkg/fi/cloudup/terraform"
)

//go:generate fitask -type=VpnGateway
type VpnGateway struct {
	Name      *string
	Lifecycle *fi.Lifecycle

	ID  *string
	VPC *VPC
	// Tags is a map of aws tags that are added to the VpnGateway
	Tags map[string]string
}

// Vpn Gateways are always shared

var _ fi.CompareWithID = &VpnGateway{}

func (e *VpnGateway) CompareWithID() *string {
	return e.ID
}

func findVpnGateway(cloud awsup.AWSCloud, request *ec2.DescribeVpnGatewaysInput) (*ec2.VpnGateway, error) {
	response, err := cloud.EC2().DescribeVpnGateways(request)
	if err != nil {
		return nil, fmt.Errorf("error listing VpnGateways: %v", err)
	}
	if response == nil || len(response.VpnGateways) == 0 {
		return nil, nil
	}

	if len(response.VpnGateways) != 1 {
		return nil, fmt.Errorf("found multiple VpnGateways matching tags")
	}
	vgw := response.VpnGateways[0]
	return vgw, nil
}

func (e *VpnGateway) Find(c *fi.Context) (*VpnGateway, error) {
	cloud := c.Cloud.(awsup.AWSCloud)

	request := &ec2.DescribeVpnGatewaysInput{}

	if fi.StringValue(e.VPC.ID) == "" {
		return nil, fmt.Errorf("VPC ID is required when using VpnGateway")
	}

	request.Filters = []*ec2.Filter{awsup.NewEC2Filter("attachment.vpc-id", *e.VPC.ID)}

	if e.ID != nil {
		request.VpnGatewayIds = []*string{e.ID}
	} else {
		request.Filters = cloud.BuildFilters(e.Name)
	}

	vgw, err := findVpnGateway(cloud, request)
	if err != nil {
		return nil, err
	}
	if vgw == nil {
		return nil, nil
	}
	actual := &VpnGateway{
		ID:   vgw.VpnGatewayId,
		Name: findNameTag(vgw.Tags),
		Tags: intersectTags(vgw.Tags, e.Tags),
	}

	glog.V(2).Infof("found matching VpnGateway %q", *actual.ID)

	for _, attachment := range vgw.VpcAttachments {
		actual.VPC = &VPC{ID: attachment.VpcId}
	}

	// Prevent spurious comparison failures
	actual.Lifecycle = e.Lifecycle
	actual.Name = e.Name
	e.ID = actual.ID
	actual.Tags = e.Tags

	return actual, nil
}

func (e *VpnGateway) Run(c *fi.Context) error {
	return fi.DefaultDeltaRunMethod(e, c)
}

func (s *VpnGateway) CheckChanges(a, e, changes *VpnGateway) error {
	if a != nil {
		// TODO: need look into what to validate before changing; we should probably only allow attachment of vpcs
		if changes.Name != nil {
			return fi.CannotChangeField("Name")
		}
		if changes.ID != nil {
			return fi.CannotChangeField("ID")
		}
		if changes.Tags != nil {
			return fi.CannotChangeField("Tags")
		}
	}

	return nil
}

func (_ *VpnGateway) RenderAWS(t *awsup.AWSAPITarget, a, e, changes *VpnGateway) error {
	// TODO: not sure we want to do creation through kops
	if a == nil {
		return fmt.Errorf("error cannot create VpnGateway")
	}

	if changes != nil && changes.VPC != nil {
		glog.V(2).Infof("Creating VpcAttachment")

		attachRequest := &ec2.AttachVpnGatewayInput{
			VpcId:        e.VPC.ID,
			VpnGatewayId: e.ID,
		}

		_, err := t.Cloud.EC2().AttachVpnGateway(attachRequest)
		if err != nil {
			return fmt.Errorf("error attaching VpnGateway to VPC: %v", err)
		}
	}

	return nil
}

type terraformVpnGateway struct {
	VPCID *terraform.Literal `json:"vpc_id"`
	Tags  map[string]string  `json:"tags,omitempty"`
}

func (_ *VpnGateway) RenderTerraform(t *terraform.TerraformTarget, a, e, changes *VpnGateway) error {
	// Not terraform owned / managed
	// But ... attempt to discover the ID so TerraformLink works
	if e.ID == nil {
		request := &ec2.DescribeVpnGatewaysInput{}
		vpcID := fi.StringValue(e.VPC.ID)
		if vpcID == "" {
			return fmt.Errorf("VPC ID is required when VpnGateway is shared")
		}
		request.Filters = []*ec2.Filter{awsup.NewEC2Filter("attachment.vpc-id", vpcID)}
		vgw, err := findVpnGateway(t.Cloud.(awsup.AWSCloud), request)
		if err != nil {
			return err
		}
		if vgw == nil {
			glog.Warningf("Cannot find virtual gateway for VPC %q", vpcID)
		} else {
			e.ID = vgw.VpnGatewayId
		}
	}

	return nil
}

func (e *VpnGateway) TerraformLink() *terraform.Literal {
	if e.ID == nil {
		glog.Fatalf("ID must be set, if VpnGateway is shared: %s", e)
	}

	glog.V(4).Infof("reusing existing VpnGateway with id %q", *e.ID)
	return terraform.LiteralFromStringValue(*e.ID)
}

type cloudformationVpnGateway struct {
	Tags []cloudformationTag `json:"Tags,omitempty"`
}

type cloudformationVpcVpnGatewayAttachment struct {
	VpcId        *cloudformation.Literal `json:"VpcId,omitempty"`
	VpnGatewayId *cloudformation.Literal `json:"VpnGatewayId,omitempty"`
}

func (_ *VpnGateway) RenderCloudformation(t *cloudformation.CloudformationTarget, a, e, changes *VpnGateway) error {
	// Not cloudformation owned / managed

	// But ... attempt to discover the ID so CloudformationLink works
	if e.ID == nil {
		request := &ec2.DescribeVpnGatewaysInput{}
		vpcID := fi.StringValue(e.VPC.ID)
		if vpcID == "" {
			return fmt.Errorf("VPC ID is required when VpnGateway is shared")
		}
		request.Filters = []*ec2.Filter{awsup.NewEC2Filter("attachment.vpc-id", vpcID)}
		vgw, err := findVpnGateway(t.Cloud.(awsup.AWSCloud), request)
		if err != nil {
			return err
		}
		if vgw == nil {
			glog.Warningf("Cannot find vpn gateway for VPC %q", vpcID)
		} else {
			e.ID = vgw.VpnGatewayId
		}
	}

	return nil
}

func (e *VpnGateway) CloudformationLink() *cloudformation.Literal {
	if e.ID == nil {
		glog.Fatalf("ID must be set, if VpnGateway is shared: %s", e)
	}

	glog.V(4).Infof("reusing existing VpnGateway with id %q", *e.ID)
	return cloudformation.LiteralString(*e.ID)
}
