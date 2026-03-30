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

	xok8s "github.com/vatesfr/xenorchestra-k8s-common"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "k8s.io/client-go/kubernetes"
)

type NodeMetadataGetter interface {
	GetNodeMetadata() (*NodeMetadata, error)
}

type NodeMetadata struct {
	NodeId string
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

// GetNodeMetadata retrieves node identity and topology metadata from Kubernetes.
func (n *NodeMetadataFromKubernetes) GetNodeMetadata() (*NodeMetadata, error) {
	node, err := n.client.CoreV1().Nodes().Get(context.Background(), n.nodeName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get node: %w", err)
	}

	vm, poolId, err := xok8s.ParseProviderID(node.Spec.ProviderID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse provider ID: %w", err)
	}

	return &NodeMetadata{
		NodeId: vm.ID.String(),
		PoolId: poolId.String(),
	}, nil
}

// NodeMetadataFromXoClient uses the XoClient to get all needed metadata.
type NodeMetadataFromXoClient struct {
	kclient  kclient.Interface
	xoClient *xok8s.XoClient
	nodeName string
}

func NewNodeMetadataFromXoClient(kubeClient kclient.Interface, xoClient *xok8s.XoClient, nodeName string) *NodeMetadataFromXoClient {
	return &NodeMetadataFromXoClient{
		xoClient: xoClient,
		kclient:  kubeClient,
		nodeName: nodeName,
	}
}

// GetNodeMetadata retrieves node identity and topology metadata directly from
// the XenOrchestra API. This implementation does not depend on the CCM having
// labeled the node beforehand; it resolves the pool ID by querying XO at startup.
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
		PoolId: poolID.String(),
	}, nil
}
