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

package awstasks

import (
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"k8s.io/kops/cloudmock/aws/mockec2"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup/awsup"
)

func TestSharedVpnGatewayDoesNotRename(t *testing.T) {
	cloud := awsup.BuildMockAWSCloud("us-east-1", "abc")
	c := &mockec2.MockEC2{}
	cloud.MockEC2 = c

	// Pre-create the vpc / subnet
	vpc, err := c.CreateVpc(&ec2.CreateVpcInput{
		CidrBlock: aws.String("172.20.0.0/16"),
	})
	if err != nil {
		t.Fatalf("error creating test VPC: %v", err)
	}
	_, err = c.CreateTags(&ec2.CreateTagsInput{
		Resources: []*string{vpc.Vpc.VpcId},
		Tags: []*ec2.Tag{
			{
				Key:   aws.String("Name"),
				Value: aws.String("ExistingVPC"),
			},
		},
	})
	if err != nil {
		t.Fatalf("error tagging test vpc: %v", err)
	}

	vpnGateway, err := c.CreateVpnGateway(&ec2.CreateVpnGatewayInput{})
	if err != nil {
		t.Fatalf("error creating test vgw: %v", err)
	}

	_, err = c.CreateTags(&ec2.CreateTagsInput{
		Resources: []*string{vpnGateway.VpnGateway.VpnGatewayId},
		Tags: []*ec2.Tag{
			{
				Key:   aws.String("Name"),
				Value: aws.String("ExistingVpnGateway"),
			},
		},
	})
	if err != nil {
		t.Fatalf("error tagging test vgw: %v", err)
	}

	_, err = c.AttachVpnGateway(&ec2.AttachVpnGatewayInput{
		VpnGatewayId: vpnGateway.VpnGateway.VpnGatewayId,
		VpcId:        vpc.Vpc.VpcId,
	})

	// We define a function so we can rebuild the tasks, because we modify in-place when running
	buildTasks := func() map[string]fi.Task {
		vpc1 := &VPC{
			Name:   s("vpc1"),
			CIDR:   s("172.20.0.0/16"),
			Tags:   map[string]string{"kubernetes.io/cluster/cluster.example.com": "shared"},
			Shared: fi.Bool(true),
			ID:     vpc.Vpc.VpcId,
		}
		vgw1 := &VpnGateway{
			Name: s("vgw1"),
			VPC:  vpc1,
			ID:   vpnGateway.VpnGateway.VpnGatewayId,
			Tags: make(map[string]string),
		}

		return map[string]fi.Task{
			"vgw1": vgw1,
			"vpc1": vpc1,
		}
	}

	{
		allTasks := buildTasks()
		vgw1 := allTasks["vgw1"].(*VpnGateway)

		target := &awsup.AWSAPITarget{
			Cloud: cloud,
		}

		context, err := fi.NewContext(target, nil, cloud, nil, nil, nil, true, allTasks)
		if err != nil {
			t.Fatalf("error building context: %v", err)
		}

		if err := context.RunTasks(testRunTasksOptions); err != nil {
			t.Fatalf("unexpected error during Run: %v", err)
		}

		if fi.StringValue(vgw1.ID) == "" {
			t.Fatalf("ID not set after create")
		}

		if len(c.VpnGatewayIds()) != 1 {
			t.Fatalf("Expected exactly one VpnGateway; found %v", c.VpnGatewayIds())
		}

		actual := c.FindVpnGateway(*vpnGateway.VpnGateway.VpnGatewayId)
		if actual == nil {
			t.Fatalf("VpnGateway created but then not found")
		}
		expected := &ec2.VpnGateway{
			VpnGatewayId: aws.String("vgw-1"),
			Tags: buildTags(map[string]string{
				"Name": "ExistingVpnGateway",
			}),
			VpcAttachments: []*ec2.VpcAttachment{
				{
					VpcId: vpc.Vpc.VpcId,
				},
			},
		}

		mockec2.SortTags(expected.Tags)
		mockec2.SortTags(actual.Tags)

		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("Unexpected VpnGateway: expected=%v actual=%v", expected, actual)
		}
	}

	{
		allTasks := buildTasks()
		checkNoChanges(t, cloud, allTasks)
	}
}
