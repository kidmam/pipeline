package api

import (
	"context"
)

// ClusterGetter
type ClusterGetter interface {
	GetClusterByIDOnly(ctx context.Context, clusterID uint) (Cluster, error)
	GetClusterByName(ctx context.Context, organizationID uint, clusterName string) (Cluster, error)
}
