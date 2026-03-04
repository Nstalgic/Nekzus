// Package kubernetes provides a Kubernetes implementation of the runtime.Runtime interface
package kubernetes

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/nstalgic/nekzus/internal/runtime"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

// Runtime implements runtime.Runtime for Kubernetes
type Runtime struct {
	clientset     kubernetes.Interface
	metricsClient metricsv.Interface
	namespaces    []string
}

// Ensure Runtime implements runtime.Runtime
var _ runtime.Runtime = (*Runtime)(nil)

// Config holds Kubernetes runtime configuration
type Config struct {
	// Namespaces to watch (empty = all namespaces)
	Namespaces []string
}

// NewRuntime creates a new Kubernetes runtime
func NewRuntime(clientset kubernetes.Interface, metricsClient metricsv.Interface) *Runtime {
	return &Runtime{
		clientset:     clientset,
		metricsClient: metricsClient,
	}
}

// NewRuntimeWithConfig creates a new Kubernetes runtime with configuration
func NewRuntimeWithConfig(clientset kubernetes.Interface, metricsClient metricsv.Interface, cfg *Config) *Runtime {
	rt := &Runtime{
		clientset:     clientset,
		metricsClient: metricsClient,
	}
	if cfg != nil {
		rt.namespaces = cfg.Namespaces
	}
	return rt
}

// Name returns the runtime name
func (r *Runtime) Name() string {
	return "Kubernetes"
}

// Type returns the runtime type
func (r *Runtime) Type() runtime.RuntimeType {
	return runtime.RuntimeKubernetes
}

// Ping checks if Kubernetes API is available
func (r *Runtime) Ping(ctx context.Context) error {
	_, err := r.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		return runtime.NewRuntimeError(runtime.RuntimeKubernetes, "ping", runtime.ErrRuntimeUnavailable)
	}
	return nil
}

// Close releases any resources (no-op for K8s)
func (r *Runtime) Close() error {
	return nil
}

// List returns pods matching the options
func (r *Runtime) List(ctx context.Context, opts runtime.ListOptions) ([]runtime.Container, error) {
	namespace := opts.Namespace
	if namespace == "" {
		namespace = metav1.NamespaceAll
	}

	pods, err := r.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, runtime.NewRuntimeError(runtime.RuntimeKubernetes, "list", err)
	}

	containers := make([]runtime.Container, 0, len(pods.Items))
	for _, pod := range pods.Items {
		// Skip non-running pods unless All is true
		if !opts.All && pod.Status.Phase != corev1.PodRunning {
			continue
		}

		containers = append(containers, convertPod(&pod))
	}

	return containers, nil
}

// Start starts a pod by scaling its parent deployment to 1
func (r *Runtime) Start(ctx context.Context, id runtime.ContainerID) error {
	// Get the pod to find its owner
	pod, err := r.clientset.CoreV1().Pods(id.Namespace).Get(ctx, id.ID, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return runtime.NewContainerError(runtime.RuntimeKubernetes, "start", id, runtime.ErrContainerNotFound)
		}
		return runtime.NewContainerError(runtime.RuntimeKubernetes, "start", id, err)
	}

	// Find the owner deployment and scale it
	deployment, err := r.findOwnerDeployment(ctx, pod)
	if err != nil {
		return runtime.NewContainerError(runtime.RuntimeKubernetes, "start", id, err)
	}

	if deployment == nil {
		// No deployment found, cannot scale standalone pod
		return runtime.NewContainerError(runtime.RuntimeKubernetes, "start", id,
			fmt.Errorf("pod has no deployment owner, cannot scale"))
	}

	// Scale to 1 replica
	replicas := int32(1)
	deployment.Spec.Replicas = &replicas

	_, err = r.clientset.AppsV1().Deployments(deployment.Namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	if err != nil {
		return runtime.NewContainerError(runtime.RuntimeKubernetes, "start", id, err)
	}

	return nil
}

// Stop stops a pod by scaling its parent deployment to 0
func (r *Runtime) Stop(ctx context.Context, id runtime.ContainerID, timeout *time.Duration) error {
	// Get the pod to find its owner
	pod, err := r.clientset.CoreV1().Pods(id.Namespace).Get(ctx, id.ID, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return runtime.NewContainerError(runtime.RuntimeKubernetes, "stop", id, runtime.ErrContainerNotFound)
		}
		return runtime.NewContainerError(runtime.RuntimeKubernetes, "stop", id, err)
	}

	// Find the owner deployment and scale it
	deployment, err := r.findOwnerDeployment(ctx, pod)
	if err != nil {
		return runtime.NewContainerError(runtime.RuntimeKubernetes, "stop", id, err)
	}

	if deployment == nil {
		// No deployment, delete the pod directly
		err = r.clientset.CoreV1().Pods(id.Namespace).Delete(ctx, id.ID, metav1.DeleteOptions{})
		if err != nil {
			return runtime.NewContainerError(runtime.RuntimeKubernetes, "stop", id, err)
		}
		return nil
	}

	// Scale to 0 replicas
	replicas := int32(0)
	deployment.Spec.Replicas = &replicas

	_, err = r.clientset.AppsV1().Deployments(deployment.Namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	if err != nil {
		return runtime.NewContainerError(runtime.RuntimeKubernetes, "stop", id, err)
	}

	return nil
}

// Restart restarts a pod by deleting it (controller will recreate)
func (r *Runtime) Restart(ctx context.Context, id runtime.ContainerID, timeout *time.Duration) error {
	// Delete the pod - if it's managed by a controller, it will be recreated
	err := r.clientset.CoreV1().Pods(id.Namespace).Delete(ctx, id.ID, metav1.DeleteOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return runtime.NewContainerError(runtime.RuntimeKubernetes, "restart", id, runtime.ErrContainerNotFound)
		}
		return runtime.NewContainerError(runtime.RuntimeKubernetes, "restart", id, err)
	}

	return nil
}

// Inspect returns detailed pod information
func (r *Runtime) Inspect(ctx context.Context, id runtime.ContainerID) (*runtime.ContainerDetails, error) {
	pod, err := r.clientset.CoreV1().Pods(id.Namespace).Get(ctx, id.ID, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, runtime.NewContainerError(runtime.RuntimeKubernetes, "inspect", id, runtime.ErrContainerNotFound)
		}
		return nil, runtime.NewContainerError(runtime.RuntimeKubernetes, "inspect", id, err)
	}

	return convertPodDetails(pod), nil
}

// GetStats returns pod resource usage statistics
func (r *Runtime) GetStats(ctx context.Context, id runtime.ContainerID) (*runtime.Stats, error) {
	// Check if metrics client is available
	if r.metricsClient == nil {
		return nil, runtime.NewContainerError(runtime.RuntimeKubernetes, "stats", id, runtime.ErrMetricsUnavailable)
	}

	// Verify pod exists first
	pod, err := r.clientset.CoreV1().Pods(id.Namespace).Get(ctx, id.ID, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, runtime.NewContainerError(runtime.RuntimeKubernetes, "stats", id, runtime.ErrContainerNotFound)
		}
		return nil, runtime.NewContainerError(runtime.RuntimeKubernetes, "stats", id, err)
	}

	// Get pod metrics
	podMetrics, err := r.metricsClient.MetricsV1beta1().PodMetricses(id.Namespace).Get(ctx, id.ID, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, runtime.NewContainerError(runtime.RuntimeKubernetes, "stats", id, runtime.ErrMetricsUnavailable)
		}
		return nil, runtime.NewContainerError(runtime.RuntimeKubernetes, "stats", id, err)
	}

	// Aggregate metrics from all containers
	var totalCPU, totalMemory int64
	for _, container := range podMetrics.Containers {
		totalCPU += container.Usage.Cpu().MilliValue()
		totalMemory += container.Usage.Memory().Value()
	}

	// Get resource limits for percentage calculation
	var cpuLimit, memoryLimit int64
	for _, container := range pod.Spec.Containers {
		if limit := container.Resources.Limits.Cpu(); limit != nil {
			cpuLimit += limit.MilliValue()
		}
		if limit := container.Resources.Limits.Memory(); limit != nil {
			memoryLimit += limit.Value()
		}
	}

	// Calculate percentages
	cpuPercent := 0.0
	if cpuLimit > 0 {
		cpuPercent = float64(totalCPU) / float64(cpuLimit) * 100.0
	}

	memoryPercent := 0.0
	memoryAvailable := uint64(0)
	if memoryLimit > 0 {
		memoryPercent = float64(totalMemory) / float64(memoryLimit) * 100.0
		if memoryLimit > totalMemory {
			memoryAvailable = uint64(memoryLimit - totalMemory)
		}
	}

	return &runtime.Stats{
		ContainerID: id,
		CPU: runtime.CPUStats{
			Usage:      cpuPercent,
			CoresUsed:  float64(totalCPU) / 1000.0, // Convert millicores to cores
			TotalCores: float64(cpuLimit) / 1000.0,
		},
		Memory: runtime.MemoryStats{
			Usage:     memoryPercent,
			Used:      uint64(totalMemory),
			Limit:     uint64(memoryLimit),
			Available: memoryAvailable,
		},
		Network: runtime.NetworkStats{
			// K8s metrics don't include network stats
			RxBytes: 0,
			TxBytes: 0,
		},
		Timestamp: time.Now().Unix(),
	}, nil
}

// GetBatchStats returns stats for multiple pods
func (r *Runtime) GetBatchStats(ctx context.Context, ids []runtime.ContainerID) ([]runtime.Stats, error) {
	if len(ids) == 0 {
		return []runtime.Stats{}, nil
	}

	results := make([]runtime.Stats, 0, len(ids))
	for _, id := range ids {
		stats, err := r.GetStats(ctx, id)
		if err != nil {
			// Skip pods that fail to get stats
			continue
		}
		results = append(results, *stats)
	}

	return results, nil
}

// BulkRestart restarts multiple pods concurrently
func (r *Runtime) BulkRestart(ctx context.Context, ids []runtime.ContainerID, timeout *time.Duration) ([]runtime.BulkResult, error) {
	if len(ids) == 0 {
		return []runtime.BulkResult{}, nil
	}

	// Use a semaphore to limit concurrency
	const maxConcurrency = 10
	sem := make(chan struct{}, maxConcurrency)

	results := make([]runtime.BulkResult, len(ids))
	var wg sync.WaitGroup

	for i, id := range ids {
		wg.Add(1)
		go func(idx int, containerID runtime.ContainerID) {
			defer wg.Done()

			// Acquire semaphore
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[idx] = runtime.BulkResult{
					ContainerID: containerID,
					Success:     false,
					Error:       ctx.Err().Error(),
				}
				return
			}

			// Perform restart
			err := r.Restart(ctx, containerID, timeout)
			if err != nil {
				results[idx] = runtime.BulkResult{
					ContainerID: containerID,
					Success:     false,
					Error:       err.Error(),
				}
			} else {
				results[idx] = runtime.BulkResult{
					ContainerID: containerID,
					Success:     true,
				}
			}
		}(i, id)
	}

	wg.Wait()
	return results, nil
}

// StreamLogs returns a reader for pod logs
func (r *Runtime) StreamLogs(ctx context.Context, id runtime.ContainerID, opts runtime.LogOptions) (io.ReadCloser, error) {
	// Verify pod exists
	pod, err := r.clientset.CoreV1().Pods(id.Namespace).Get(ctx, id.ID, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, runtime.NewContainerError(runtime.RuntimeKubernetes, "logs", id, runtime.ErrContainerNotFound)
		}
		return nil, runtime.NewContainerError(runtime.RuntimeKubernetes, "logs", id, err)
	}

	// Determine container name
	containerName := opts.Container
	if containerName == "" && len(pod.Spec.Containers) > 0 {
		containerName = pod.Spec.Containers[0].Name
	}

	// Build log options
	logOpts := &corev1.PodLogOptions{
		Container:  containerName,
		Follow:     opts.Follow,
		Timestamps: opts.Timestamps,
	}

	if opts.Tail > 0 {
		tailLines := opts.Tail
		logOpts.TailLines = &tailLines
	}

	if !opts.Since.IsZero() {
		sinceTime := metav1.NewTime(opts.Since)
		logOpts.SinceTime = &sinceTime
	}

	// Get log stream
	req := r.clientset.CoreV1().Pods(id.Namespace).GetLogs(id.ID, logOpts)
	stream, err := req.Stream(ctx)
	if err != nil {
		return nil, runtime.NewContainerError(runtime.RuntimeKubernetes, "logs", id, err)
	}

	return stream, nil
}

// findOwnerDeployment finds the Deployment that owns a pod (via ReplicaSet)
func (r *Runtime) findOwnerDeployment(ctx context.Context, pod *corev1.Pod) (*appsv1.Deployment, error) {
	// First, find the ReplicaSet owner
	var rsName string
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "ReplicaSet" {
			rsName = owner.Name
			break
		}
	}

	if rsName == "" {
		// No ReplicaSet owner - check for direct Deployment owner (rare but possible)
		for _, owner := range pod.OwnerReferences {
			if owner.Kind == "Deployment" {
				deployment, err := r.clientset.AppsV1().Deployments(pod.Namespace).Get(ctx, owner.Name, metav1.GetOptions{})
				if err != nil {
					return nil, err
				}
				return deployment, nil
			}
		}
		return nil, nil
	}

	// Get the ReplicaSet
	rs, err := r.clientset.AppsV1().ReplicaSets(pod.Namespace).Get(ctx, rsName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	// Find the Deployment owner of the ReplicaSet
	for _, owner := range rs.OwnerReferences {
		if owner.Kind == "Deployment" {
			deployment, err := r.clientset.AppsV1().Deployments(pod.Namespace).Get(ctx, owner.Name, metav1.GetOptions{})
			if err != nil {
				return nil, err
			}
			return deployment, nil
		}
	}

	return nil, nil
}

// convertPod converts a Kubernetes Pod to runtime.Container
func convertPod(pod *corev1.Pod) runtime.Container {
	// Get primary image (first container)
	image := ""
	if len(pod.Spec.Containers) > 0 {
		image = pod.Spec.Containers[0].Image
	}

	// Convert ports from all containers
	var ports []runtime.PortBinding
	for _, container := range pod.Spec.Containers {
		for _, port := range container.Ports {
			ports = append(ports, runtime.PortBinding{
				PrivatePort: int(port.ContainerPort),
				Protocol:    string(port.Protocol),
			})
		}
	}

	// Build status string
	status := buildPodStatus(pod)

	return runtime.Container{
		ID: runtime.ContainerID{
			Runtime:   runtime.RuntimeKubernetes,
			ID:        pod.Name,
			Namespace: pod.Namespace,
		},
		Name:    pod.Name,
		Image:   image,
		State:   convertPodPhase(pod.Status.Phase),
		Status:  status,
		Created: pod.CreationTimestamp.Unix(),
		Ports:   ports,
		Labels:  pod.Labels,
	}
}

// convertPodDetails converts a Kubernetes Pod to runtime.ContainerDetails
func convertPodDetails(pod *corev1.Pod) *runtime.ContainerDetails {
	container := convertPod(pod)

	details := &runtime.ContainerDetails{
		Container: container,
		Raw:       pod,
	}

	// Build config from first container
	if len(pod.Spec.Containers) > 0 {
		c := pod.Spec.Containers[0]
		var env []string
		for _, e := range c.Env {
			if e.Value != "" {
				env = append(env, fmt.Sprintf("%s=%s", e.Name, e.Value))
			} else if e.ValueFrom != nil {
				env = append(env, fmt.Sprintf("%s=(from secret/configmap)", e.Name))
			}
		}

		details.Config = runtime.ContainerConfig{
			Env:        env,
			Cmd:        c.Command,
			WorkingDir: c.WorkingDir,
		}
	}

	// Network settings
	details.NetworkSettings = &runtime.NetworkSettings{
		IPAddress: pod.Status.PodIP,
	}

	// Mounts
	for _, vol := range pod.Spec.Volumes {
		mount := runtime.Mount{
			Type:   getVolumeType(vol),
			Source: getVolumeSource(vol),
		}
		// Find corresponding volume mount
		for _, container := range pod.Spec.Containers {
			for _, vm := range container.VolumeMounts {
				if vm.Name == vol.Name {
					mount.Destination = vm.MountPath
					mount.ReadOnly = vm.ReadOnly
					break
				}
			}
		}
		details.Mounts = append(details.Mounts, mount)
	}

	return details
}

// convertPodPhase converts Kubernetes PodPhase to runtime.ContainerState
func convertPodPhase(phase corev1.PodPhase) runtime.ContainerState {
	switch phase {
	case corev1.PodRunning:
		return runtime.StateRunning
	case corev1.PodPending:
		return runtime.StatePending
	case corev1.PodSucceeded:
		return runtime.StateStopped
	case corev1.PodFailed:
		return runtime.StateFailed
	case corev1.PodUnknown:
		return runtime.StateStopped
	default:
		return runtime.StateStopped
	}
}

// buildPodStatus creates a human-readable status string
func buildPodStatus(pod *corev1.Pod) string {
	if pod.Status.Phase == corev1.PodRunning {
		// Count ready containers
		ready := 0
		total := len(pod.Status.ContainerStatuses)
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.Ready {
				ready++
			}
		}
		return fmt.Sprintf("Running (%d/%d ready)", ready, total)
	}

	// Check for waiting reasons
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Waiting != nil {
			return cs.State.Waiting.Reason
		}
		if cs.State.Terminated != nil {
			return fmt.Sprintf("Terminated: %s", cs.State.Terminated.Reason)
		}
	}

	return string(pod.Status.Phase)
}

// getVolumeType returns the volume type
func getVolumeType(vol corev1.Volume) string {
	switch {
	case vol.PersistentVolumeClaim != nil:
		return "persistentVolumeClaim"
	case vol.ConfigMap != nil:
		return "configMap"
	case vol.Secret != nil:
		return "secret"
	case vol.EmptyDir != nil:
		return "emptyDir"
	case vol.HostPath != nil:
		return "hostPath"
	default:
		return "unknown"
	}
}

// getVolumeSource returns the volume source identifier
func getVolumeSource(vol corev1.Volume) string {
	switch {
	case vol.PersistentVolumeClaim != nil:
		return vol.PersistentVolumeClaim.ClaimName
	case vol.ConfigMap != nil:
		return vol.ConfigMap.Name
	case vol.Secret != nil:
		return vol.Secret.SecretName
	case vol.HostPath != nil:
		return vol.HostPath.Path
	default:
		return vol.Name
	}
}

// isNotFoundError checks if an error indicates the resource was not found
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "not found") || errors.IsNotFound(err)
}
