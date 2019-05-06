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
	"fmt"

	"github.com/pkg/errors"
)

type unknownFeature struct {
	name string
}

func (e *unknownFeature) Error() string {
	return "unknown feature"
}

func (e *unknownFeature) Context() []interface{} {
	return []interface{}{
		"name", e.name,
	}
}

// IsUnknownFeatureError returns true if the passed in error designates an unknown feature (no registered handler) error
func IsUnknownFeatureError(err error) bool {
	_, ok := errors.Cause(err).(*clusterGroupNotFoundError)

	return ok
}

type clusterGroupNotFoundError struct {
	clusterGroup ClusterGroupModel
}

func (e *clusterGroupNotFoundError) Error() string {
	return "cluster group not found"
}

func (e *clusterGroupNotFoundError) Context() []interface{} {
	return []interface{}{
		"clusterGroupID", e.clusterGroup.ID,
		"organizationID", e.clusterGroup.OrganizationID,
	}
}

// IsClusterGroupNotFoundError returns true if the passed in error designates a cluster group not found error
func IsClusterGroupNotFoundError(err error) bool {
	_, ok := errors.Cause(err).(*clusterGroupNotFoundError)

	return ok
}

type clusterGroupAlreadyExistsError struct {
	clusterGroup ClusterGroupModel
}

func (e *clusterGroupAlreadyExistsError) Error() string {
	return "cluster group already exists with this name"
}

func (e *clusterGroupAlreadyExistsError) Context() []interface{} {
	return []interface{}{
		"clusterGroupName", e.clusterGroup.Name,
		"organizationID", e.clusterGroup.OrganizationID,
	}
}

// IsClusterGroupAlreadyExistsError returns true if the passed in error designates a cluster group already exists error
func IsClusterGroupAlreadyExistsError(err error) bool {
	_, ok := errors.Cause(err).(*clusterGroupAlreadyExistsError)

	return ok
}

type memberClusterNotFoundError struct {
	orgID     uint
	clusterID uint
}

func (e *memberClusterNotFoundError) Error() string {
	return "member cluster not found"
}

func (e *memberClusterNotFoundError) Message() string {
	return fmt.Sprintf("%s: %d", e.Error(), e.clusterID)
}

func (e *memberClusterNotFoundError) Context() []interface{} {
	return []interface{}{
		"clusterID", e.clusterID,
		"organizationID", e.orgID,
	}
}

// IsMemberClusterNotFoundError returns true if the passed in error designates a cluster group member is not found
func IsMemberClusterNotFoundError(err error) (*memberClusterNotFoundError, bool) {
	e, ok := errors.Cause(err).(*memberClusterNotFoundError)

	return e, ok
}

type deploymentNotFoundError struct {
	clusterGroupID uint
	deploymentName string
}

func (e *deploymentNotFoundError) Error() string {
	return "deployment not found"
}

func (e *deploymentNotFoundError) Context() []interface{} {
	return []interface{}{
		"clusterGroupID", e.clusterGroupID,
		"deploymentName", e.deploymentName,
	}
}

// IsDeploymentNotFoundError returns true if the passed in error designates a deployment not found error
func IsDeploymentNotFoundError(err error) bool {
	_, ok := errors.Cause(err).(*deploymentNotFoundError)

	return ok
}

type recordNotFoundError struct{}

func (e *recordNotFoundError) Error() string {
	return "record not found"
}

// IsRecordNotFoundError returns true if the passed in error designates that a DB record not found
func IsRecordNotFoundError(err error) bool {
	_, ok := errors.Cause(err).(*recordNotFoundError)

	return ok
}

type featureRecordNotFoundError struct{}

func (e *featureRecordNotFoundError) Error() string {
	return "feature not found"
}

// IsFeatureRecordNotFoundError returns true if the passed in error designates that a feature DB record not found
func IsFeatureRecordNotFoundError(err error) bool {
	_, ok := errors.Cause(err).(*featureRecordNotFoundError)

	return ok
}

type clusterGroupUpdateRejectedError struct {
	featureName string
}

func (e *clusterGroupUpdateRejectedError) Error() string {
	return "update rejected by feature handler"
}

func (e *clusterGroupUpdateRejectedError) Message() string {
	return fmt.Sprintf("%s: %s", e.Error(), e.featureName)
}

func (e *clusterGroupUpdateRejectedError) Context() []interface{} {
	return []interface{}{
		"featureName", e.featureName,
	}
}

// IsClusterGroupUpdateRejectedError returns true if the passed in error designates that a cluster group update is denied by an enabled feature's handler
func IsClusterGroupUpdateRejectedError(err error) (*clusterGroupUpdateRejectedError, bool) {
	e, ok := errors.Cause(err).(*clusterGroupUpdateRejectedError)

	return e, ok
}

type noReadyMembersError struct {
	clusterGroup ClusterGroupModel
}

func (e *noReadyMembersError) Error() string {
	return "no ready cluster members found"
}

func (e *noReadyMembersError) Context() []interface{} {
	return []interface{}{
		"clusterGroupName", e.clusterGroup.Name,
		"organizationID", e.clusterGroup.OrganizationID,
	}
}

// IsNoReadyMembersError returns true if the passed in error designates no ready cluster members found for a cluster group error
func IsNoReadyMembersError(err error) bool {
	_, ok := errors.Cause(err).(*noReadyMembersError)

	return ok
}

type memberClusterPartOfAClusterGroupError struct {
	orgID     uint
	clusterID uint
}

func (e *memberClusterPartOfAClusterGroupError) Error() string {
	return "member cluster is already part of a cluster group"
}

func (e *memberClusterPartOfAClusterGroupError) Message() string {
	return fmt.Sprintf("%s: %d", e.Error(), e.clusterID)
}

func (e *memberClusterPartOfAClusterGroupError) Context() []interface{} {
	return []interface{}{
		"clusterID", e.clusterID,
		"organizationID", e.orgID,
	}
}

// IsMemberClusterPartOfAClusterGroupError returns true if the passed in error designates a cluster group member is already part of a cluster group
func IsMemberClusterPartOfAClusterGroupError(err error) (*memberClusterPartOfAClusterGroupError, bool) {
	e, ok := errors.Cause(err).(*memberClusterPartOfAClusterGroupError)

	return e, ok
}
