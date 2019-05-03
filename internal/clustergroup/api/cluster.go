package api

import (
	"github.com/banzaicloud/pipeline/pkg/cluster"
)

// Cluster
type Cluster interface {
	GetID() uint
	GetName() string
	GetK8sConfig() ([]byte, error)
	GetStatus() (*cluster.GetClusterStatusResponse, error)
	IsReady() (bool, error)
}
