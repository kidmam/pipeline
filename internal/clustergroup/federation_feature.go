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
	"github.com/goph/emperror"
	"github.com/sirupsen/logrus"

	"github.com/banzaicloud/pipeline/internal/clustergroup/api"
)

type FederationHandler struct {
	logger       logrus.FieldLogger
	errorHandler emperror.Handler
}

const FederationFeatureName = "federation"
const DeploymentFeatureName = "deployment"

// NewFederationHandler returns a new FederationHandler instance.
func NewFederationHandler(
	logger logrus.FieldLogger,
	errorHandler emperror.Handler,
) *FederationHandler {
	return &FederationHandler{
		logger:       logger,
		errorHandler: errorHandler,
	}
}

func (f *FederationHandler) ReconcileState(featureState api.Feature) error {
	f.logger.Infof("federation enabled %v on group: %v", featureState.Enabled, featureState.ClusterGroup.Name)
	return nil
}

func (f *FederationHandler) ValidateState(featureState api.Feature) error {
	return nil
}

func (f *FederationHandler) ValidateProperties(properties interface{}) error {
	return nil
}

func (f *FederationHandler) GetMembersStatus(featureState api.Feature) (map[string]string, error) {
	statusMap := make(map[string]string, 0)
	for _, memberCluster := range featureState.ClusterGroup.MemberClusters {
		statusMap[memberCluster.GetName()] = "ready"
	}
	return statusMap, nil
}
