/*
Copyright 2024 The Beskar7 Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/annotations"
)

// isPaused checks if a resource has the pause annotation present.
// It returns true if the pause annotation exists (regardless of value).
func isPaused(obj metav1.Object) bool {
	return annotations.HasPaused(obj)
}

// isClusterPaused checks if the owner cluster has the pause annotation present.
// It returns true if the pause annotation exists (regardless of value).
func isClusterPaused(cluster *clusterv1.Cluster) bool {
	if cluster == nil {
		return false
	}
	return annotations.HasPaused(cluster)
}
