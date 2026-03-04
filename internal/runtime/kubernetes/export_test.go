package kubernetes

import (
	"context"
	"testing"

	"github.com/nstalgic/nekzus/internal/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestKubernetesRuntime_Export(t *testing.T) {
	t.Run("exports pod with deployment owner", func(t *testing.T) {
		// Create deployment, replicaset, and pod
		replicas := int32(3)
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nginx",
				Namespace: "default",
				Labels: map[string]string{
					"app": "nginx",
				},
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "nginx"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": "nginx"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "nginx",
								Image: "nginx:1.21",
								Ports: []corev1.ContainerPort{
									{ContainerPort: 80},
								},
							},
						},
					},
				},
			},
		}

		replicaSet := &appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nginx-rs",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "nginx",
						UID:        "deploy-uid-123",
					},
				},
			},
		}

		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nginx-abc123",
				Namespace: "default",
				Labels: map[string]string{
					"app": "nginx",
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: "apps/v1",
						Kind:       "ReplicaSet",
						Name:       "nginx-rs",
						UID:        "rs-uid-123",
					},
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "nginx", Image: "nginx:1.21"}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		}

		clientset := fake.NewSimpleClientset(deployment, replicaSet, pod)
		rt := NewRuntime(clientset, nil)

		id := runtime.ContainerID{
			Runtime:   runtime.RuntimeKubernetes,
			ID:        "nginx-abc123",
			Namespace: "default",
		}

		result, err := rt.Export(context.Background(), []runtime.ContainerID{id}, ExportOptions{
			Format: ExportFormatYAML,
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		require.Len(t, result.Manifests, 1)

		manifest := result.Manifests[0]
		assert.Equal(t, "Deployment", manifest.Kind)
		assert.Equal(t, "nginx", manifest.Name)
		assert.Contains(t, manifest.Content, "kind: Deployment")
		assert.Contains(t, manifest.Content, "name: nginx")
		assert.Contains(t, manifest.Content, "image: nginx:1.21")
	})

	t.Run("exports standalone pod", func(t *testing.T) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "standalone-pod",
				Namespace: "default",
				Labels: map[string]string{
					"app": "standalone",
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "app", Image: "myapp:latest"},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		}

		clientset := fake.NewSimpleClientset(pod)
		rt := NewRuntime(clientset, nil)

		id := runtime.ContainerID{
			Runtime:   runtime.RuntimeKubernetes,
			ID:        "standalone-pod",
			Namespace: "default",
		}

		result, err := rt.Export(context.Background(), []runtime.ContainerID{id}, ExportOptions{
			Format: ExportFormatYAML,
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		require.Len(t, result.Manifests, 1)

		manifest := result.Manifests[0]
		assert.Equal(t, "Pod", manifest.Kind)
		assert.Equal(t, "standalone-pod", manifest.Name)
		assert.Contains(t, manifest.Content, "kind: Pod")
	})

	t.Run("exports with related services", func(t *testing.T) {
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "web",
				Namespace: "default",
				Labels:    map[string]string{"app": "web"},
			},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "web"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": "web"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "web", Image: "web:latest"},
						},
					},
				},
			},
		}

		service := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "web-svc",
				Namespace: "default",
			},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "web"},
				Ports: []corev1.ServicePort{
					{Port: 80, Name: "http"},
				},
			},
		}

		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "web-xyz",
				Namespace: "default",
				Labels:    map[string]string{"app": "web"},
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "ReplicaSet", Name: "web-rs"},
				},
			},
		}

		replicaSet := &appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "web-rs",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "Deployment", Name: "web"},
				},
			},
		}

		clientset := fake.NewSimpleClientset(deployment, service, pod, replicaSet)
		rt := NewRuntime(clientset, nil)

		id := runtime.ContainerID{
			Runtime:   runtime.RuntimeKubernetes,
			ID:        "web-xyz",
			Namespace: "default",
		}

		result, err := rt.Export(context.Background(), []runtime.ContainerID{id}, ExportOptions{
			Format:          ExportFormatYAML,
			IncludeServices: true,
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		require.Len(t, result.Manifests, 2) // Deployment + Service

		// Check we have both Deployment and Service
		kinds := make(map[string]bool)
		for _, m := range result.Manifests {
			kinds[m.Kind] = true
		}
		assert.True(t, kinds["Deployment"])
		assert.True(t, kinds["Service"])
	})

	t.Run("pod not found returns error", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		rt := NewRuntime(clientset, nil)

		id := runtime.ContainerID{
			Runtime:   runtime.RuntimeKubernetes,
			ID:        "nonexistent",
			Namespace: "default",
		}

		_, err := rt.Export(context.Background(), []runtime.ContainerID{id}, ExportOptions{})

		assert.Error(t, err)
	})

	t.Run("empty list returns empty result", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		rt := NewRuntime(clientset, nil)

		result, err := rt.Export(context.Background(), []runtime.ContainerID{}, ExportOptions{})

		require.NoError(t, err)
		assert.Empty(t, result.Manifests)
	})
}
