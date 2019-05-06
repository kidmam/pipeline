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
	"fmt"
	"strconv"

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

func (g *Manager) CreateClusterGroup(ctx context.Context, name string, orgID uint, members []string) (*uint, error) {
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
	for _, clusterName := range members {
		var cluster api.Cluster
		err = nil
		if clusterID, err := strconv.ParseUint(clusterName, 10, 64); err == nil {
			cluster, err = g.clusterGetter.GetClusterByID(ctx, orgID, uint(clusterID))
			if err == nil {
				clusterName = cluster.GetName()
			}
		}
		if cluster == nil {
			cluster, err = g.clusterGetter.GetClusterByName(ctx, orgID, clusterName)
		}
		if err != nil {
			return nil, errors.WithStack(&memberClusterNotFoundError{
				orgID:       orgID,
				clusterName: clusterName,
			})
		}
		if ok, err := g.isClusterMemberOfAClusterGroup(cluster.GetID(), 0); ok {
			return nil, errors.WithStack(&memberClusterPartOfAClusterGroupError{
				orgID:       orgID,
				clusterName: clusterName,
			})
		} else if err != nil {
			return nil, errors.WithStack(err)
		}
		clusterIsReady, err := cluster.IsReady()
		if err == nil && clusterIsReady {
			g.logger.Infof(clusterName)
			memberClusterModels = append(memberClusterModels, MemberClusterModel{
				ClusterID: cluster.GetID(),
			})
			g.logger.Infof("Join cluster %s to group: %s", clusterName, name)
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
		Name:           DeploymentFeatureName,
		ClusterGroupID: *cgId,
	}
	err = g.cgRepo.SaveFeature(deploymentFeature)
	if err != nil {
		return nil, err
	}
	return cgId, nil

}

func (g *Manager) UpdateClusterGroup(ctx context.Context, orgID uint, clusterGroupId uint, name string, members []string) error {
	cgModel, err := g.cgRepo.FindOne(ClusterGroupModel{
		ID: clusterGroupId,
	})
	if err != nil {
		return err
	}

	existingClusterGroup := g.GetClusterGroupFromModel(ctx, cgModel, false)
	newMembers := make(map[uint]api.Cluster, 0)

	for _, clusterName := range members {
		var cluster api.Cluster
		err = nil
		if clusterID, err := strconv.ParseUint(clusterName, 10, 64); err == nil {
			cluster, err = g.clusterGetter.GetClusterByID(ctx, orgID, uint(clusterID))
			if err == nil {
				clusterName = cluster.GetName()
			}
		}
		if cluster == nil {
			cluster, err = g.clusterGetter.GetClusterByName(ctx, orgID, clusterName)
		}
		if err != nil {
			return errors.WithStack(&memberClusterNotFoundError{
				orgID:       orgID,
				clusterName: clusterName,
			})
		}
		if ok, err := g.isClusterMemberOfAClusterGroup(cluster.GetID(), existingClusterGroup.Id); ok {
			return errors.WithStack(&memberClusterPartOfAClusterGroupError{
				orgID:       orgID,
				clusterName: clusterName,
			})
		} else if err != nil {
			return errors.WithStack(err)
		}
		clusterIsReady, err := cluster.IsReady()
		if err == nil && clusterIsReady {
			g.logger.Infof("Join cluster %s to group: %s", clusterName, existingClusterGroup.Name)
			newMembers[cluster.GetID()] = cluster
		} else {
			g.logger.Infof("Can't join cluster %s to group: %s as it not ready!", clusterName, existingClusterGroup.Name)
		}
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

func (g *Manager) DeleteClusterGroup(ctx context.Context, clusterGroupId uint) error {
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

func (g *Manager) GetClusterGroupFromModel(ctx context.Context, cg *ClusterGroupModel, withStatus bool) *api.ClusterGroup {
	var clusterGroup api.ClusterGroup
	clusterGroup.Name = cg.Name
	clusterGroup.Id = cg.ID
	clusterGroup.UID = cg.UID
	clusterGroup.OrganizationID = cg.OrganizationID
	clusterGroup.Members = make([]api.MemberCluster, 0)
	clusterGroup.MemberClusters = make(map[string]api.Cluster, 0)

	for _, m := range cg.Members {
		cluster, err := g.clusterGetter.GetClusterByIDOnly(ctx, m.ClusterID)
		if err != nil {
			clusterGroup.Members = append(clusterGroup.Members, api.MemberCluster{
				Name:   fmt.Sprintf("clusterID: %v", m.ClusterID),
				Status: "cluster not found",
			})
			continue
		}
		memberCluster := api.MemberCluster{
			ID:   cluster.GetID(),
			Name: cluster.GetName(),
		}
		if withStatus {
			clusterStatus, err := cluster.GetStatus()
			if err != nil {
				memberCluster.Status = err.Error()
			} else {
				memberCluster.Status = clusterStatus.Status
			}
		}

		clusterGroup.Members = append(clusterGroup.Members, memberCluster)
		clusterGroup.MemberClusters[cluster.GetName()] = cluster
	}

	return &clusterGroup
}

func (g *Manager) GetClusterGroupById(ctx context.Context, clusterGroupId uint) (*api.ClusterGroup, error) {
	return g.GetClusterGroupByIdWithStatus(ctx, clusterGroupId, false)
}

func (g *Manager) GetClusterGroupByIdWithStatus(ctx context.Context, clusterGroupId uint, withStatus bool) (*api.ClusterGroup, error) {
	cgModel, err := g.cgRepo.FindOne(ClusterGroupModel{
		ID: clusterGroupId,
	})
	if err != nil {
		return nil, err
	}
	return g.GetClusterGroupFromModel(ctx, cgModel, withStatus), nil
}

func (g *Manager) GetClusterGroupByName(ctx context.Context, orgId uint, clusterGroupName string) (*api.ClusterGroup, error) {
	cgModel, err := g.cgRepo.FindOne(ClusterGroupModel{
		OrganizationID: orgId,
		Name:           clusterGroupName,
	})
	if err != nil {
		return nil, err
	}
	return g.GetClusterGroupFromModel(ctx, cgModel, false), nil
}

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
