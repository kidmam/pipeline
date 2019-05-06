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

func (e *clusterGroupNotFoundError) NotFound() bool {
	return true
}

// IsClusterGroupNotFoundError returns true if the passed in error designates a cluster group not found error
func IsClusterGroupNotFoundError(err error) bool {
	notFoundErr, ok := errors.Cause(err).(*clusterGroupNotFoundError)

	return ok && notFoundErr.NotFound()
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
	orgID       uint
	clusterName string
}

func (e *memberClusterNotFoundError) Error() string {
	return "member cluster not found"
}

func (e *memberClusterNotFoundError) Message() string {
	return fmt.Sprintf("%s: %s", e.Error(), e.clusterName)
}

func (e *memberClusterNotFoundError) Context() []interface{} {
	return []interface{}{
		"clusterGroupName", e.clusterName,
		"organizationID", e.orgID,
	}
}

// IsMemberClusterNotFoundError returns true if the passed in error designates a cluster group member is not found
func IsMemberClusterNotFoundError(err error) (*memberClusterNotFoundError, bool) {
	e, ok := errors.Cause(err).(*memberClusterNotFoundError)

	return e, ok
}

type clusterGroupHasEnabledFeaturesError struct {
	clusterGroupName string
	featureNames     interface{}
}

func (e *clusterGroupHasEnabledFeaturesError) Error() string {
	return "you have to disable features, before deleting the cluster group"
}

func (e *clusterGroupHasEnabledFeaturesError) Context() []interface{} {
	return []interface{}{
		"clusterGroupName", e.clusterGroupName,
		"featureNames", e.featureNames,
	}
}

// IsClusterGroupHasEnabledFeaturesError returns true if the passed in error designates no ready cluster members found for a cluster group error
func IsClusterGroupHasEnabledFeaturesError(err error) bool {
	_, ok := errors.Cause(err).(*clusterGroupHasEnabledFeaturesError)

	return ok
}

type recordNotFoundError struct{}

func (e *recordNotFoundError) Error() string {
	return "record not found"
}

func IsRecordNotFoundError(err error) bool {
	_, ok := errors.Cause(err).(*recordNotFoundError)

	return ok
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
	orgID       uint
	clusterName string
}

func (e *memberClusterPartOfAClusterGroupError) Error() string {
	return "member cluster is already part of a cluster group"
}

func (e *memberClusterPartOfAClusterGroupError) Message() string {
	return fmt.Sprintf("%s: %s", e.Error(), e.clusterName)
}

func (e *memberClusterPartOfAClusterGroupError) Context() []interface{} {
	return []interface{}{
		"clusterGroupName", e.clusterName,
		"organizationID", e.orgID,
	}
}

// IsMemberClusterPartOfAClusterGroupError returns true if the passed in error designates a cluster group member is already part of a cluster group
func IsMemberClusterPartOfAClusterGroupError(err error) (*memberClusterPartOfAClusterGroupError, bool) {
	e, ok := errors.Cause(err).(*memberClusterPartOfAClusterGroupError)

	return e, ok
}
