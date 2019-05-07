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

	"github.com/banzaicloud/pipeline/internal/clustergroup/deployment"
	"github.com/goph/emperror"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/banzaicloud/pipeline/internal/clustergroup/api"
)

// Manager
type Manager struct {
	clusterGetter     api.ClusterGetter
	cgRepo            *ClusterGroupRepository
	logger            logrus.FieldLogger
	errorHandler      emperror.Handler
	featureHandlerMap map[string]api.FeatureHandler
}

// NewManager returns a new Manager instance.
func NewManager(
	clusterGetter api.ClusterGetter,
	repository *ClusterGroupRepository,
	logger logrus.FieldLogger,
	errorHandler emperror.Handler,
) *Manager {
	featureHandlerMap := make(map[string]api.FeatureHandler, 0)
	return &Manager{
		clusterGetter:     clusterGetter,
		cgRepo:            repository,
		logger:            logger,
		errorHandler:      errorHandler,
		featureHandlerMap: featureHandlerMap,
	}
}

// CreateClusterGroup creates a cluster group
func (g *Manager) CreateClusterGroup(ctx context.Context, name string, orgID uint, members []uint) (*uint, error) {
	cgModel, err := g.cgRepo.FindOne(ClusterGroupModel{
		OrganizationID: orgID,
		Name:           name,
	})
	if err != nil {
		if !IsClusterGroupNotFoundError(err) {
			return nil, err
		}
	}
	if cgModel != nil {
		return nil, errors.WithStack(&clusterGroupAlreadyExistsError{
			clusterGroup: *cgModel,
		})
	}

	memberClusterModels := make([]MemberClusterModel, 0)
	for _, clusterID := range members {
		var cluster api.Cluster
		cluster, err := g.clusterGetter.GetClusterByID(ctx, orgID, clusterID)
		if err != nil {
			return nil, errors.WithStack(&memberClusterNotFoundError{
				orgID:     orgID,
				clusterID: clusterID,
			})
		}
		if ok, err := g.isClusterMemberOfAClusterGroup(cluster.GetID(), 0); ok {
			return nil, errors.WithStack(&memberClusterPartOfAClusterGroupError{
				orgID:     orgID,
				clusterID: clusterID,
			})
		} else if err != nil {
			return nil, errors.WithStack(err)
		}
		clusterIsReady, err := cluster.IsReady()
		if err == nil && clusterIsReady {
			memberClusterModels = append(memberClusterModels, MemberClusterModel{
				ClusterID: cluster.GetID(),
			})
			g.logger.WithFields(logrus.Fields{
				"clusterName":      cluster.GetName(),
				"clusterGroupName": name,
			}).Info("Join cluster to group")
		}
	}
	if len(memberClusterModels) == 0 {
		return nil, errors.WithStack(&noReadyMembersError{
			clusterGroup: *cgModel,
		})
	}

	cgId, err := g.cgRepo.Create(name, orgID, memberClusterModels)
	if err != nil {
		return nil, err
	}

	// enable DeploymentFeature by default on every cluster group
	deploymentFeature := &ClusterGroupFeatureModel{
		Enabled:        true,
		Name:           deployment.FeatureName,
		ClusterGroupID: *cgId,
	}
	err = g.cgRepo.SaveFeature(deploymentFeature)
	if err != nil {
		return nil, err
	}
	return cgId, nil

}

// UpdateClusterGroup updates a cluster group
func (g *Manager) UpdateClusterGroup(ctx context.Context, orgID uint, clusterGroupId uint, name string, members []uint) error {
	cgModel, err := g.cgRepo.FindOne(ClusterGroupModel{
		ID: clusterGroupId,
	})
	if err != nil {
		return err
	}

	existingClusterGroup := g.GetClusterGroupFromModel(ctx, cgModel, false)
	newMembers := make(map[uint]api.Cluster, 0)

	for _, clusterID := range members {
		var cluster api.Cluster
		cluster, err = g.clusterGetter.GetClusterByID(ctx, orgID, clusterID)
		if err != nil {
			return errors.WithStack(&memberClusterNotFoundError{
				orgID:     orgID,
				clusterID: clusterID,
			})
		}
		if ok, err := g.isClusterMemberOfAClusterGroup(cluster.GetID(), existingClusterGroup.Id); ok {
			return errors.WithStack(&memberClusterPartOfAClusterGroupError{
				orgID:     orgID,
				clusterID: clusterID,
			})
		} else if err != nil {
			return errors.WithStack(err)
		}
		clusterIsReady, err := cluster.IsReady()
		if err != nil {
			return emperror.WrapWith(err, "could not check cluster readiness", "clusterID", cluster.GetID())
		}
		if !clusterIsReady {
			return emperror.WrapWith(errors.New("cluster is not ready"), "could not join cluster to group", "clusterID", cluster.GetID(), "clusterGroupName", existingClusterGroup.Name, "clusterName", cluster.GetName())
		}
		if err == nil && clusterIsReady {
			g.logger.WithFields(logrus.Fields{
				"clusterName":      cluster.GetName(),
				"clusterGroupName": existingClusterGroup.Name,
			}).Info("Join cluster to group")
			newMembers[cluster.GetID()] = cluster
		}
	}

	err = g.validateBeforeClusterGroupUpdate(*existingClusterGroup, newMembers)
	if err != nil {
		return emperror.Wrap(err, "update denied")
	}

	err = g.cgRepo.UpdateMembers(existingClusterGroup, name, newMembers)
	if err != nil {
		return err
	}

	// call feature handlers on members update
	err = g.ReconcileFeatures(*existingClusterGroup, true)
	if err != nil {
		return err
	}

	return nil
}

// DeleteClusterGroup deletes a cluster group by id
func (g *Manager) DeleteClusterGroupByID(ctx context.Context, clusterGroupId uint) error {
	cgModel, err := g.cgRepo.FindOne(ClusterGroupModel{
		ID: clusterGroupId,
	})
	if err != nil {
		return err
	}
	cgroup := g.GetClusterGroupFromModel(ctx, cgModel, false)

	// call feature handlers
	err = g.DisableFeatures(*cgroup)
	if err != nil {
		return err
	}

	return g.cgRepo.Delete(cgModel)
}

// GetClusterGroupFromModel converts a ClusterGroupModel to api.ClusterGroup
func (g *Manager) GetClusterGroupFromModel(ctx context.Context, cg *ClusterGroupModel, withStatus bool) *api.ClusterGroup {
	var clusterGroup api.ClusterGroup
	clusterGroup.Name = cg.Name
	clusterGroup.Id = cg.ID
	clusterGroup.UID = cg.UID
	clusterGroup.OrganizationID = cg.OrganizationID
	clusterGroup.Members = make([]api.Member, 0)
	clusterGroup.Clusters = make(map[uint]api.Cluster, 0)

	enabledFeatures := make([]string, 0)
	for _, feature := range cg.FeatureParams {
		if feature.Enabled {
			enabledFeatures = append(enabledFeatures, feature.Name)
		}
	}
	clusterGroup.EnabledFeatures = enabledFeatures

	for _, m := range cg.Members {
		cluster, err := g.clusterGetter.GetClusterByIDOnly(ctx, m.ClusterID)
		if err != nil {
			clusterGroup.Members = append(clusterGroup.Members, api.Member{
				ID:     m.ClusterID,
				Status: "cluster not found",
			})
			continue
		}
		member := api.Member{
			ID:           cluster.GetID(),
			Cloud:        cluster.GetCloud(),
			Distribution: cluster.GetDistribution(),
			Name:         cluster.GetName(),
		}
		if withStatus {
			clusterStatus, err := cluster.GetStatus()
			if err != nil {
				member.Status = err.Error()
			} else {
				member.Status = clusterStatus.Status
			}
		}
		clusterGroup.Members = append(clusterGroup.Members, member)
		clusterGroup.Clusters[cluster.GetID()] = cluster
	}

	return &clusterGroup
}

// GetClusterGroupByID gets a cluster group by id
func (g *Manager) GetClusterGroupByID(ctx context.Context, clusterGroupId uint) (*api.ClusterGroup, error) {
	return g.GetClusterGroupByIDWithStatus(ctx, clusterGroupId, false)
}

// GetClusterGroupByIDWithStatus gets a cluster group by id - optionally with a status info
func (g *Manager) GetClusterGroupByIDWithStatus(ctx context.Context, clusterGroupId uint, withStatus bool) (*api.ClusterGroup, error) {
	cgModel, err := g.cgRepo.FindOne(ClusterGroupModel{
		ID: clusterGroupId,
	})
	if err != nil {
		return nil, err
	}
	return g.GetClusterGroupFromModel(ctx, cgModel, withStatus), nil
}

// GetAllClusterGroups returns every cluster groups
func (g *Manager) GetAllClusterGroups(ctx context.Context) ([]api.ClusterGroup, error) {
	groups := make([]api.ClusterGroup, 0)

	clusterGroups, err := g.cgRepo.FindAll()
	if err != nil {
		return nil, err
	}
	for _, cgModel := range clusterGroups {
		cg := g.GetClusterGroupFromModel(ctx, cgModel, false)
		groups = append(groups, *cg)
	}

	return groups, nil
}

func (g *Manager) isClusterMemberOfAClusterGroup(clusterID uint, clusterGroupId uint) (bool, error) {
	result, err := g.cgRepo.FindMemberClusterByID(clusterID)
	if IsRecordNotFoundError(err) {
		return false, nil
	}

	if err != nil {
		return true, err
	}

	if clusterGroupId > 0 && result.ClusterGroupID == clusterGroupId {
		return false, nil
	}

	return true, nil
}

func (g *Manager) validateBeforeClusterGroupUpdate(clusterGroup api.ClusterGroup, newClusters map[uint]api.Cluster) error {
	g.logger.WithField("clusterGroupName", clusterGroup.Name).Debug("validate group members before update")

	features, err := g.GetFeatures(clusterGroup)
	if err != nil {
		return err
	}

	members := make([]api.Member, 0)
	for _, cluster := range newClusters {
		member := api.Member{
			ID:           cluster.GetID(),
			Cloud:        cluster.GetCloud(),
			Distribution: cluster.GetDistribution(),
			Name:         cluster.GetName(),
		}
		members = append(members, member)
	}

	for name, feature := range features {
		if !feature.Enabled {
			continue
		}

		clusterGroup.Clusters = newClusters
		clusterGroup.Members = members
		feature.ClusterGroup = clusterGroup

		handler, err := g.GetFeatureHandler(name)
		if err != nil {
			return err
		}
		err = handler.ValidateState(feature)
		if err != nil {
			return &clusterGroupUpdateRejectedError{
				featureName: feature.Name,
			}
		}
	}

	return nil
}
