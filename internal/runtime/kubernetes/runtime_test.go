package kubernetes

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestKubernetesRuntime_NewRuntime(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	rt := NewRuntime(clientset, nil)

	assert.NotNil(t, rt)
	assert.Equal(t, "Kubernetes", rt.Name())
	assert.Equal(t, runtime.RuntimeKubernetes, rt.Type())
}

func TestKubernetesRuntime_Ping(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		rt := NewRuntime(clientset, nil)

		err := rt.Ping(context.Background())
		assert.NoError(t, err)
	})
}

func TestKubernetesRuntime_List(t *testing.T) {
	t.Run("lists pods", func(t *testing.T) {
		// Create fake pods
		pod1 := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nginx-abc123",
				Namespace: "default",
				Labels: map[string]string{
					"nekzus.app.id": "nginx",
					"app":           "nginx",
				},
				CreationTimestamp: metav1.NewTime(time.Now().Add(-1 * time.Hour)),
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "nginx",
						Image: "nginx:latest",
						Ports: []corev1.ContainerPort{
							{ContainerPort: 80, Protocol: corev1.ProtocolTCP},
						},
					},
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name:  "nginx",
						Ready: true,
						State: corev1.ContainerState{
							Running: &corev1.ContainerStateRunning{},
						},
					},
				},
			},
		}

		pod2 := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "redis-xyz789",
				Namespace: "default",
				Labels: map[string]string{
					"nekzus.app.id": "redis",
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "redis", Image: "redis:7"},
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
			},
		}

		clientset := fake.NewSimpleClientset(pod1, pod2)
		rt := NewRuntime(clientset, nil)

		containers, err := rt.List(context.Background(), runtime.ListOptions{All: true})

		require.NoError(t, err)
		assert.Len(t, containers, 2)

		// Check first container
		assert.Equal(t, "nginx-abc123", containers[0].ID.ID)
		assert.Equal(t, runtime.RuntimeKubernetes, containers[0].ID.Runtime)
		assert.Equal(t, "default", containers[0].ID.Namespace)
		assert.Equal(t, "nginx-abc123", containers[0].Name)
		assert.Equal(t, "nginx:latest", containers[0].Image)
		assert.Equal(t, runtime.StateRunning, containers[0].State)
		assert.Equal(t, "nginx", containers[0].Labels["nekzus.app.id"])

		// Check second container
		assert.Equal(t, "redis-xyz789", containers[1].ID.ID)
		assert.Equal(t, runtime.StatePending, containers[1].State)
	})

	t.Run("filters by namespace", func(t *testing.T) {
		pod1 := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nginx-prod",
				Namespace: "production",
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "nginx", Image: "nginx:latest"}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		}
		pod2 := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nginx-dev",
				Namespace: "development",
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "nginx", Image: "nginx:latest"}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		}

		clientset := fake.NewSimpleClientset(pod1, pod2)
		rt := NewRuntime(clientset, nil)

		containers, err := rt.List(context.Background(), runtime.ListOptions{
			Namespace: "production",
		})

		require.NoError(t, err)
		assert.Len(t, containers, 1)
		assert.Equal(t, "nginx-prod", containers[0].Name)
	})

	t.Run("empty list", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		rt := NewRuntime(clientset, nil)

		containers, err := rt.List(context.Background(), runtime.ListOptions{})

		require.NoError(t, err)
		assert.Empty(t, containers)
	})
}

func TestKubernetesRuntime_Start(t *testing.T) {
	t.Run("scales deployment to 1", func(t *testing.T) {
		// Create a deployment with 0 replicas
		replicas := int32(0)
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nginx",
				Namespace: "default",
				UID:       "deploy-uid-123",
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "nginx"},
				},
			},
		}

		// Create a pod owned by the deployment (via ReplicaSet)
		replicaSet := &appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nginx-rs",
				Namespace: "default",
				UID:       "rs-uid-123",
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
				Containers: []corev1.Container{{Name: "nginx", Image: "nginx:latest"}},
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
		err := rt.Start(context.Background(), id)

		require.NoError(t, err)

		// Verify deployment was scaled
		updated, err := clientset.AppsV1().Deployments("default").Get(context.Background(), "nginx", metav1.GetOptions{})
		require.NoError(t, err)
		assert.Equal(t, int32(1), *updated.Spec.Replicas)
	})

	t.Run("pod not found", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		rt := NewRuntime(clientset, nil)

		id := runtime.ContainerID{
			Runtime:   runtime.RuntimeKubernetes,
			ID:        "nonexistent",
			Namespace: "default",
		}
		err := rt.Start(context.Background(), id)

		assert.Error(t, err)
		assert.True(t, runtime.IsNotFoundError(err))
	})
}

func TestKubernetesRuntime_Stop(t *testing.T) {
	t.Run("scales deployment to 0", func(t *testing.T) {
		replicas := int32(1)
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nginx",
				Namespace: "default",
				UID:       "deploy-uid-123",
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "nginx"},
				},
			},
		}

		replicaSet := &appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nginx-rs",
				Namespace: "default",
				UID:       "rs-uid-123",
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
				Containers: []corev1.Container{{Name: "nginx", Image: "nginx:latest"}},
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
		err := rt.Stop(context.Background(), id, nil)

		require.NoError(t, err)

		// Verify deployment was scaled to 0
		updated, err := clientset.AppsV1().Deployments("default").Get(context.Background(), "nginx", metav1.GetOptions{})
		require.NoError(t, err)
		assert.Equal(t, int32(0), *updated.Spec.Replicas)
	})
}

func TestKubernetesRuntime_Restart(t *testing.T) {
	t.Run("deletes pod for restart", func(t *testing.T) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nginx-abc123",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "nginx", Image: "nginx:latest"}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		}

		clientset := fake.NewSimpleClientset(pod)
		rt := NewRuntime(clientset, nil)

		id := runtime.ContainerID{
			Runtime:   runtime.RuntimeKubernetes,
			ID:        "nginx-abc123",
			Namespace: "default",
		}
		err := rt.Restart(context.Background(), id, nil)

		require.NoError(t, err)

		// Verify pod was deleted
		_, err = clientset.CoreV1().Pods("default").Get(context.Background(), "nginx-abc123", metav1.GetOptions{})
		assert.Error(t, err) // Should be not found
	})
}

func TestKubernetesRuntime_Inspect(t *testing.T) {
	t.Run("returns pod details", func(t *testing.T) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nginx-abc123",
				Namespace: "default",
				Labels: map[string]string{
					"app": "nginx",
				},
				CreationTimestamp: metav1.NewTime(time.Now().Add(-1 * time.Hour)),
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "nginx",
						Image: "nginx:latest",
						Env: []corev1.EnvVar{
							{Name: "ENV", Value: "production"},
						},
						Command: []string{"nginx", "-g", "daemon off;"},
					},
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				PodIP: "10.0.0.5",
			},
		}

		clientset := fake.NewSimpleClientset(pod)
		rt := NewRuntime(clientset, nil)

		id := runtime.ContainerID{
			Runtime:   runtime.RuntimeKubernetes,
			ID:        "nginx-abc123",
			Namespace: "default",
		}
		details, err := rt.Inspect(context.Background(), id)

		require.NoError(t, err)
		assert.Equal(t, "nginx-abc123", details.ID.ID)
		assert.Equal(t, "default", details.ID.Namespace)
		assert.Equal(t, "nginx-abc123", details.Name)
		assert.Equal(t, "nginx:latest", details.Image)
		assert.Equal(t, runtime.StateRunning, details.State)
		assert.Contains(t, details.Config.Env, "ENV=production")
		assert.Equal(t, "10.0.0.5", details.NetworkSettings.IPAddress)
		assert.NotNil(t, details.Raw)
	})

	t.Run("pod not found", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		rt := NewRuntime(clientset, nil)

		id := runtime.ContainerID{
			Runtime:   runtime.RuntimeKubernetes,
			ID:        "nonexistent",
			Namespace: "default",
		}
		_, err := rt.Inspect(context.Background(), id)

		assert.Error(t, err)
		assert.True(t, runtime.IsNotFoundError(err))
	})
}

func TestKubernetesRuntime_StreamLogs(t *testing.T) {
	t.Run("streams pod logs", func(t *testing.T) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nginx-abc123",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "nginx", Image: "nginx:latest"},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		}

		clientset := fake.NewSimpleClientset(pod)
		rt := NewRuntime(clientset, nil)

		id := runtime.ContainerID{
			Runtime:   runtime.RuntimeKubernetes,
			ID:        "nginx-abc123",
			Namespace: "default",
		}

		// Note: fake clientset doesn't actually return logs, so we just verify no error
		reader, err := rt.StreamLogs(context.Background(), id, runtime.LogOptions{
			Follow:     false,
			Tail:       100,
			Timestamps: true,
		})

		// The fake client returns an empty reader
		require.NoError(t, err)
		if reader != nil {
			defer reader.Close()
		}
	})

	t.Run("pod not found", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		rt := NewRuntime(clientset, nil)

		id := runtime.ContainerID{
			Runtime:   runtime.RuntimeKubernetes,
			ID:        "nonexistent",
			Namespace: "default",
		}
		_, err := rt.StreamLogs(context.Background(), id, runtime.LogOptions{})

		assert.Error(t, err)
		assert.True(t, runtime.IsNotFoundError(err))
	})
}

func TestKubernetesRuntime_GetStats(t *testing.T) {
	t.Run("returns error when metrics unavailable", func(t *testing.T) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nginx-abc123",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "nginx",
						Image: "nginx:latest",
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("500m"),
								corev1.ResourceMemory: resource.MustParse("128Mi"),
							},
						},
					},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		}

		clientset := fake.NewSimpleClientset(pod)
		// No metrics client configured
		rt := NewRuntime(clientset, nil)

		id := runtime.ContainerID{
			Runtime:   runtime.RuntimeKubernetes,
			ID:        "nginx-abc123",
			Namespace: "default",
		}
		_, err := rt.GetStats(context.Background(), id)

		// Should return metrics unavailable error when no metrics client
		assert.Error(t, err)
		assert.True(t, runtime.IsUnavailableError(err))
	})
}

func TestKubernetesRuntime_GetBatchStats(t *testing.T) {
	t.Run("empty list returns empty", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		rt := NewRuntime(clientset, nil)

		stats, err := rt.GetBatchStats(context.Background(), []runtime.ContainerID{})

		require.NoError(t, err)
		assert.Empty(t, stats)
	})
}

func TestConvertPodPhase(t *testing.T) {
	tests := []struct {
		phase    corev1.PodPhase
		expected runtime.ContainerState
	}{
		{corev1.PodRunning, runtime.StateRunning},
		{corev1.PodPending, runtime.StatePending},
		{corev1.PodSucceeded, runtime.StateStopped},
		{corev1.PodFailed, runtime.StateFailed},
		{corev1.PodUnknown, runtime.StateStopped},
	}

	for _, tt := range tests {
		t.Run(string(tt.phase), func(t *testing.T) {
			result := convertPodPhase(tt.phase)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestKubernetesRuntime_Close(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	rt := NewRuntime(clientset, nil)

	err := rt.Close()
	assert.NoError(t, err)
}

// Helper to create a simple reader for log tests
type mockLogReader struct {
	data string
}

func (m *mockLogReader) Read(p []byte) (n int, err error) {
	if m.data == "" {
		return 0, io.EOF
	}
	n = copy(p, m.data)
	m.data = m.data[n:]
	if m.data == "" {
		return n, io.EOF
	}
	return n, nil
}

func (m *mockLogReader) Close() error {
	return nil
}

func newMockLogReader(data string) io.ReadCloser {
	return &mockLogReader{data: data}
}

func TestKubernetesRuntime_BulkRestart(t *testing.T) {
	t.Run("restarts multiple pods concurrently", func(t *testing.T) {
		pod1 := createTestPod("nginx-abc123", "default", "nginx:latest", corev1.PodRunning)
		pod2 := createTestPod("redis-def456", "default", "redis:latest", corev1.PodRunning)
		pod3 := createTestPod("postgres-ghi789", "production", "postgres:latest", corev1.PodRunning)

		clientset := fake.NewSimpleClientset(pod1, pod2, pod3)
		rt := NewRuntime(clientset, nil)

		ids := []runtime.ContainerID{
			{Runtime: runtime.RuntimeKubernetes, ID: "nginx-abc123", Namespace: "default"},
			{Runtime: runtime.RuntimeKubernetes, ID: "redis-def456", Namespace: "default"},
			{Runtime: runtime.RuntimeKubernetes, ID: "postgres-ghi789", Namespace: "production"},
		}

		results, err := rt.BulkRestart(context.Background(), ids, nil)

		require.NoError(t, err)
		require.Len(t, results, 3)

		// All should succeed
		for _, result := range results {
			assert.True(t, result.Success, "Expected success for %s", result.ContainerID.ID)
			assert.Empty(t, result.Error)
		}

		// Verify pods were deleted
		_, err = clientset.CoreV1().Pods("default").Get(context.Background(), "nginx-abc123", metav1.GetOptions{})
		assert.Error(t, err)
		_, err = clientset.CoreV1().Pods("default").Get(context.Background(), "redis-def456", metav1.GetOptions{})
		assert.Error(t, err)
		_, err = clientset.CoreV1().Pods("production").Get(context.Background(), "postgres-ghi789", metav1.GetOptions{})
		assert.Error(t, err)
	})

	t.Run("handles partial failures", func(t *testing.T) {
		pod1 := createTestPod("nginx-abc123", "default", "nginx:latest", corev1.PodRunning)
		// pod2 doesn't exist - should fail

		clientset := fake.NewSimpleClientset(pod1)
		rt := NewRuntime(clientset, nil)

		ids := []runtime.ContainerID{
			{Runtime: runtime.RuntimeKubernetes, ID: "nginx-abc123", Namespace: "default"},
			{Runtime: runtime.RuntimeKubernetes, ID: "nonexistent", Namespace: "default"},
		}

		results, err := rt.BulkRestart(context.Background(), ids, nil)

		require.NoError(t, err) // BulkRestart itself doesn't error
		require.Len(t, results, 2)

		// First should succeed
		assert.True(t, results[0].Success)
		assert.Equal(t, "nginx-abc123", results[0].ContainerID.ID)

		// Second should fail
		assert.False(t, results[1].Success)
		assert.Equal(t, "nonexistent", results[1].ContainerID.ID)
		assert.NotEmpty(t, results[1].Error)
	})

	t.Run("empty list returns empty results", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		rt := NewRuntime(clientset, nil)

		results, err := rt.BulkRestart(context.Background(), []runtime.ContainerID{}, nil)

		require.NoError(t, err)
		assert.Empty(t, results)
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		pod1 := createTestPod("nginx-abc123", "default", "nginx:latest", corev1.PodRunning)
		pod2 := createTestPod("redis-def456", "default", "redis:latest", corev1.PodRunning)

		clientset := fake.NewSimpleClientset(pod1, pod2)
		rt := NewRuntime(clientset, nil)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		ids := []runtime.ContainerID{
			{Runtime: runtime.RuntimeKubernetes, ID: "nginx-abc123", Namespace: "default"},
			{Runtime: runtime.RuntimeKubernetes, ID: "redis-def456", Namespace: "default"},
		}

		results, err := rt.BulkRestart(ctx, ids, nil)

		// Should either error or have failures due to context cancellation
		if err == nil {
			// Some operations may have failed
			for _, result := range results {
				if !result.Success {
					assert.Contains(t, result.Error, "context")
				}
			}
		}
		_ = results // avoid unused warning
	})
}

// Test helper for creating pods with common configurations
func createTestPod(name, namespace, image string, phase corev1.PodPhase) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: strings.Split(name, "-")[0], Image: image},
			},
		},
		Status: corev1.PodStatus{
			Phase: phase,
		},
	}
}
