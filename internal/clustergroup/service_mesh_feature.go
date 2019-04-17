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

import "github.com/banzaicloud/pipeline/cluster"

type ServiceMeshClusterGroupFeature struct {
	ClusterGroupFeature
}

func (f *ServiceMeshClusterGroupFeature) Enable() error {

	return nil
}

func (f *ServiceMeshClusterGroupFeature) Disable() error {

	return nil
}

func (f *ServiceMeshClusterGroupFeature) JoinCluster(cluster cluster.CommonCluster) error {

	return nil
}
func (f *ServiceMeshClusterGroupFeature) LeaveCluster(cluster cluster.CommonCluster) error {

	return nil
}

func (f *ServiceMeshClusterGroupFeature) GetMembersStatus() (map[string]string, error) {
	statusMap := make(map[string]string, 0)
	for _, memberCluster := range f.ClusterGroup.MemberClusters {
		statusMap[memberCluster.GetName()] = "ready"
	}
	return statusMap, nil
}
