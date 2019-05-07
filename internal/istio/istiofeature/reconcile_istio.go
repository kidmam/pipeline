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

package istiofeature

import (
	"strings"
	"time"

	"github.com/goph/emperror"
	"github.com/pkg/errors"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/banzaicloud/istio-operator/pkg/apis/istio/v1beta1"
	istiooperatorclientset "github.com/banzaicloud/istio-operator/pkg/client/clientset/versioned"
	"github.com/banzaicloud/pipeline/internal/backoff"
	pkgCluster "github.com/banzaicloud/pipeline/pkg/cluster"
)

func (m *MeshReconciler) ReconcileIstio(desiredState DesiredState) error {
	m.logger.Debug("reconciling Istio CR")
	defer m.logger.Debug("Istio CR reconciled")

	client, err := m.GetMasterIstioOperatorK8sClient()
	if err != nil {
		return err
	}

	if desiredState == DesiredStatePresent {
		_, err := client.IstioV1beta1().Istios(istioOperatorNamespace).Get(m.Configuration.name, metav1.GetOptions{})
		if err != nil && !k8serrors.IsNotFound(err) {
			return emperror.Wrap(err, "could not check existance Istio CR")
		}

		if err == nil {
			m.logger.Debug("Istio CR already exists")
			return nil
		}

		ipRanges, err := m.Master.GetK8sIpv4Cidrs()
		if err != nil {
			return emperror.Wrap(err, "could not get ipv4 ranges for cluster")
		}
		istioCR := m.generateIstioCR(m.Configuration, ipRanges)
		_, err = client.IstioV1beta1().Istios(istioOperatorNamespace).Create(&istioCR)
		if err != nil {
			return emperror.Wrap(err, "could not create Istio CR")
		}
	} else {
		err := client.IstioV1beta1().Istios(istioOperatorNamespace).Delete(m.Configuration.name, &metav1.DeleteOptions{})
		if err != nil && !k8serrors.IsNotFound(err) {
			return emperror.Wrap(err, "could not remove Istio CR")
		}

		err = m.waitForIstioCRToBeDeleted(client)
		if err != nil {
			return emperror.Wrap(err, "timeout during waiting for Istio CR to be deleted")
		}
	}

	return nil
}

// waitForIstioCRToBeDeleted wait for Istio CR to be deleted
func (m *MeshReconciler) waitForIstioCRToBeDeleted(client *istiooperatorclientset.Clientset) error {
	m.logger.Debug("waiting for Istio CR to be deleted")

	var backoffConfig = backoff.ConstantBackoffConfig{
		Delay:      time.Duration(backoffDelaySeconds) * time.Second,
		MaxRetries: backoffMaxretries,
	}
	var backoffPolicy = backoff.NewConstantBackoffPolicy(&backoffConfig)

	err := backoff.Retry(func() error {
		_, err := client.IstioV1beta1().Istios(istioOperatorNamespace).Get(m.Configuration.name, metav1.GetOptions{})
		if k8serrors.IsNotFound(err) {
			return nil
		}

		return errors.New("Istio CR still exists")
	}, backoffPolicy)

	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

// generateIstioCR generates istio-operator specific CR based on the given params
func (m *MeshReconciler) generateIstioCR(config Config, ipRanges *pkgCluster.Ipv4Cidrs) v1beta1.Istio {
	istioConfig := v1beta1.Istio{
		ObjectMeta: metav1.ObjectMeta{
			Name: m.Configuration.name,
			Labels: map[string]string{
				"controller-tools.k8s.io": "1.0",
			},
		},
		Spec: v1beta1.IstioSpec{
			MTLS:                    config.EnableMTLS,
			AutoInjectionNamespaces: config.AutoSidecarInjectNamespaces,
			Version:                 istioVersion,
			Pilot: v1beta1.PilotConfiguration{
				Image: "waynz0r/pilot:latest",
			},
			Mixer: v1beta1.MixerConfiguration{
				Image: "waynz0r/mixer:latest",
			},
		},
	}

	if len(m.Remotes) > 0 {
		enabled := true
		istioConfig.Spec.UseMCP = enabled
		istioConfig.Spec.MTLS = enabled
		istioConfig.Spec.MeshExpansion = &enabled
		istioConfig.Spec.ControlPlaneSecurityEnabled = enabled
	}

	if config.BypassEgressTraffic {
		istioConfig.Spec.IncludeIPRanges = strings.Join(ipRanges.PodIPRanges, ",") + "," + strings.Join(ipRanges.ServiceClusterIPRanges, ",")
	}

	return istioConfig
}
