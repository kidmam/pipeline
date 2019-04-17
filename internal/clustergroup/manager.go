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

	"github.com/banzaicloud/pipeline/cluster"
	"github.com/banzaicloud/pipeline/pkg/clustergroup"
	"github.com/goph/emperror"
	"github.com/jinzhu/gorm"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const FeatureEnabled = "enabled"

type ClusterGetter interface {
	GetClusterByIDOnly(ctx context.Context, clusterID uint) (cluster.CommonCluster, error)
}

// Manager
type Manager struct {
	clusterGetter ClusterGetter
	db            *gorm.DB
	logger        logrus.FieldLogger
	errorHandler  emperror.Handler
}

// NewManager returns a new Manager instance.
func NewManager(
	clusterGetter ClusterGetter,
	db *gorm.DB,
	logger logrus.FieldLogger,
	errorHandler emperror.Handler,
) *Manager {
	return &Manager{
		clusterGetter: clusterGetter,
		db:            db,
		logger:        logger,
		errorHandler:  errorHandler,
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
		return nil, errors.WithStack(errors.New("cluster group not found"))
	}
	if err != nil {
		return nil, emperror.With(err,
			"clusterGroupId", cg.ID,
			"organizationID", cg.OrganizationID,
		)
	}

	return &result, nil
}

// GetFeature returns params of a cluster group feature by clusterGroupId and feature name
func (g *Manager) GetFeature(clusterGroup clustergroup.ClusterGroup, featureName string) (*ClusterGroupFeature, error) {
	if clusterGroup.Id == 0 {
		return nil, errors.New("missing parameter: clusterGroupId")
	}
	var results []ClusterGroupFeatureParamModel
	err := g.db.Find(&results, ClusterGroupFeatureParamModel{
		ClusterGroupID: clusterGroup.Id,
		FeatureName: featureName,
	}).Error
	if gorm.IsRecordNotFoundError(err) {
		return nil, errors.WithStack(errors.New("cluster group not found"))
	}
	if err != nil {
		return nil, emperror.With(err,
			"clusterGroupId", clusterGroup.Id,
		)
	}

	params := make(map[string]string, 0)
	for _, r:= range results {
		params[r.ParamName] = r.ParamValue
	}
	feature := &ClusterGroupFeature{
		ClusterGroup: clusterGroup,
		Params: params,
		Name: featureName,
	}

	if params[FeatureEnabled] == "true" {
		feature.Enabled = true
	}
	return feature, nil
}

func getFeatureHandler(featureName string, clusterGroup clustergroup.ClusterGroup, params map[string]string) ClusterGroupFeatureHandler {
	switch featureName {
	case "federation":
		return &FederationClusterGroupFeature{
			ClusterGroupFeature: ClusterGroupFeature{
				Name: featureName,
				ClusterGroup: clusterGroup,
				Params: params,
			},
		}
	case "service-mesh":
		return &ServiceMeshClusterGroupFeature{
			ClusterGroupFeature: ClusterGroupFeature{
				Name: featureName,
				ClusterGroup: clusterGroup,
				Params: params,
			},
		}
	}
	return nil
}


// GetEnabledFeatures
func (g *Manager) GetEnabledFeatureHandlers(clusterGroup clustergroup.ClusterGroup) (map[string]ClusterGroupFeatureHandler, error) {
	if clusterGroup.Id == 0 {
		return nil, errors.New("missing parameter: clusterGroupId")
	}
	var results []ClusterGroupFeatureParamModel
	err := g.db.Find(&results, ClusterGroupFeatureParamModel{
		ClusterGroupID: clusterGroup.Id,
	}).Error
	if gorm.IsRecordNotFoundError(err) {
		return nil, errors.WithStack(errors.New("cluster group not found"))
	}
	if err != nil {
		return nil, emperror.With(err,
			"clusterGroupId", clusterGroup.Id,
		)
	}

	features := make(map[string]ClusterGroupFeatureHandler, 0)
	for _, r := range results {
		cgFeature, exists := features[r.FeatureName]
		if !exists {
			params := make(map[string]string, 0)
			cgFeature := getFeatureHandler(r.FeatureName, clusterGroup, params)
			features[r.FeatureName] = cgFeature
		}
		if r.ParamName == FeatureEnabled {
			cgFeature.SetEnabled(true)
		} else {
			cgFeature.SetParam(r.ParamName, r.ParamValue)
		}
	}

	return features, nil
}

// SetFeatureParams sets params of a cluster group feature
func (g *Manager) SetFeatureParams(feature ClusterGroupFeature, enabled bool, params map[string]string) error {

	var results []ClusterGroupFeatureParamModel
	err := g.db.Find(&results, ClusterGroupFeatureParamModel{
		ClusterGroupID: feature.ClusterGroup.Id,
		FeatureName: feature.Name,
	}).Error
	if gorm.IsRecordNotFoundError(err) {
		return errors.WithStack(errors.New("cluster group not found"))
	}
	if err != nil {
		return emperror.With(err,
			"clusterGroupId", feature.ClusterGroup.Id,
		)
	}

	if enabled {
		params[FeatureEnabled] = "true"
	}

	paramsToCreateUpdate := make([]ClusterGroupFeatureParamModel, 0)
	paramsToDelete := make([]ClusterGroupFeatureParamModel, 0)
	for _, r := range results {
		value, paramExists := params[r.ParamName]
		if !paramExists {
			paramsToDelete = append(paramsToDelete, r)
		}
		if value != r.ParamValue {
			paramsToCreateUpdate = append(paramsToCreateUpdate, r)
		}
		delete(params, r.ParamName)
	}

	// add new params to paramsToCreateUpdate
	for k, v := range params {
		paramsToCreateUpdate = append(paramsToCreateUpdate, ClusterGroupFeatureParamModel{
			ClusterGroupID: feature.ClusterGroup.Id,
			FeatureName: feature.Name,
			ParamName: k,
			ParamValue: v,
		})
	}

	//tx := g.db.Begin()
	//if tx.Error != nil {
	//	return emperror.Wrap(err, "Error saving feature params")
	//}
	for _, r := range paramsToCreateUpdate {
		err = g.db.Save(&r).Error
		if err != nil {
			//rollbackErr := tx.Rollback().Error
			//if rollbackErr != nil {
			//	return emperror.Wrapf(err, "Error rollback saving feature params: %s", rollbackErr.Error())
			//}
			return emperror.Wrap(err, "Error saving feature params")
		}
	}
	for _, r := range paramsToDelete {
		err = g.db.Delete(&r).Error
		if err != nil {
			//rollbackErr := tx.Rollback().Error
			//if rollbackErr != nil {
			//	return emperror.Wrapf(err, "Error rollback deleting feature params: %s", rollbackErr.Error())
			//}
			return emperror.Wrap(err, "Error deleting feature params")
		}
	}
	//err = tx.Commit().Error
	//if err != nil {
	//	return emperror.Wrap(err, "Error saving feature params")
	//}
	return nil
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
	var response clustergroup.ClusterGroup
	response.Name = cg.Name
	response.Id = cg.ID
	response.UID = cg.UID
	if withStatus {
		response.MembersStatus = make([]clustergroup.MemberClusterStatus, 0)
	} else {
		response.Members = make([]string, 0)
	}
	response.MemberClusters = make(map[string]cluster.CommonCluster, 0)
	for _, m := range cg.Members {
		cluster, err := g.clusterGetter.GetClusterByIDOnly(ctx, m.ClusterID)
		if withStatus {
			if err != nil {
				response.MembersStatus = append(response.MembersStatus, clustergroup.MemberClusterStatus{
					Name:   cluster.GetName(),
					Status: err.Error(),
				})
			} else {
				clusterStatus, err := cluster.GetStatus()
				if err != nil {
					response.MembersStatus = append(response.MembersStatus, clustergroup.MemberClusterStatus{
						Name:   cluster.GetName(),
						Status: err.Error(),
					})
				} else {
					response.MembersStatus = append(response.MembersStatus, clustergroup.MemberClusterStatus{
						Name:   cluster.GetName(),
						Status: clusterStatus.Status,
					})
				}
			}
		} else {
			response.Members = append(response.Members, cluster.GetName())
		}
		response.MemberClusters[cluster.GetName()] = cluster
	}

	return &response
}
