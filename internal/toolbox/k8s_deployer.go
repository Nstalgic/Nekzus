package toolbox

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/nstalgic/nekzus/internal/types"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

// KubernetesDeployer handles Kubernetes deployment for toolbox services.
type KubernetesDeployer struct {
	clientset kubernetes.Interface
	namespace string
}

// NewKubernetesDeployer creates a new Kubernetes deployer.
func NewKubernetesDeployer(clientset kubernetes.Interface, namespace string) *KubernetesDeployer {
	if namespace == "" {
		namespace = "default"
	}
	return &KubernetesDeployer{
		clientset: clientset,
		namespace: namespace,
	}
}

// Deploy creates Kubernetes Deployment and Service resources.
// Returns the deployment name as the identifier.
func (d *KubernetesDeployer) Deploy(ctx context.Context, template *types.ServiceTemplate, deployment *types.ToolboxDeployment) (string, error) {
	if template == nil {
		return "", fmt.Errorf("template cannot be nil")
	}
	if deployment == nil {
		return "", fmt.Errorf("deployment cannot be nil")
	}

	// Generate resource name
	name := d.GenerateResourceName(deployment.ServiceName)

	// Determine image
	image := template.DockerConfig.Image
	if deployment.CustomImage != "" {
		image = deployment.CustomImage
	}

	// Build environment variables
	envVars := d.buildEnvironmentVars(template, deployment)

	// Build container ports
	containerPorts := d.buildContainerPorts(template)

	// Create labels
	labels := map[string]string{
		"app":                         name,
		"nekzus.toolbox":          "true",
		"nekzus.toolbox.template": template.ID,
	}

	// Create Deployment
	replicas := int32(0) // Start with 0, will be scaled up by Start()
	k8sDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: d.namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  name,
							Image: image,
							Env:   envVars,
							Ports: containerPorts,
						},
					},
				},
			},
		},
	}

	// Create the Deployment
	_, err := d.clientset.AppsV1().Deployments(d.namespace).Create(ctx, k8sDeployment, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to create deployment: %w", err)
	}

	// Create Service if ports are defined
	if len(template.DockerConfig.Ports) > 0 {
		if err := d.createService(ctx, name, labels, template.DockerConfig.Ports); err != nil {
			// Clean up deployment on service creation failure
			_ = d.clientset.AppsV1().Deployments(d.namespace).Delete(ctx, name, metav1.DeleteOptions{})
			return "", fmt.Errorf("failed to create service: %w", err)
		}
	}

	return name, nil
}

// createService creates a Kubernetes Service for the deployment.
func (d *KubernetesDeployer) createService(ctx context.Context, name string, labels map[string]string, ports []types.PortMapping) error {
	servicePorts := make([]corev1.ServicePort, 0, len(ports))
	for i, p := range ports {
		protocol := corev1.ProtocolTCP
		if strings.ToLower(p.Protocol) == "udp" {
			protocol = corev1.ProtocolUDP
		}

		portName := fmt.Sprintf("port-%d", i)

		servicePorts = append(servicePorts, corev1.ServicePort{
			Name:       portName,
			Port:       int32(p.Container),
			TargetPort: intstr.FromInt(p.Container),
			Protocol:   protocol,
		})
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: d.namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": name},
			Ports:    servicePorts,
			Type:     corev1.ServiceTypeClusterIP,
		},
	}

	_, err := d.clientset.CoreV1().Services(d.namespace).Create(ctx, service, metav1.CreateOptions{})
	return err
}

// buildEnvironmentVars builds environment variables from template and deployment.
func (d *KubernetesDeployer) buildEnvironmentVars(template *types.ServiceTemplate, deployment *types.ToolboxDeployment) []corev1.EnvVar {
	envVars := make([]corev1.EnvVar, 0)

	// Add template defaults
	for key, value := range template.DockerConfig.Environment {
		envVars = append(envVars, corev1.EnvVar{
			Name:  key,
			Value: value,
		})
	}

	// Add/override with user-provided vars
	for key, value := range deployment.EnvVars {
		found := false
		for i, env := range envVars {
			if env.Name == key {
				envVars[i].Value = value
				found = true
				break
			}
		}
		if !found {
			envVars = append(envVars, corev1.EnvVar{
				Name:  key,
				Value: value,
			})
		}
	}

	return envVars
}

// buildContainerPorts builds container port definitions.
func (d *KubernetesDeployer) buildContainerPorts(template *types.ServiceTemplate) []corev1.ContainerPort {
	ports := make([]corev1.ContainerPort, 0, len(template.DockerConfig.Ports))
	for _, p := range template.DockerConfig.Ports {
		protocol := corev1.ProtocolTCP
		if strings.ToLower(p.Protocol) == "udp" {
			protocol = corev1.ProtocolUDP
		}

		ports = append(ports, corev1.ContainerPort{
			ContainerPort: int32(p.Container),
			Protocol:      protocol,
		})
	}
	return ports
}

// Start scales the deployment to 1 replica.
func (d *KubernetesDeployer) Start(ctx context.Context, name string) error {
	return d.scaleDeployment(ctx, name, 1)
}

// Stop scales the deployment to 0 replicas.
func (d *KubernetesDeployer) Stop(ctx context.Context, name string) error {
	return d.scaleDeployment(ctx, name, 0)
}

// scaleDeployment updates the replica count of a deployment.
func (d *KubernetesDeployer) scaleDeployment(ctx context.Context, name string, replicas int32) error {
	deployment, err := d.clientset.AppsV1().Deployments(d.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get deployment: %w", err)
	}

	deployment.Spec.Replicas = &replicas
	_, err = d.clientset.AppsV1().Deployments(d.namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to scale deployment: %w", err)
	}

	return nil
}

// Remove deletes the deployment and associated resources.
func (d *KubernetesDeployer) Remove(ctx context.Context, name string, removeVolumes bool) error {
	// Delete Deployment
	err := d.clientset.AppsV1().Deployments(d.namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete deployment: %w", err)
	}

	// Delete Service
	err = d.clientset.CoreV1().Services(d.namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete service: %w", err)
	}

	// If removeVolumes is true, delete associated PVCs (by label)
	if removeVolumes {
		pvcList, err := d.clientset.CoreV1().PersistentVolumeClaims(d.namespace).List(ctx, metav1.ListOptions{
			LabelSelector: fmt.Sprintf("app=%s", name),
		})
		if err == nil {
			for _, pvc := range pvcList.Items {
				_ = d.clientset.CoreV1().PersistentVolumeClaims(d.namespace).Delete(ctx, pvc.Name, metav1.DeleteOptions{})
			}
		}
	}

	return nil
}

// GenerateResourceName generates a valid Kubernetes resource name from a service name.
func (d *KubernetesDeployer) GenerateResourceName(serviceName string) string {
	// Convert to lowercase
	name := strings.ToLower(serviceName)

	// Replace spaces and underscores with hyphens
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "_", "-")

	// Remove special characters (keep only alphanumeric and hyphens)
	reg := regexp.MustCompile(`[^a-z0-9-]`)
	name = reg.ReplaceAllString(name, "")

	// Remove consecutive hyphens
	reg = regexp.MustCompile(`-+`)
	name = reg.ReplaceAllString(name, "-")

	// Trim hyphens from start and end
	name = strings.Trim(name, "-")

	// Kubernetes names must start with a letter
	if len(name) > 0 && (name[0] >= '0' && name[0] <= '9') {
		name = "svc-" + name
	}

	// Limit to 63 characters (Kubernetes limit)
	if len(name) > 63 {
		name = name[:63]
	}

	// Ensure it doesn't end with a hyphen after truncation
	name = strings.TrimRight(name, "-")

	return name
}

// ValidateDeployment validates a deployment configuration for Kubernetes.
func (d *KubernetesDeployer) ValidateDeployment(template *types.ServiceTemplate, envVars map[string]string) error {
	if template == nil {
		return fmt.Errorf("template cannot be nil")
	}

	// Check if using Compose (not supported for K8s deployer)
	if template.ComposeProject != nil {
		return fmt.Errorf("Compose projects are not supported by Kubernetes deployer")
	}

	// Validate image is set
	if template.DockerConfig.Image == "" {
		return fmt.Errorf("image is required")
	}

	return nil
}

// Close is a no-op for Kubernetes deployer (clientset lifecycle managed externally).
func (d *KubernetesDeployer) Close() error {
	return nil
}
