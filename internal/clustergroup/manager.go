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
	"context"
	"encoding/json"

	"github.com/banzaicloud/pipeline/cluster"
	"github.com/banzaicloud/pipeline/pkg/clustergroup"
	"github.com/goph/emperror"
	"github.com/jinzhu/gorm"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type ClusterGetter interface {
	GetClusterByIDOnly(ctx context.Context, clusterID uint) (cluster.CommonCluster, error)
}

// Manager
type Manager struct {
	clusterGetter     ClusterGetter
	db                *gorm.DB
	logger            logrus.FieldLogger
	errorHandler      emperror.Handler
	featureHandlerMap map[string]ClusterGroupFeatureHandler
}

// NewManager returns a new Manager instance.
func NewManager(
	clusterGetter ClusterGetter,
	db *gorm.DB,
	logger logrus.FieldLogger,
	errorHandler emperror.Handler,
) *Manager {
	featureHandlerMap := make(map[string]ClusterGroupFeatureHandler, 0)
	return &Manager{
		clusterGetter:     clusterGetter,
		db:                db,
		logger:            logger,
		errorHandler:      errorHandler,
		featureHandlerMap: featureHandlerMap,
	}
}

// findOne returns a cluster group instance for an organization by clusterGroupId.
func (g *Manager) FindOne(cg ClusterGroupModel) (*ClusterGroupModel, error) {
	if cg.ID == 0 && len(cg.Name) == 0 {
		return nil, errors.New("either clusterGroupId or name is required")
	}
	var result ClusterGroupModel
	err := g.db.Where(cg).Preload("Members").First(&result).Error
	if gorm.IsRecordNotFoundError(err) {
		return nil, nil
	}
	if err != nil {
		return nil, emperror.With(err,
			"clusterGroupId", cg.ID,
			"organizationID", cg.OrganizationID,
		)
	}

	return &result, nil
}

// findAll returns all cluster groups
func (g *Manager) FindAll() ([]*ClusterGroupModel, error) {
	var cgroups []*ClusterGroupModel

	err := g.db.Preload("Members").Find(&cgroups).Error
	if err != nil {
		return nil, errors.Wrap(err, "could not fetch cluster groups")
	}

	return cgroups, nil
}

func (g *Manager) GetClusterGroupById(ctx context.Context, orgId uint, clusterGroupId uint) (*clustergroup.ClusterGroup, error) {
	cgModel, err := g.FindOne(ClusterGroupModel{
		OrganizationID: orgId,
		ID:             clusterGroupId,
	})
	if err != nil {
		return nil, err
	}
	return g.GetClusterGroupFromModel(ctx, cgModel, false), nil
}

func (g *Manager) RegisterFeatureHandler(featureName string, handler ClusterGroupFeatureHandler) {
	g.featureHandlerMap[featureName] = handler
}

func (g *Manager) GetFeatureStatus(feature ClusterGroupFeature) (map[string]string, error) {
	handler, ok := g.featureHandlerMap[feature.Name]
	if !ok {
		return nil, nil
	}
	return handler.GetMembersStatus(feature)
}

func (g *Manager) GetEnabledFeatures(clusterGroup clustergroup.ClusterGroup) (map[string]ClusterGroupFeature, error) {
	enabledFeatures := make(map[string]ClusterGroupFeature, 0)

	features, err := g.getFeatures(clusterGroup)
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

func (g *Manager) ReconcileFeatureHandlers(clusterGroup clustergroup.ClusterGroup) error {
	g.logger.Debugf("reconcile features for group: %s", clusterGroup.Name)

	features, err := g.getFeatures(clusterGroup)
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
			handler.ReconcileState(feature)
		}
	}

	return nil
}

func (g *Manager) getFeatures(clusterGroup clustergroup.ClusterGroup) (map[string]ClusterGroupFeature, error) {
	features := make(map[string]ClusterGroupFeature, 0)

	var results []ClusterGroupFeatureModel
	err := g.db.Find(&results, ClusterGroupFeatureModel{
		ClusterGroupID: clusterGroup.Id,
	}).Error
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
		cgFeature := ClusterGroupFeature{
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
func (g *Manager) GetFeature(clusterGroup clustergroup.ClusterGroup, featureName string) (*ClusterGroupFeature, error) {
	var result ClusterGroupFeatureModel
	err := g.db.Where(ClusterGroupFeatureModel{
		ClusterGroupID: clusterGroup.Id,
		Name:           featureName,
	}).First(&result).Error

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
	feature := &ClusterGroupFeature{
		ClusterGroup: clusterGroup,
		Properties:   featureProperties,
		Name:         featureName,
		Enabled:      result.Enabled,
	}

	return feature, nil
}

// SetFeatureParams sets params of a cluster group feature
func (g *Manager) SetFeatureParams(featureName string, clusterGroup *clustergroup.ClusterGroup, enabled bool, properties interface{}) error {

	var result ClusterGroupFeatureModel
	err := g.db.Where(ClusterGroupFeatureModel{
		ClusterGroupID: clusterGroup.Id,
		Name:           featureName,
	}).First(&result).Error

	if err != nil && !gorm.IsRecordNotFoundError(err) {
		return emperror.With(err,
			"clusterGroupId", clusterGroup.Id,
			"featureName", featureName,
		)
	}

	if result.ID == 0 {
		result.Name = featureName
		result.ClusterGroupID = clusterGroup.Id
	}

	result.Enabled = enabled
	result.Properties, err = json.Marshal(properties)
	if err != nil {
		return emperror.Wrap(err, "Error marshalling feature properties")
	}

	err = g.db.Save(&result).Error
	if err != nil {
		return emperror.Wrap(err, "Error saving feature")
	}

	return nil
}

func (g *Manager) DeleteClusterGroup(cgroup *ClusterGroupModel) error {

	for _, fp := range cgroup.FeatureParams {
		err := g.db.Delete(fp).Error
		if err != nil {
			return err
		}
	}

	for _, member := range cgroup.Members {
		err := g.db.Delete(member).Error
		if err != nil {
			return err
		}
	}

	err := g.db.Delete(cgroup).Error
	if err != nil {
		return err
	}

	return nil
}

func (g *Manager) GetClusterGroupFromModel(ctx context.Context, cg *ClusterGroupModel, withStatus bool) *clustergroup.ClusterGroup {
	var clusterGroup clustergroup.ClusterGroup
	clusterGroup.Name = cg.Name
	clusterGroup.Id = cg.ID
	clusterGroup.UID = cg.UID
	clusterGroup.OrganizationID = cg.OrganizationID
	if withStatus {
		clusterGroup.MembersStatus = make([]clustergroup.MemberClusterStatus, 0)
	} else {
		clusterGroup.Members = make([]string, 0)
	}
	clusterGroup.MemberClusters = make(map[string]cluster.CommonCluster, 0)
	for _, m := range cg.Members {
		cluster, err := g.clusterGetter.GetClusterByIDOnly(ctx, m.ClusterID)
		if withStatus {
			if err != nil {
				clusterGroup.MembersStatus = append(clusterGroup.MembersStatus, clustergroup.MemberClusterStatus{
					Name:   cluster.GetName(),
					Status: err.Error(),
				})
			} else {
				clusterStatus, err := cluster.GetStatus()
				if err != nil {
					clusterGroup.MembersStatus = append(clusterGroup.MembersStatus, clustergroup.MemberClusterStatus{
						Name:   cluster.GetName(),
						Status: err.Error(),
					})
				} else {
					clusterGroup.MembersStatus = append(clusterGroup.MembersStatus, clustergroup.MemberClusterStatus{
						Name:   cluster.GetName(),
						Status: clusterStatus.Status,
					})
				}
			}
		} else {
			clusterGroup.Members = append(clusterGroup.Members, cluster.GetName())
		}
		clusterGroup.MemberClusters[cluster.GetName()] = cluster
	}

	return &clusterGroup
}

func (g *Manager) CreateClusterGroup(name string, orgID uint, memberClusterModels []MemberClusterModel) (*uint, error) {
	clusterGroupModel := &ClusterGroupModel{
		Name:           name,
		OrganizationID: orgID,
		Members:        memberClusterModels,
	}

	err := g.db.Save(clusterGroupModel).Error
	if err != nil {
		return nil, err
	}
	return &clusterGroupModel.ID, nil
}

func (g *Manager) UpdateClusterGroup(ctx context.Context, cgroup *clustergroup.ClusterGroup, name string, newMembers map[uint]cluster.CommonCluster) (*clustergroup.ClusterGroup, error) {
	cgModel, err := g.FindOne(ClusterGroupModel{
		ID: cgroup.Id,
	})
	if err != nil {
		return nil, err
	}
	updatedMembers := make([]MemberClusterModel, 0)

	for _, member := range cgModel.Members {
		if _, ok := newMembers[member.ClusterID]; !ok {
			err = g.db.Delete(member).Error
			if err != nil {
				return nil, err
			}
		} else {
			updatedMembers = append(updatedMembers, member)
		}
	}
	//TODO add new

	cgModel.Members = updatedMembers
	err = g.db.Save(cgModel).Error
	if err != nil {
		return nil, err
	}
	return g.GetClusterGroupFromModel(ctx, cgModel, false), nil
}
