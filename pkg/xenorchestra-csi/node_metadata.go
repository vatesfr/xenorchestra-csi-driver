// Copyright 2025 Marc Siegenthaler
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package xenorchestracsi

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/vatesfr/xenorchestra-cloud-controller-manager/pkg/xenorchestra"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "k8s.io/client-go/kubernetes"
)

type NodeMetadataGetter interface {
	GetNodeMetadata() (*NodeMetadata, error)
}

type NodeMetadata struct {
	NodeId string
	HostId string
	PoolId string
}

type NodeMetadataFromKubernetes struct {
	client   kclient.Interface
	nodeName string
}

func NewNodeMetadataFromKubernetes(client kclient.Interface, nodeName string) *NodeMetadataFromKubernetes {
	return &NodeMetadataFromKubernetes{
		client:   client,
		nodeName: nodeName,
	}
}

// TODO: rework this part to use only CCM labels and not rely on DMI product UUID
func (n *NodeMetadataFromKubernetes) GetNodeMetadata() (*NodeMetadata, error) {
	nodeId, err := getNodeIdFromDmiProductUUID()
	if err != nil {
		return nil, fmt.Errorf("failed to get node id: %w", err)
	}

	node, err := n.client.CoreV1().Nodes().Get(context.Background(), n.nodeName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get node: %w", err)
	}

	hostId := node.Labels["topology.k8s.xenorchestra/host_id"]
	poolId := node.Labels["topology.k8s.xenorchestra/pool_id"]

	return &NodeMetadata{
		NodeId: nodeId,
		HostId: hostId,
		PoolId: poolId,
	}, nil
}

// This should give us the VM UUID as reported in Xen Orchestra
// We are using `/sys/class/dmi/id/product_uuid`. However, I am not sure if there are more reliable ways to get the VM UUID.
// Another option is to use `xenstore-ls`
// There is also `/sys/hypervisor/uuid` but it's not clear if that's the same as the VM UUID
func getNodeIdFromDmiProductUUID() (string, error) {
	productUUID, err := os.ReadFile("/sys/class/dmi/id/product_uuid")
	if err != nil {
		return "", fmt.Errorf("failed to read product UUID: %w", err)
	}

	uuid := strings.TrimSpace(string(productUUID))
	if uuid == "" {
		return "", fmt.Errorf("failed to get product UUID: product UUID is empty")
	}

	return uuid, nil
}

/**
 * Use the XoClient to get all needed metadata
 */
type NodeMetadataFromXoClient struct {
	kclient  kclient.Interface
	xoClient *xenorchestra.XoClient
	nodeName string
}

func NewNodeMetadataFromXoClient(kubeClient kclient.Interface, xoClient *xenorchestra.XoClient, nodeName string) *NodeMetadataFromXoClient {
	return &NodeMetadataFromXoClient{
		xoClient: xoClient,
		kclient:  kubeClient,
		nodeName: nodeName,
	}
}

func (n *NodeMetadataFromXoClient) GetNodeMetadata() (*NodeMetadata, error) {
	node, err := n.kclient.CoreV1().Nodes().Get(context.Background(), n.nodeName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get node: %w", err)
	}
	vm, poolID, err := n.xoClient.FindVMByNode(context.Background(), node)
	if err != nil {
		return nil, fmt.Errorf("failed to find VM by node: %w", err)
	}
	return &NodeMetadata{
		NodeId: vm.ID.String(),
		HostId: vm.Container,
		PoolId: poolID.String(),
	}, nil
}
