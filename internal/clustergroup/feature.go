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

package clustergroup

import (
	"github.com/banzaicloud/pipeline/cluster"
	cgroup "github.com/banzaicloud/pipeline/pkg/clustergroup"
)

type ClusterGroupFeature struct {
	Name         string
	ClusterGroup cgroup.ClusterGroup
	Enabled      bool
	Params       map[string]string
}

type ClusterGroupFeatureHandler interface {
	Enable() error
	Disable() error
	JoinCluster(cluster cluster.CommonCluster) error
	LeaveCluster(cluster cluster.CommonCluster) error
	SetParam(name string, value string)
	SetEnabled(enabled bool)
	GetMembersStatus() (map[string]string, error)
}

func (c *ClusterGroupFeature) SetParam(name string, value string) {
	c.Params[name] = value
}

func (c *ClusterGroupFeature) SetEnabled(enabled bool) {
	c.Enabled = enabled
}
