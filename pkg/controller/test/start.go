/*
Licensed to the Apache Software Foundation (ASF) under one or more
contributor license agreements.  See the NOTICE file distributed with
this work for additional information regarding copyright ownership.
The ASF licenses this file to You under the Apache License, Version 2.0
(the "License"); you may not use this file except in compliance with
the License.  You may obtain a copy of the License at

   http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package test

import (
	"context"

	"github.com/jboss-fuse/yaks/pkg/apis/yaks/v1alpha1"
	"github.com/jboss-fuse/yaks/pkg/install"
	"github.com/jboss-fuse/yaks/pkg/util/kubernetes"
	"github.com/jboss-fuse/yaks/version"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/rbac/v1beta1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewStartAction creates a new start action
func NewStartAction() Action {
	return &startAction{}
}

type startAction struct {
	baseAction
}

// Name returns a common name of the action
func (action *startAction) Name() string {
	return "start"
}

// CanHandle tells whether this action can handle the test
func (action *startAction) CanHandle(build *v1alpha1.Test) bool {
	return build.Status.Phase == v1alpha1.TestPhasePending
}

// Handle handles the test
func (action *startAction) Handle(ctx context.Context, test *v1alpha1.Test) (*v1alpha1.Test, error) {
	// Create the viewer service account
	if err := action.ensureServiceAccountRoles(ctx, test.Namespace); err != nil {
		return nil, err
	}

	cm := action.newTestingConfigMap(ctx, test)
	pod := action.newTestingPod(ctx, test, cm)
	resources := []runtime.Object{cm, pod}
	if err := kubernetes.ReplaceResources(ctx, action.client, resources); err != nil {
		return nil, err
	}

	test.Status.Phase = v1alpha1.TestPhaseRunning
	return test, nil
}

func (action *startAction) newTestingPod(ctx context.Context, test *v1alpha1.Test, cm *v1.ConfigMap) *v1.Pod {
	controller := true
	blockOwnerDeletion := true
	pod := v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: test.Namespace,
			Name:      TestPodNameFor(test),
			Labels: map[string]string{
				"yaks.dev/test":    test.Name,
				"yaks.dev/test-id": test.Status.TestID,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         test.APIVersion,
					Kind:               test.Kind,
					Name:               test.Name,
					UID:                test.UID,
					Controller:         &controller,
					BlockOwnerDeletion: &blockOwnerDeletion,
				},
			},
		},
		Spec: v1.PodSpec{
			ServiceAccountName: "yaks-viewer",
			Containers: []v1.Container{
				{
					Name:            "test",
					Image:           "yaks/yaks:" + version.Version,
					Command:         []string{"/usr/local/s2i/run"},
					ImagePullPolicy: v1.PullIfNotPresent,
					VolumeMounts: []v1.VolumeMount{
						{
							Name:      "tests",
							MountPath: "/etc/yaks/test",
						},
					},
					Env: []v1.EnvVar{
						{
							Name:  "JAVA_MAIN_CLASS",
							Value: "dev.yaks.testing.TestRunner",
						},
						{
							Name:  "JAVA_LIB_DIR",
							Value: "/deployments/dependencies/*",
						},
					},
				},
			},
			RestartPolicy: v1.RestartPolicyNever,
			Volumes: []v1.Volume{
				{
					Name: "tests",
					VolumeSource: v1.VolumeSource{
						ConfigMap: &v1.ConfigMapVolumeSource{
							LocalObjectReference: v1.LocalObjectReference{
								Name: cm.Name,
							},
						},
					},
				},
			},
		},
	}

	return &pod
}

func (action *startAction) newTestingConfigMap(ctx context.Context, test *v1alpha1.Test) *v1.ConfigMap {
	controller := true
	blockOwnerDeletion := true

	sources := make(map[string]string)
	sources[test.Spec.Source.Name] = test.Spec.Source.Content

	cm := v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: test.Namespace,
			Name:      TestResourceNameFor(test),
			Labels: map[string]string{
				"yaks.dev/test":    test.Name,
				"yaks.dev/test-id": test.Status.TestID,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         test.APIVersion,
					Kind:               test.Kind,
					Name:               test.Name,
					UID:                test.UID,
					Controller:         &controller,
					BlockOwnerDeletion: &blockOwnerDeletion,
				},
			},
		},
		Data: sources,
	}
	return &cm
}

func (action *startAction) ensureServiceAccountRoles(ctx context.Context, namespace string) error {
	rb := v1beta1.RoleBinding{}
	rbKey := client.ObjectKey{
		Name:      "yaks-viewer",
		Namespace: namespace,
	}

	err := action.client.Get(ctx, rbKey, &rb)
	if err != nil && k8serrors.IsNotFound(err) {
		// Create proper service account and roles
		return install.ViewerServiceAccountRoles(ctx, action.client, namespace)
	}
	return err
}
