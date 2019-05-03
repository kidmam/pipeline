// Copyright © 2019 Banzai Cloud
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

package adapter

import (
	"context"

	"github.com/banzaicloud/pipeline/cluster"
	pkgCluster "github.com/banzaicloud/pipeline/pkg/cluster"
	"github.com/pkg/errors"
)

type ClusterGetter interface {
	GetClusterByIDOnly(ctx context.Context, clusterID uint) (Cluster, error)
	GetClusterByName(ctx context.Context, organizationID uint, clusterName string) (Cluster, error)
}

type Cluster interface {
	GetID() uint
	GetName() string
	GetK8sConfig() ([]byte, error)
	GetStatus() (*pkgCluster.GetClusterStatusResponse, error)
	IsReady() (bool, error)
}

type clusterGetter struct {
	clusterManager *cluster.Manager
}

// New creates a new ClusterGetter
func NewClusterGetter(manager *cluster.Manager) ClusterGetter {
	return &clusterGetter{
		clusterManager: manager,
	}
}

// GetClusterByName returns the cluster instance for an organization ID by cluster name.
func (m *clusterGetter) GetClusterByName(ctx context.Context, organizationID uint, clusterName string) (Cluster, error) {
	c, err := m.clusterManager.GetClusterByName(ctx, organizationID, clusterName)
	if err != nil {
		return nil, err
	}

	if cluster, ok := c.(Cluster); ok {
		return cluster, nil
	}

	return nil, errors.New("could not assert to Cluster")
}

// GetClusterByIDOnly returns the cluster instance by cluster ID.
func (m *clusterGetter) GetClusterByIDOnly(ctx context.Context, clusterID uint) (Cluster, error) {
	c, err := m.clusterManager.GetClusterByIDOnly(ctx, clusterID)
	if err != nil {
		return nil, err
	}

	if cluster, ok := c.(Cluster); ok {
		return cluster, nil
	}

	return nil, errors.New("could not assert to Cluster")
}
