package toolbox

import (
	"context"
	"testing"

	"github.com/nstalgic/nekzus/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestKubernetesDeployer_Deploy(t *testing.T) {
	t.Run("creates deployment and service", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		deployer := NewKubernetesDeployer(clientset, "default")

		template := &types.ServiceTemplate{
			ID:          "nginx",
			Name:        "Nginx",
			Description: "Web server",
			DockerConfig: types.DockerContainerConfig{
				Image: "nginx:latest",
				Ports: []types.PortMapping{
					{Container: 80, HostDefault: 8080, Protocol: "tcp"},
				},
			},
		}

		deployment := &types.ToolboxDeployment{
			ID:                "deploy-123",
			ServiceTemplateID: "nginx",
			ServiceName:       "my-nginx",
			EnvVars: map[string]string{
				"ENV": "production",
			},
		}

		identifier, err := deployer.Deploy(context.Background(), template, deployment)

		require.NoError(t, err)
		assert.NotEmpty(t, identifier)

		// Verify Deployment was created
		k8sDeploy, err := clientset.AppsV1().Deployments("default").Get(context.Background(), identifier, metav1.GetOptions{})
		require.NoError(t, err)
		assert.Equal(t, "nginx:latest", k8sDeploy.Spec.Template.Spec.Containers[0].Image)

		// Verify Service was created
		svc, err := clientset.CoreV1().Services("default").Get(context.Background(), identifier, metav1.GetOptions{})
		require.NoError(t, err)
		assert.Equal(t, int32(80), svc.Spec.Ports[0].Port)
	})

	t.Run("uses custom image if provided", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		deployer := NewKubernetesDeployer(clientset, "default")

		template := &types.ServiceTemplate{
			ID:   "nginx",
			Name: "Nginx",
			DockerConfig: types.DockerContainerConfig{
				Image: "nginx:latest",
			},
		}

		deployment := &types.ToolboxDeployment{
			ID:                "deploy-123",
			ServiceTemplateID: "nginx",
			ServiceName:       "my-nginx",
			CustomImage:       "nginx:alpine",
		}

		identifier, err := deployer.Deploy(context.Background(), template, deployment)
		require.NoError(t, err)

		k8sDeploy, err := clientset.AppsV1().Deployments("default").Get(context.Background(), identifier, metav1.GetOptions{})
		require.NoError(t, err)
		assert.Equal(t, "nginx:alpine", k8sDeploy.Spec.Template.Spec.Containers[0].Image)
	})

	t.Run("sets environment variables", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		deployer := NewKubernetesDeployer(clientset, "default")

		template := &types.ServiceTemplate{
			ID:   "app",
			Name: "App",
			DockerConfig: types.DockerContainerConfig{
				Image: "app:latest",
				Environment: map[string]string{
					"DEFAULT_VAR": "default",
				},
			},
		}

		deployment := &types.ToolboxDeployment{
			ID:          "deploy-123",
			ServiceName: "my-app",
			EnvVars: map[string]string{
				"USER_VAR": "custom",
			},
		}

		identifier, err := deployer.Deploy(context.Background(), template, deployment)
		require.NoError(t, err)

		k8sDeploy, err := clientset.AppsV1().Deployments("default").Get(context.Background(), identifier, metav1.GetOptions{})
		require.NoError(t, err)

		envVars := k8sDeploy.Spec.Template.Spec.Containers[0].Env
		envMap := make(map[string]string)
		for _, env := range envVars {
			envMap[env.Name] = env.Value
		}

		assert.Equal(t, "default", envMap["DEFAULT_VAR"])
		assert.Equal(t, "custom", envMap["USER_VAR"])
	})
}

func TestKubernetesDeployer_Start(t *testing.T) {
	t.Run("scales deployment to 1 replica", func(t *testing.T) {
		replicas := int32(0)
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-app",
				Namespace: "default",
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
			},
		}

		clientset := fake.NewSimpleClientset(deployment)
		deployer := NewKubernetesDeployer(clientset, "default")

		err := deployer.Start(context.Background(), "my-app")

		require.NoError(t, err)

		updated, err := clientset.AppsV1().Deployments("default").Get(context.Background(), "my-app", metav1.GetOptions{})
		require.NoError(t, err)
		assert.Equal(t, int32(1), *updated.Spec.Replicas)
	})
}

func TestKubernetesDeployer_Stop(t *testing.T) {
	t.Run("scales deployment to 0 replicas", func(t *testing.T) {
		replicas := int32(1)
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-app",
				Namespace: "default",
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
			},
		}

		clientset := fake.NewSimpleClientset(deployment)
		deployer := NewKubernetesDeployer(clientset, "default")

		err := deployer.Stop(context.Background(), "my-app")

		require.NoError(t, err)

		updated, err := clientset.AppsV1().Deployments("default").Get(context.Background(), "my-app", metav1.GetOptions{})
		require.NoError(t, err)
		assert.Equal(t, int32(0), *updated.Spec.Replicas)
	})
}

func TestKubernetesDeployer_Remove(t *testing.T) {
	t.Run("deletes deployment and service", func(t *testing.T) {
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-app",
				Namespace: "default",
			},
		}
		service := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-app",
				Namespace: "default",
			},
		}

		clientset := fake.NewSimpleClientset(deployment, service)
		deployer := NewKubernetesDeployer(clientset, "default")

		err := deployer.Remove(context.Background(), "my-app", false)

		require.NoError(t, err)

		// Verify deployment was deleted
		_, err = clientset.AppsV1().Deployments("default").Get(context.Background(), "my-app", metav1.GetOptions{})
		assert.Error(t, err)

		// Verify service was deleted
		_, err = clientset.CoreV1().Services("default").Get(context.Background(), "my-app", metav1.GetOptions{})
		assert.Error(t, err)
	})

	t.Run("idempotent - no error if resources don't exist", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		deployer := NewKubernetesDeployer(clientset, "default")

		err := deployer.Remove(context.Background(), "nonexistent", false)

		assert.NoError(t, err)
	})
}

func TestKubernetesDeployer_GenerateResourceName(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	deployer := NewKubernetesDeployer(clientset, "default")

	tests := []struct {
		input    string
		expected string
	}{
		{"My Service", "my-service"},
		{"MY_SERVICE", "my-service"},
		{"my--service", "my-service"},
		{"123-service", "svc-123-service"}, // Must start with letter
		{"-service-", "service"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := deployer.GenerateResourceName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
