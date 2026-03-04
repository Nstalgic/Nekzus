package kubernetes

import (
	"context"
	"fmt"

	"github.com/nstalgic/nekzus/internal/runtime"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/client-go/kubernetes/scheme"
)

// ExportFormat defines the output format for export
type ExportFormat string

const (
	// ExportFormatYAML exports as YAML manifests
	ExportFormatYAML ExportFormat = "yaml"
)

// ExportOptions configures export behavior
type ExportOptions struct {
	// Format is the output format (yaml is default)
	Format ExportFormat
	// IncludeSecrets includes ConfigMaps and Secrets
	IncludeSecrets bool
	// IncludeServices includes related Services
	IncludeServices bool
	// SanitizeSecrets replaces secret values with placeholders
	SanitizeSecrets bool
}

// ExportResult contains the exported manifests
type ExportResult struct {
	// Manifests contains the exported Kubernetes manifests
	Manifests []Manifest
	// Warnings contains any non-fatal issues encountered
	Warnings []string
}

// Manifest represents a single Kubernetes manifest
type Manifest struct {
	// Kind is the Kubernetes resource kind
	Kind string
	// Name is the resource name
	Name string
	// Namespace is the resource namespace
	Namespace string
	// Content is the YAML content
	Content string
}

// Export exports pod configurations to Kubernetes YAML manifests
func (r *Runtime) Export(ctx context.Context, ids []runtime.ContainerID, opts ExportOptions) (*ExportResult, error) {
	if len(ids) == 0 {
		return &ExportResult{Manifests: []Manifest{}}, nil
	}

	result := &ExportResult{
		Manifests: make([]Manifest, 0),
		Warnings:  make([]string, 0),
	}

	// Track exported resources to avoid duplicates
	exported := make(map[string]bool)

	for _, id := range ids {
		manifests, warnings, err := r.exportPod(ctx, id, opts, exported)
		if err != nil {
			return nil, err
		}
		result.Manifests = append(result.Manifests, manifests...)
		result.Warnings = append(result.Warnings, warnings...)
	}

	return result, nil
}

// exportPod exports a single pod and its owner resources
func (r *Runtime) exportPod(ctx context.Context, id runtime.ContainerID, opts ExportOptions, exported map[string]bool) ([]Manifest, []string, error) {
	manifests := make([]Manifest, 0)
	warnings := make([]string, 0)

	// Get the pod
	pod, err := r.clientset.CoreV1().Pods(id.Namespace).Get(ctx, id.ID, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil, runtime.NewContainerError(runtime.RuntimeKubernetes, "export", id, runtime.ErrContainerNotFound)
		}
		return nil, nil, runtime.NewContainerError(runtime.RuntimeKubernetes, "export", id, err)
	}

	// Try to find the owner Deployment
	deployment, err := r.findOwnerDeployment(ctx, pod)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("failed to find owner deployment for pod %s: %v", pod.Name, err))
	}

	if deployment != nil {
		// Export the Deployment
		key := fmt.Sprintf("Deployment/%s/%s", deployment.Namespace, deployment.Name)
		if !exported[key] {
			exported[key] = true
			manifest, err := r.toManifest(deployment)
			if err != nil {
				return nil, nil, err
			}
			manifests = append(manifests, manifest)
		}

		// Export related services if requested
		if opts.IncludeServices {
			services, err := r.findRelatedServices(ctx, deployment)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("failed to find services: %v", err))
			} else {
				for _, svc := range services {
					key := fmt.Sprintf("Service/%s/%s", svc.Namespace, svc.Name)
					if !exported[key] {
						exported[key] = true
						manifest, err := r.toManifest(&svc)
						if err != nil {
							warnings = append(warnings, fmt.Sprintf("failed to export service %s: %v", svc.Name, err))
							continue
						}
						manifests = append(manifests, manifest)
					}
				}
			}
		}
	} else {
		// Export the Pod directly (standalone pod)
		key := fmt.Sprintf("Pod/%s/%s", pod.Namespace, pod.Name)
		if !exported[key] {
			exported[key] = true
			// Clean up pod for export
			cleanPod := r.cleanPodForExport(pod)
			manifest, err := r.toManifest(cleanPod)
			if err != nil {
				return nil, nil, err
			}
			manifests = append(manifests, manifest)
			warnings = append(warnings, fmt.Sprintf("pod %s has no owner; exported as standalone Pod", pod.Name))
		}
	}

	return manifests, warnings, nil
}

// findRelatedServices finds Services that select the given Deployment's pods
func (r *Runtime) findRelatedServices(ctx context.Context, deployment *appsv1.Deployment) ([]corev1.Service, error) {
	services, err := r.clientset.CoreV1().Services(deployment.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	// Get deployment's pod labels
	podLabels := deployment.Spec.Template.Labels
	if podLabels == nil {
		return nil, nil
	}

	related := make([]corev1.Service, 0)
	for _, svc := range services.Items {
		if svc.Spec.Selector == nil {
			continue
		}

		// Check if service selector matches pod labels
		matches := true
		for key, value := range svc.Spec.Selector {
			if podLabels[key] != value {
				matches = false
				break
			}
		}

		if matches {
			related = append(related, svc)
		}
	}

	return related, nil
}

// cleanPodForExport removes runtime-specific fields from a Pod
func (r *Runtime) cleanPodForExport(pod *corev1.Pod) *corev1.Pod {
	clean := pod.DeepCopy()

	// Clear status
	clean.Status = corev1.PodStatus{}

	// Clear runtime metadata
	clean.UID = ""
	clean.ResourceVersion = ""
	clean.Generation = 0
	clean.CreationTimestamp = metav1.Time{}
	clean.DeletionTimestamp = nil
	clean.DeletionGracePeriodSeconds = nil
	clean.OwnerReferences = nil
	clean.ManagedFields = nil

	// Clear annotations that are runtime-specific
	if clean.Annotations != nil {
		delete(clean.Annotations, "kubectl.kubernetes.io/last-applied-configuration")
	}

	return clean
}

// toManifest converts a Kubernetes object to a Manifest
func (r *Runtime) toManifest(obj k8sruntime.Object) (Manifest, error) {
	// Create YAML serializer
	serializer := json.NewYAMLSerializer(json.DefaultMetaFactory, scheme.Scheme, scheme.Scheme)

	// Clean up object metadata
	r.cleanMetadata(obj)

	// Set GVK on object for proper serialization
	r.setGVK(obj)

	// Serialize to YAML
	var buf []byte
	var err error
	buf, err = k8sruntime.Encode(serializer, obj)
	if err != nil {
		return Manifest{}, fmt.Errorf("failed to serialize object: %w", err)
	}

	// Get object metadata
	meta, kind := r.getObjectMeta(obj)

	return Manifest{
		Kind:      kind,
		Name:      meta.Name,
		Namespace: meta.Namespace,
		Content:   string(buf),
	}, nil
}

// setGVK sets the GroupVersionKind on Kubernetes objects for proper serialization
func (r *Runtime) setGVK(obj k8sruntime.Object) {
	switch o := obj.(type) {
	case *appsv1.Deployment:
		o.APIVersion = "apps/v1"
		o.Kind = "Deployment"
	case *corev1.Service:
		o.APIVersion = "v1"
		o.Kind = "Service"
	case *corev1.Pod:
		o.APIVersion = "v1"
		o.Kind = "Pod"
	case *corev1.ConfigMap:
		o.APIVersion = "v1"
		o.Kind = "ConfigMap"
	case *corev1.Secret:
		o.APIVersion = "v1"
		o.Kind = "Secret"
	}
}

// cleanMetadata removes runtime-specific metadata from an object
func (r *Runtime) cleanMetadata(obj k8sruntime.Object) {
	switch o := obj.(type) {
	case *appsv1.Deployment:
		o.UID = ""
		o.ResourceVersion = ""
		o.Generation = 0
		o.CreationTimestamp = metav1.Time{}
		o.ManagedFields = nil
		o.Status = appsv1.DeploymentStatus{}
		if o.Annotations != nil {
			delete(o.Annotations, "kubectl.kubernetes.io/last-applied-configuration")
			delete(o.Annotations, "deployment.kubernetes.io/revision")
		}
	case *corev1.Service:
		o.UID = ""
		o.ResourceVersion = ""
		o.Generation = 0
		o.CreationTimestamp = metav1.Time{}
		o.ManagedFields = nil
		o.Status = corev1.ServiceStatus{}
		// Clear cluster IP for export (will be assigned on apply)
		o.Spec.ClusterIP = ""
		o.Spec.ClusterIPs = nil
		if o.Annotations != nil {
			delete(o.Annotations, "kubectl.kubernetes.io/last-applied-configuration")
		}
	case *corev1.Pod:
		o.UID = ""
		o.ResourceVersion = ""
		o.Generation = 0
		o.CreationTimestamp = metav1.Time{}
		o.ManagedFields = nil
		o.Status = corev1.PodStatus{}
		o.OwnerReferences = nil
		if o.Annotations != nil {
			delete(o.Annotations, "kubectl.kubernetes.io/last-applied-configuration")
		}
	}
}

// getObjectMeta extracts metadata and kind from a Kubernetes object
func (r *Runtime) getObjectMeta(obj k8sruntime.Object) (metav1.ObjectMeta, string) {
	switch o := obj.(type) {
	case *appsv1.Deployment:
		return o.ObjectMeta, "Deployment"
	case *corev1.Service:
		return o.ObjectMeta, "Service"
	case *corev1.Pod:
		return o.ObjectMeta, "Pod"
	case *corev1.ConfigMap:
		return o.ObjectMeta, "ConfigMap"
	case *corev1.Secret:
		return o.ObjectMeta, "Secret"
	default:
		return metav1.ObjectMeta{}, "Unknown"
	}
}
