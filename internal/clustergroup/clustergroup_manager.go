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

	"github.com/banzaicloud/pipeline/cluster"
	"github.com/banzaicloud/pipeline/pkg/clustergroup"
	"github.com/goph/emperror"
	"github.com/jinzhu/gorm"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type ClusterGetter interface {
	GetClusterByIDOnly(ctx context.Context, clusterID uint) (cluster.CommonCluster, error)
	GetClusterByName(ctx context.Context, organizationID uint, clusterName string) (cluster.CommonCluster, error)
}

// Manager
type Manager struct {
	clusterGetter     ClusterGetter
	cgRepo            *ClusterGroupRepository
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
		cgRepo:            NewClusterGroupRepository(db, logger),
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
		cluster, err := g.clusterGetter.GetClusterByName(ctx, orgID, clusterName)
		if err != nil {
			return nil, errors.WithStack(&memberClusterNotFoundError{
				orgID:       orgID,
				clusterName: clusterName,
			})
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
		Enabled:		true,
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
	newMembers := make(map[uint]cluster.CommonCluster, 0)

	for _, clusterName := range members {
		cluster, err := g.clusterGetter.GetClusterByName(ctx, orgID, clusterName)
		if err != nil {
			return errors.WithStack(&memberClusterNotFoundError{
				orgID:       orgID,
				clusterName: clusterName,
			})
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
	err = g.ReconcileFeatures(*existingClusterGroup,true)
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
		if err != nil {
			clusterGroup.MembersStatus = append(clusterGroup.MembersStatus, clustergroup.MemberClusterStatus{
				Name:   fmt.Sprintf("clusterID: %v", m.ClusterID),
				Status: "cluster not found",
			})
			continue
		}
		clusterGroup.Members = append(clusterGroup.Members, cluster.GetName())
		clusterGroup.MemberClusters[cluster.GetName()] = cluster

		if withStatus {
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

	}

	return &clusterGroup
}

func (g *Manager) GetClusterGroupById(ctx context.Context, clusterGroupId uint) (*clustergroup.ClusterGroup, error) {
	return g.GetClusterGroupByIdWithStatus(ctx, clusterGroupId, false)
}

func (g *Manager) GetClusterGroupByIdWithStatus(ctx context.Context, clusterGroupId uint, withStatus bool) (*clustergroup.ClusterGroup, error) {
	cgModel, err := g.cgRepo.FindOne(ClusterGroupModel{
		ID: clusterGroupId,
	})
	if err != nil {
		return nil, err
	}
	return g.GetClusterGroupFromModel(ctx, cgModel, withStatus), nil
}

func (g *Manager) GetClusterGroupByName(ctx context.Context, orgId uint, clusterGroupName string) (*clustergroup.ClusterGroup, error) {
	cgModel, err := g.cgRepo.FindOne(ClusterGroupModel{
		OrganizationID: orgId,
		Name:           clusterGroupName,
	})
	if err != nil {
		return nil, err
	}
	return g.GetClusterGroupFromModel(ctx, cgModel, false), nil
}

func (g *Manager) GetAllClusterGroups(ctx context.Context) ([]clustergroup.ClusterGroup, error) {
	groups := make([]clustergroup.ClusterGroup, 0)

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
