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

	"github.com/goph/emperror"

	"github.com/banzaicloud/pipeline/internal/clustergroup/api"
)

func (g *Manager) RegisterFeatureHandler(featureName string, handler api.FeatureHandler) {
	g.featureHandlerMap[featureName] = handler
}

func (g *Manager) GetFeatureHandler(featureName string) (api.FeatureHandler, error) {
	handler := g.featureHandlerMap[featureName]
	if handler == nil {
		return nil, &unknownFeature{
			name: featureName,
		}
	}

	return handler, nil
}

func (g *Manager) GetFeatureStatus(feature api.Feature) (map[uint]string, error) {
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
	g.logger.WithField("clusterGroupName", clusterGroup.Name).Debug("disable all enabled features")

	features, err := g.GetFeatures(clusterGroup)
	if err != nil {
		return err
	}

	for name, feature := range features {
		if feature.Enabled {
			handler, err := g.GetFeatureHandler(name)
			if err != nil {
				return err
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
		if IsRecordNotFoundError(err) {
			return features, nil
		}
		return nil, emperror.With(err,
			"clusterGroupId", clusterGroup.Id,
		)
	}

	for _, r := range results {
		var featureProperties interface{}
		if r.Properties != nil {
			err := json.Unmarshal(r.Properties, &featureProperties)
			if err != nil {
				g.errorHandler.Handle(err)
			}
		}
		features[r.Name] = api.Feature{
			Name:         r.Name,
			Enabled:      r.Enabled,
			ClusterGroup: clusterGroup,
			Properties:   featureProperties,
		}
	}

	return features, nil
}

// GetFeature returns params of a cluster group feature by clusterGroupId and feature name
func (g *Manager) GetFeature(clusterGroup api.ClusterGroup, featureName string) (*api.Feature, error) {
	result, err := g.cgRepo.GetFeature(clusterGroup.Id, featureName)
	if err != nil {
		return nil, emperror.With(err,
			"clusterGroupId", clusterGroup.Id,
			"featureName", featureName,
		)
	}

	var featureProperties interface{}
	err = json.Unmarshal(result.Properties, &featureProperties)
	if err != nil {
		return nil, emperror.Wrap(err, "could not unmarshal feature properties")
	}
	feature := &api.Feature{
		ClusterGroup: clusterGroup,
		Properties:   featureProperties,
		Name:         featureName,
		Enabled:      result.Enabled,
	}

	return feature, nil
}

// DisableFeature disable a cluster group feature
func (g *Manager) DisableFeature(featureName string, clusterGroup *api.ClusterGroup) error {
	err := g.disableFeature(featureName, clusterGroup)
	if err != nil {
		return emperror.Wrap(err, "could not disable feature")
	}

	return nil
}

func (g *Manager) disableFeature(featureName string, clusterGroup *api.ClusterGroup) error {
	_, err := g.GetFeatureHandler(featureName)
	if err != nil {
		return err
	}

	result, err := g.cgRepo.GetFeature(clusterGroup.Id, featureName)
	if err != nil {
		return emperror.With(err,
			"clusterGroupId", clusterGroup.Id,
			"featureName", featureName,
		)
	}

	result.Enabled = false
	err = g.cgRepo.SaveFeature(result)
	if err != nil {
		return emperror.Wrap(err, "could not save feature")
	}

	return nil
}

func (g *Manager) EnableFeature(featureName string, clusterGroup *api.ClusterGroup, properties interface{}) error {
	err := g.setFeatureParams(featureName, clusterGroup, properties)
	if err != nil {
		return emperror.Wrap(err, "could not enable feature")
	}

	return nil
}

func (g *Manager) UpdateFeature(featureName string, clusterGroup *api.ClusterGroup, properties interface{}) error {
	err := g.setFeatureParams(featureName, clusterGroup, properties)
	if err != nil {
		return emperror.Wrap(err, "could not update feature")
	}

	return nil
}

// SetFeatureParams sets params of a cluster group feature
func (g *Manager) setFeatureParams(featureName string, clusterGroup *api.ClusterGroup, properties interface{}) error {
	handler, err := g.GetFeatureHandler(featureName)
	if err != nil {
		return emperror.Wrap(err, "could not get feature handler")
	}

	err = handler.ValidateProperties(*clusterGroup, properties)
	if err != nil {
		return emperror.Wrap(err, "invalid properties")
	}

	result, err := g.cgRepo.GetFeature(clusterGroup.Id, featureName)
	if IsFeatureRecordNotFoundError(err) {
		result = &ClusterGroupFeatureModel{
			Name:           featureName,
			ClusterGroupID: clusterGroup.Id,
		}
	} else if err != nil {
		return emperror.With(err,
			"clusterGroupId", clusterGroup.Id,
			"featureName", featureName,
		)
	}

	result.Enabled = true
	result.Properties, err = json.Marshal(properties)
	if err != nil {
		return emperror.Wrap(err, "could not marshal feature properties")
	}

	err = g.cgRepo.SaveFeature(result)
	if err != nil {
		return emperror.Wrap(err, "could not save feature")
	}

	return nil
}
