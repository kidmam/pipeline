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
	"encoding/json"

	"github.com/banzaicloud/pipeline/internal/clustergroup/api"
	"github.com/goph/emperror"
	"github.com/jinzhu/gorm"
	"github.com/pkg/errors"
)

type FeatureHandler interface {
	ReconcileState(featureState api.Feature) error
	GetMembersStatus(featureState api.Feature) (map[string]string, error)
}

func (g *Manager) RegisterFeatureHandler(featureName string, handler FeatureHandler) {
	g.featureHandlerMap[featureName] = handler
}

func (g *Manager) GetFeatureStatus(feature api.Feature) (map[string]string, error) {
	handler, ok := g.featureHandlerMap[feature.Name]
	if !ok {
		return nil, nil
	}
	return handler.GetMembersStatus(feature)
}

func (g *Manager) GetEnabledFeatures(clusterGroup api.ClusterGroup) (map[string]api.Feature, error) {
	enabledFeatures := make(map[string]api.Feature, 0)

	features, err := g.GetFeatures(clusterGroup)
	if err != nil {
		return nil, err
	}

	for name, feature := range features {
		if feature.Enabled {
			enabledFeatures[name] = feature
		}
	}

	return enabledFeatures, nil
}

func (g *Manager) ReconcileFeatures(clusterGroup api.ClusterGroup, onlyEnabledHandlers bool) error {
	g.logger.Debugf("reconcile features for group: %s", clusterGroup.Name)

	features, err := g.GetFeatures(clusterGroup)
	if err != nil {
		return err
	}

	for name, feature := range features {
		if feature.Enabled || !onlyEnabledHandlers {
			handler := g.featureHandlerMap[name]
			if handler == nil {
				g.logger.Debugf("no handler registered for cluster group feature %s", name)
				continue
			}
			handler.ReconcileState(feature)
		}
	}

	return nil
}

func (g *Manager) DisableFeatures(clusterGroup api.ClusterGroup) error {
	g.logger.Debugf("disable features for group: %s", clusterGroup.Name)

	features, err := g.GetFeatures(clusterGroup)
	if err != nil {
		return err
	}

	for name, feature := range features {
		if feature.Enabled {
			handler := g.featureHandlerMap[name]
			if handler == nil {
				g.logger.Debugf("no handler registered for cluster group feature %s", name)
				continue
			}
			feature.Enabled = false
			handler.ReconcileState(feature)
		}
	}

	return nil
}

func (g *Manager) GetFeatures(clusterGroup api.ClusterGroup) (map[string]api.Feature, error) {
	features := make(map[string]api.Feature, 0)

	results, err := g.cgRepo.GetAllFeatures(clusterGroup.Id)
	if err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return features, nil
		}
		return nil, emperror.With(err,
			"clusterGroupId", clusterGroup.Id,
		)
	}

	for _, r := range results {
		var featureProperties interface{}
		json.Unmarshal(r.Properties, featureProperties)
		cgFeature := api.Feature{
			Name:         r.Name,
			Enabled:      r.Enabled,
			ClusterGroup: clusterGroup,
			Properties:   featureProperties,
		}
		features[r.Name] = cgFeature
	}

	return features, nil
}

// GetFeature returns params of a cluster group feature by clusterGroupId and feature name
func (g *Manager) GetFeature(clusterGroup api.ClusterGroup, featureName string) (*api.Feature, error) {

	result, err := g.cgRepo.GetFeature(clusterGroup.Id, featureName)
	if err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return nil, errors.WithStack(errors.New("cluster group feature not found"))
		}
		return nil, emperror.With(err,
			"clusterGroupId", clusterGroup.Id,
			"featureName", featureName,
		)

	}

	var featureProperties interface{}
	json.Unmarshal(result.Properties, featureProperties)
	feature := &api.Feature{
		ClusterGroup: clusterGroup,
		Properties:   featureProperties,
		Name:         featureName,
		Enabled:      result.Enabled,
	}

	return feature, nil
}

// SetFeatureParams sets params of a cluster group feature
func (g *Manager) SetFeatureParams(featureName string, clusterGroup *api.ClusterGroup, enabled bool, properties interface{}) error {

	result, err := g.cgRepo.GetFeature(clusterGroup.Id, featureName)
	if err != nil && !gorm.IsRecordNotFoundError(err) {
		return emperror.With(err,
			"clusterGroupId", clusterGroup.Id,
			"featureName", featureName,
		)
	}

	if result == nil {
		result = &ClusterGroupFeatureModel{
			Name:           featureName,
			ClusterGroupID: clusterGroup.Id,
		}
	}

	result.Enabled = enabled
	result.Properties, err = json.Marshal(properties)
	if err != nil {
		return emperror.Wrap(err, "Error marshalling feature properties")
	}

	err = g.cgRepo.SaveFeature(result)
	if err != nil {
		return emperror.Wrap(err, "Error saving feature")
	}

	return nil
}
