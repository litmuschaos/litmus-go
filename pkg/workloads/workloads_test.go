package workloads

import (
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dfake "k8s.io/client-go/dynamic/fake"
	"testing"
)

var fakeDynamicClient dynamic.Interface = nil

func Test_getPodsFromWorkload(t *testing.T) {
	tests := []struct {
		name      string
		target    types.AppDetails
		allPods   *corev1.PodList
		wantCount int
		expectErr bool
	}{
		{
			name: "match pod by kind and name",
			target: types.AppDetails{
				Kind:      "statefulset",
				Namespace: "default",
				Names:     []string{"my-statefulset"},
			},
			allPods: &corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: v1.ObjectMeta{
							Name:      "pod-1",
							Namespace: "default",
							OwnerReferences: []v1.OwnerReference{
								{
									Kind: "StatefulSet",
									Name: "my-statefulset",
								},
							},
						},
					},
				},
			},
			wantCount: 1,
			expectErr: false,
		},
		{
			name: "no matching pod",
			target: types.AppDetails{
				Kind:      "daemonset",
				Namespace: "default",
				Names:     []string{"non-existent"},
			},
			allPods: &corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: v1.ObjectMeta{
							Name:      "pod-2",
							Namespace: "default",
							OwnerReferences: []v1.OwnerReference{
								{
									Kind: "DaemonSet",
									Name: "some-other-daemonset",
								},
							},
						},
					},
				},
			},
			wantCount: 0,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getPodsFromWorkload(tt.target, tt.allPods, fakeDynamicClient)
			if (err != nil) != tt.expectErr {
				t.Errorf("getPodsFromWorkload() error = %v, expectErr = %v", err, tt.expectErr)
			}
			if len(got.Items) != tt.wantCount {
				t.Errorf("expected %d pods, got %d", tt.wantCount, len(got.Items))
			}
		})
	}
}

func Test_getParent(t *testing.T) {
	gvr := schema.GroupVersionResource{
		Group:    "apps",
		Version:  "v1",
		Resource: "replicasets",
	}

	scheme := runtime.NewScheme()
	tests := []struct {
		name         string
		resourceName string
		namespace    string
		owners       []metav1.OwnerReference
		expectKind   string
		expectName   string
		expectError  bool
	}{
		{
			name:         "has deployment owner",
			resourceName: "my-replicaset",
			namespace:    "default",
			owners: []metav1.OwnerReference{
				{Kind: "Deployment", Name: "my-deployment"},
			},
			expectKind:  "deployment",
			expectName:  "my-deployment",
			expectError: false,
		},
		{
			name:         "has rollout owner",
			resourceName: "rollout-set",
			namespace:    "default",
			owners: []metav1.OwnerReference{
				{Kind: "Rollout", Name: "my-rollout"},
			},
			expectKind:  "rollout",
			expectName:  "my-rollout",
			expectError: false,
		},
		{
			name:         "has deploymentconfig owner",
			resourceName: "dc-set",
			namespace:    "default",
			owners: []metav1.OwnerReference{
				{Kind: "DeploymentConfig", Name: "my-dc"},
			},
			expectKind:  "deploymentconfig",
			expectName:  "my-dc",
			expectError: false,
		},
		{
			name:         "no matching owner kind",
			resourceName: "other-set",
			namespace:    "default",
			owners: []metav1.OwnerReference{
				{Kind: "StatefulSet", Name: "my-ss"},
			},
			expectKind:  "",
			expectName:  "",
			expectError: false,
		},
		{
			name:         "resource not found",
			resourceName: "missing-set",
			namespace:    "default",
			owners:       nil,
			expectKind:   "",
			expectName:   "",
			expectError:  true,
		},
	}

	objs := []runtime.Object{}
	for _, tt := range tests {
		if tt.owners != nil {
			obj := &unstructured.Unstructured{}
			obj.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   "apps",
				Version: "v1",
				Kind:    "ReplicaSet",
			})
			obj.SetName(tt.resourceName)
			obj.SetNamespace(tt.namespace)
			obj.SetOwnerReferences(tt.owners)
			objs = append(objs, obj)
		}
	}

	fakeDynamic := dfake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
		gvr: "ReplicaSetList",
	}, objs...)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, name, err := getParent(tt.resourceName, tt.namespace, gvr, fakeDynamic)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectKind, kind)
				assert.Equal(t, tt.expectName, name)
			}
		})
	}
}

func Test_GetPodOwnerTypeAndName(t *testing.T) {
	scheme := runtime.NewScheme()
	gvr := schema.GroupVersionResource{
		Group:    "apps",
		Version:  "v1",
		Resource: "replicasets",
	}

	// Setup dynamic client for ReplicaSet parent
	rsObj := &unstructured.Unstructured{}
	rsObj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "ReplicaSet",
	})
	rsObj.SetName("my-replicaset")
	rsObj.SetNamespace("default")
	rsObj.SetOwnerReferences([]metav1.OwnerReference{
		{
			Kind: "Deployment",
			Name: "my-deployment",
		},
	})

	fakeDynamic := dfake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
		gvr: "ReplicaSetList",
	}, rsObj)

	tests := []struct {
		name          string
		pod           *corev1.Pod
		expectedKind  string
		expectedName  string
		expectErr     bool
		dynamicClient dynamic.Interface
	}{
		{
			name: "StatefulSet owner",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-ss",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "StatefulSet", Name: "my-ss"},
					},
				},
			},
			expectedKind:  "statefulset",
			expectedName:  "my-ss",
			expectErr:     false,
			dynamicClient: nil,
		},
		{
			name: "ReplicaSet with pod-template-hash match",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-rs",
					Namespace: "default",
					Labels: map[string]string{
						"pod-template-hash": "my-replicaset",
					},
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "ReplicaSet", Name: "my-replicaset"},
					},
				},
			},
			expectedKind:  "deployment",
			expectedName:  "my-deployment",
			expectErr:     false,
			dynamicClient: fakeDynamic,
		},
		{
			name: "No owner",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "orphan-pod",
					OwnerReferences: nil,
				},
			},
			expectedKind:  "",
			expectedName:  "",
			expectErr:     false,
			dynamicClient: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, name, err := GetPodOwnerTypeAndName(tt.pod, tt.dynamicClient)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedKind, kind)
				assert.Equal(t, tt.expectedName, name)
			}
		})
	}
}
