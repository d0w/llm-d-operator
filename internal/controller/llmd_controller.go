/*
Copyright 2025.

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

package controller

import (
	"context"
	"fmt"
	"reflect"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	llmdv1alpha1 "github.com/d0w/llmd-operator/api/v1alpha1"
)

const llmdFinalizer = "llmd.opendatahub.io/finalizer"

const (
	// Real llm-d component names based on llm-d-deployer
	ComponentModelServiceController = "model-service-controller"
	ComponentRedis                  = "redis"
	ComponentInferenceGateway       = "inference-gateway"

	// Real llm-d images from ghcr.io registry
	DefaultModelServiceControllerImage = "ghcr.io/llm-d/llm-d-model-service:0.0.10"
	DefaultInferenceSchedulerImage     = "ghcr.io/llm-d/llm-d-inference-scheduler:0.0.4"
	DefaultLLMDRuntimeImage           = "ghcr.io/llm-d/llm-d:0.0.8"
	DefaultRedisImage                 = "redis:7-alpine"

	// Condition types
	ConditionTypeReady      = "Ready"
	ConditionTypeReconciled = "Reconciled"

	// Phase constants
	PhaseProgressing = "Progressing"
	PhaseReady       = "Ready"
	PhaseFailed      = "Failed"

	// RBAC constants for ModelService controller
	ModelServiceControllerServiceAccount = "modelservice-controller"
	ModelServiceControllerClusterRole    = "modelservice-controller"
	ModelServiceControllerClusterRoleBinding = "modelservice-controller"
)

// LLMDReconciler reconciles a LLMD object
type LLMDReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=llmd.opendatahub.io,resources=llmds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=llmd.opendatahub.io,resources=llmds/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=llmd.opendatahub.io,resources=llmds/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services;configmaps;secrets;serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings;clusterroles;clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways;httproutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *LLMDReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the LLMD instance
	llmd := &llmdv1alpha1.LLMD{}
	if err := r.Get(ctx, req.NamespacedName, llmd); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("LLMD resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get LLMD")
		return ctrl.Result{}, err
	}

	// Initialize status if needed
	if llmd.Status.Phase == "" {
		llmd.Status.Phase = PhaseProgressing
		llmd.Status.ObservedGeneration = llmd.Generation
		if err := r.Status().Update(ctx, llmd); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Handle deletion
	if llmd.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, llmd)
	}

	// Add finalizer if needed
	if !controllerutil.ContainsFinalizer(llmd, llmdFinalizer) {
		controllerutil.AddFinalizer(llmd, llmdFinalizer)
		if err := r.Update(ctx, llmd); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile components
	if err := r.reconcileComponents(ctx, llmd); err != nil {
		r.updateCondition(llmd, ConditionTypeReconciled, metav1.ConditionFalse, "ReconcileError", err.Error())
		llmd.Status.Phase = PhaseFailed
		if statusErr := r.Status().Update(ctx, llmd); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: time.Minute}, err
	}

	// Update status based on component readiness
	if err := r.updateStatus(ctx, llmd); err != nil {
		return ctrl.Result{}, err
	}

	r.updateCondition(llmd, ConditionTypeReconciled, metav1.ConditionTrue, "ReconcileSuccess", "All components reconciled successfully")

	// Update final status
	if err := r.Status().Update(ctx, llmd); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
}

func (r *LLMDReconciler) handleDeletion(ctx context.Context, llmd *llmdv1alpha1.LLMD) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if controllerutil.ContainsFinalizer(llmd, llmdFinalizer) {
		// Cleanup logic would go here if needed
		log.Info("Cleaning up LLMD resources")

		// Remove finalizer
		controllerutil.RemoveFinalizer(llmd, llmdFinalizer)
		if err := r.Update(ctx, llmd); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *LLMDReconciler) reconcileComponents(ctx context.Context, llmd *llmdv1alpha1.LLMD) error {
	targetNamespace := r.getTargetNamespace(llmd)

	// Create namespace if it doesn't exist and is different from current
	if targetNamespace != llmd.Namespace {
		if err := r.ensureNamespace(ctx, targetNamespace); err != nil {
			return fmt.Errorf("failed to ensure namespace %s: %w", targetNamespace, err)
		}
	}

	// Reconcile ConfigMap presets first (needed by other components)
	if err := r.reconcileConfigMapPresets(ctx, llmd, targetNamespace); err != nil {
		return fmt.Errorf("failed to reconcile ConfigMap presets: %w", err)
	}

	// Reconcile Redis if enabled
	if llmd.Spec.Redis.Enabled {
		if err := r.reconcileRedis(ctx, llmd, targetNamespace); err != nil {
			return fmt.Errorf("failed to reconcile Redis: %w", err)
		}
	}

	// Reconcile ModelService controller if enabled
	if llmd.Spec.ModelServiceController.Enabled {
		if err := r.reconcileModelServiceController(ctx, llmd, targetNamespace); err != nil {
			return fmt.Errorf("failed to reconcile ModelService controller: %w", err)
		}
	}

	// Reconcile Gateway if enabled
	if llmd.Spec.Gateway.Enabled {
		if err := r.reconcileGateway(ctx, llmd, targetNamespace); err != nil {
			return fmt.Errorf("failed to reconcile Gateway: %w", err)
		}
	}

	return nil
}

func (r *LLMDReconciler) getTargetNamespace(llmd *llmdv1alpha1.LLMD) string {
	if llmd.Spec.Namespace != "" {
		return llmd.Spec.Namespace
	}
	return llmd.Namespace
}

func (r *LLMDReconciler) ensureNamespace(ctx context.Context, namespace string) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}

	if err := r.Get(ctx, types.NamespacedName{Name: namespace}, ns); err != nil {
		if apierrors.IsNotFound(err) {
			return r.Create(ctx, ns)
		}
		return err
	}
	return nil
}

func (r *LLMDReconciler) reconcileConfigMapPresets(ctx context.Context, llmd *llmdv1alpha1.LLMD, namespace string) error {
	for _, presetName := range llmd.Spec.ConfigMapPresets {
		preset := r.buildConfigMapPreset(llmd, namespace, presetName)
		if err := r.createOrUpdateConfigMap(ctx, llmd, preset); err != nil {
			return fmt.Errorf("failed to reconcile preset %s: %w", presetName, err)
		}
	}
	return nil
}

func (r *LLMDReconciler) buildConfigMapPreset(llmd *llmdv1alpha1.LLMD, namespace, presetName string) *corev1.ConfigMap {
	labels := r.getLabels(llmd, ComponentModelServiceController)

	preset := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      presetName,
			Namespace: namespace,
			Labels:    labels,
		},
		Data: r.getPresetData(presetName),
	}

	return preset
}

func (r *LLMDReconciler) getPresetData(presetName string) map[string]string {
	// These would normally come from the llm-d-deployer repository
	// For now, providing basic presets
	switch presetName {
	case "basic-gpu-preset":
		return map[string]string{
			"vllm-image":            "ghcr.io/llm-d/llm-d:0.0.8",
			"routing-proxy-image":   "ghcr.io/llm-d/llm-d-routing-sidecar:0.0.6",
			"endpoint-picker-image": "ghcr.io/llm-d/llm-d-inference-scheduler:0.0.4",
			"inference-sim-image":   "ghcr.io/llm-d/llm-d-inference-sim:0.0.4",
			"gpu-resources":         "nvidia.com/gpu: 1",
			"tensor-parallel-size":  "1",
		}
	case "basic-gpu-with-nixl-preset":
		return map[string]string{
			"vllm-image":            "ghcr.io/llm-d/llm-d:0.0.8",
			"routing-proxy-image":   "ghcr.io/llm-d/llm-d-routing-sidecar:0.0.6",
			"endpoint-picker-image": "ghcr.io/llm-d/llm-d-inference-scheduler:0.0.4",
			"inference-sim-image":   "ghcr.io/llm-d/llm-d-inference-sim:0.0.4",
			"gpu-resources":         "nvidia.com/gpu: 1",
			"tensor-parallel-size":  "1",
			"nixl-enabled":          "true",
		}
	case "basic-gpu-with-nixl-and-redis-lookup-preset":
		return map[string]string{
			"vllm-image":            "ghcr.io/llm-d/llm-d:0.0.8",
			"routing-proxy-image":   "ghcr.io/llm-d/llm-d-routing-sidecar:0.0.6",
			"endpoint-picker-image": "ghcr.io/llm-d/llm-d-inference-scheduler:0.0.4",
			"inference-sim-image":   "ghcr.io/llm-d/llm-d-inference-sim:0.0.4",
			"gpu-resources":         "nvidia.com/gpu: 1",
			"tensor-parallel-size":  "1",
			"nixl-enabled":          "true",
			"redis-lookup-enabled":  "true",
		}
	default:
		return map[string]string{
			"vllm-image":            "ghcr.io/llm-d/llm-d:0.0.8",
			"routing-proxy-image":   "ghcr.io/llm-d/llm-d-routing-sidecar:0.0.6",
			"endpoint-picker-image": "ghcr.io/llm-d/llm-d-inference-scheduler:0.0.4",
		}
	}
}

func (r *LLMDReconciler) reconcileRedis(ctx context.Context, llmd *llmdv1alpha1.LLMD, namespace string) error {
	// Create ServiceAccount for Redis
	if err := r.reconcileRedisServiceAccount(ctx, llmd, namespace); err != nil {
		return err
	}

	// Create Redis Deployment
	if err := r.reconcileRedisDeployment(ctx, llmd, namespace); err != nil {
		return err
	}

	// Create Redis Service
	if err := r.reconcileRedisService(ctx, llmd, namespace); err != nil {
		return err
	}

	// Create PVC if persistence is enabled
	if llmd.Spec.Redis.Internal.Persistence.Enabled {
		if err := r.reconcileRedisPVC(ctx, llmd, namespace); err != nil {
			return err
		}
	}

		return nil
}

func (r *LLMDReconciler) reconcileRedisServiceAccount(ctx context.Context, llmd *llmdv1alpha1.LLMD, namespace string) error {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "redis",
			Namespace: namespace,
			Labels:    r.getLabels(llmd, ComponentRedis),
		},
	}

	return r.createOrUpdateServiceAccount(ctx, llmd, sa)
}

func (r *LLMDReconciler) reconcileRedisDeployment(ctx context.Context, llmd *llmdv1alpha1.LLMD, namespace string) error {
	image := DefaultRedisImage
	if llmd.Spec.Redis.Internal.Image != "" {
		image = llmd.Spec.Redis.Internal.Image
	}

	labels := r.getLabels(llmd, ComponentRedis)

	volumes := []corev1.Volume{}
	volumeMounts := []corev1.VolumeMount{}

	if llmd.Spec.Redis.Internal.Persistence.Enabled {
		volumes = append(volumes, corev1.Volume{
			Name: "redis-data",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: "redis-data",
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "redis-data",
			MountPath: "/data",
		})
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "redis",
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To[int32](1),
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "redis",
					Containers: []corev1.Container{
						{
							Name:  "redis",
							Image: image,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 6379,
									Name:          "redis",
								},
							},
							Resources:    llmd.Spec.Redis.Internal.Resources,
							VolumeMounts: volumeMounts,
							Args: []string{
								"redis-server",
								"--appendonly", "yes",
								"--maxmemory-policy", "allkeys-lru",
							},
						},
					},
					Volumes: volumes,
				},
			},
		},
	}

	return r.createOrUpdateDeployment(ctx, llmd, deployment)
}

func (r *LLMDReconciler) reconcileRedisService(ctx context.Context, llmd *llmdv1alpha1.LLMD, namespace string) error {
	labels := r.getLabels(llmd, ComponentRedis)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "redis",
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Port:       6379,
					TargetPort: intstr.FromInt(6379),
					Name:       "redis",
				},
			},
		},
	}

	return r.createOrUpdateService(ctx, llmd, service)
}

func (r *LLMDReconciler) reconcileRedisPVC(ctx context.Context, llmd *llmdv1alpha1.LLMD, namespace string) error {
	size := resource.MustParse("8Gi")
	if llmd.Spec.Redis.Internal.Persistence.Size != "" {
		size = resource.MustParse(llmd.Spec.Redis.Internal.Persistence.Size)
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "redis-data",
			Namespace: namespace,
			Labels:    r.getLabels(llmd, ComponentRedis),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: size,
				},
			},
		},
	}

	if llmd.Spec.Redis.Internal.Persistence.StorageClass != "" {
		pvc.Spec.StorageClassName = &llmd.Spec.Redis.Internal.Persistence.StorageClass
	}

	return r.createOrUpdatePVC(ctx, llmd, pvc)
}

func (r *LLMDReconciler) reconcileModelServiceController(ctx context.Context, llmd *llmdv1alpha1.LLMD, namespace string) error {
	// Create ServiceAccount
	if err := r.reconcileModelServiceControllerServiceAccount(ctx, llmd, namespace); err != nil {
		return err
	}

	// Create RBAC
	if err := r.reconcileModelServiceControllerRBAC(ctx, llmd, namespace); err != nil {
		return err
	}

	// Create Deployment
	if err := r.reconcileModelServiceControllerDeployment(ctx, llmd, namespace); err != nil {
		return err
	}

	// Create Service
	if err := r.reconcileModelServiceControllerService(ctx, llmd, namespace); err != nil {
		return err
	}

	return nil
}

func (r *LLMDReconciler) reconcileModelServiceControllerServiceAccount(ctx context.Context, llmd *llmdv1alpha1.LLMD, namespace string) error {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "model-service-controller",
			Namespace: namespace,
			Labels:    r.getLabels(llmd, ComponentModelServiceController),
		},
	}

	return r.createOrUpdateServiceAccount(ctx, llmd, sa)
}

func (r *LLMDReconciler) reconcileModelServiceControllerRBAC(ctx context.Context, llmd *llmdv1alpha1.LLMD, namespace string) error {
	// This would create the necessary ClusterRole and ClusterRoleBinding
	// for the ModelService controller to manage ModelService resources
	// For brevity, implementing a basic version

	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "model-service-controller",
			Labels: r.getLabels(llmd, ComponentModelServiceController),
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"llm-d.ai"},
				Resources: []string{"modelservices"},
				Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
			},
			{
				APIGroups: []string{"apps"},
				Resources: []string{"deployments"},
				Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"services", "configmaps", "secrets"},
				Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
			},
		},
	}

	if err := r.createOrUpdateClusterRole(ctx, llmd, clusterRole); err != nil {
		return err
	}

	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "model-service-controller",
			Labels: r.getLabels(llmd, ComponentModelServiceController),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "model-service-controller",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "model-service-controller",
				Namespace: namespace,
			},
		},
	}

	return r.createOrUpdateClusterRoleBinding(ctx, llmd, clusterRoleBinding)
}

func (r *LLMDReconciler) reconcileModelServiceControllerDeployment(ctx context.Context, llmd *llmdv1alpha1.LLMD, namespace string) error {
	image := DefaultModelServiceControllerImage
	if llmd.Spec.ModelServiceController.Image != "" {
		image = llmd.Spec.ModelServiceController.Image
	}

	replicas := int32(1)
	if llmd.Spec.ModelServiceController.Replicas != nil {
		replicas = *llmd.Spec.ModelServiceController.Replicas
	}

	labels := r.getLabels(llmd, ComponentModelServiceController)
	labels["control-plane"] = "controller-manager"
	labels["app.kubernetes.io/component"] = "modelservice"

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "model-service-controller",
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(replicas),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"control-plane":                "controller-manager",
					"app.kubernetes.io/component":  "modelservice",
					"app.kubernetes.io/name":       llmd.Name,
					"app.kubernetes.io/instance":   llmd.Name,
					"app.kubernetes.io/managed-by": "llmd-operator",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "model-service-controller",
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr.To(true),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{
						{
							Name:    "manager",
							Image:   image,
							Command: []string{"/manager"},
							Args: []string{
								"--leader-elect=false",
								"--health-probe-bind-address=:8081",
								"--epp-cluster-role", "model-service-controller-endpoint-picker",
								"--epp-pull-secrets", "", // Will be populated based on configuration
								"--pd-pull-secrets", "",  // Will be populated based on configuration
							},
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: ptr.To(false),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/healthz",
										Port: intstr.FromInt(8081),
									},
								},
								InitialDelaySeconds: 15,
								PeriodSeconds:       20,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/readyz",
										Port: intstr.FromInt(8081),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       10,
							},
							Resources: func() corev1.ResourceRequirements {
								if llmd.Spec.ModelServiceController.Resources.Requests != nil || llmd.Spec.ModelServiceController.Resources.Limits != nil {
									return llmd.Spec.ModelServiceController.Resources
								}
								// Default resources from real llm-d-deployer
								return corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("100Mi"),
										corev1.ResourceCPU:    resource.MustParse("100m"),
									},
									Limits: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("250Mi"),
										corev1.ResourceCPU:    resource.MustParse("250m"),
									},
								}
							}(),
						},
					},
					Tolerations:  llmd.Spec.ModelServiceController.Tolerations,
					NodeSelector: llmd.Spec.ModelServiceController.NodeSelector,
				},
			},
		},
	}

	return r.createOrUpdateDeployment(ctx, llmd, deployment)
}

func (r *LLMDReconciler) reconcileModelServiceControllerService(ctx context.Context, llmd *llmdv1alpha1.LLMD, namespace string) error {
	labels := r.getLabels(llmd, ComponentModelServiceController)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "model-service-controller",
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Port:       8443,
					TargetPort: intstr.FromInt(8443),
					Name:       "webhook",
				},
				{
					Port:       8080,
					TargetPort: intstr.FromInt(8080),
					Name:       "metrics",
				},
			},
		},
	}

	return r.createOrUpdateService(ctx, llmd, service)
}

func (r *LLMDReconciler) reconcileGateway(ctx context.Context, llmd *llmdv1alpha1.LLMD, namespace string) error {
	// Gateway reconciliation would create Gateway API resources
	// This is a placeholder implementation
	return nil
}

// Helper functions for creating/updating resources

func (r *LLMDReconciler) createOrUpdateConfigMap(ctx context.Context, llmd *llmdv1alpha1.LLMD, desired *corev1.ConfigMap) error {
	current := &corev1.ConfigMap{}
	key := types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}

	if err := controllerutil.SetControllerReference(llmd, desired, r.Scheme); err != nil {
	return err
}

	if err := r.Get(ctx, key, current); err != nil {
		if apierrors.IsNotFound(err) {
			return r.Create(ctx, desired)
		}
		return err
	}

	if !reflect.DeepEqual(current.Data, desired.Data) {
		current.Data = desired.Data
		current.Labels = desired.Labels
		return r.Update(ctx, current)
	}

	return nil
}

func (r *LLMDReconciler) createOrUpdateDeployment(ctx context.Context, llmd *llmdv1alpha1.LLMD, desired *appsv1.Deployment) error {
	current := &appsv1.Deployment{}
	key := types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}

	if err := controllerutil.SetControllerReference(llmd, desired, r.Scheme); err != nil {
		return err
	}

	if err := r.Get(ctx, key, current); err != nil {
		if apierrors.IsNotFound(err) {
			return r.Create(ctx, desired)
		}
		return err
	}

	// Update deployment if changed
	current.Spec = desired.Spec
	current.Labels = desired.Labels
	return r.Update(ctx, current)
}

func (r *LLMDReconciler) createOrUpdateService(ctx context.Context, llmd *llmdv1alpha1.LLMD, desired *corev1.Service) error {
	current := &corev1.Service{}
	key := types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}

	if err := controllerutil.SetControllerReference(llmd, desired, r.Scheme); err != nil {
		return err
	}

	if err := r.Get(ctx, key, current); err != nil {
		if apierrors.IsNotFound(err) {
			return r.Create(ctx, desired)
	}
	return err
}

	// Update service if changed
	current.Spec.Ports = desired.Spec.Ports
	current.Spec.Selector = desired.Spec.Selector
	current.Labels = desired.Labels
	return r.Update(ctx, current)
}

func (r *LLMDReconciler) createOrUpdateServiceAccount(ctx context.Context, llmd *llmdv1alpha1.LLMD, desired *corev1.ServiceAccount) error {
	current := &corev1.ServiceAccount{}
	key := types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}

	if err := controllerutil.SetControllerReference(llmd, desired, r.Scheme); err != nil {
		return err
	}

	if err := r.Get(ctx, key, current); err != nil {
		if apierrors.IsNotFound(err) {
			return r.Create(ctx, desired)
	}
	return err
}

		return nil
	}

func (r *LLMDReconciler) createOrUpdatePVC(ctx context.Context, llmd *llmdv1alpha1.LLMD, desired *corev1.PersistentVolumeClaim) error {
	current := &corev1.PersistentVolumeClaim{}
	key := types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}

	if err := controllerutil.SetControllerReference(llmd, desired, r.Scheme); err != nil {
			return err
		}

	if err := r.Get(ctx, key, current); err != nil {
		if apierrors.IsNotFound(err) {
			return r.Create(ctx, desired)
		}
		return err
	}

	return nil
}

func (r *LLMDReconciler) createOrUpdateClusterRole(ctx context.Context, llmd *llmdv1alpha1.LLMD, desired *rbacv1.ClusterRole) error {
	current := &rbacv1.ClusterRole{}
	key := types.NamespacedName{Name: desired.Name}

	if err := r.Get(ctx, key, current); err != nil {
		if apierrors.IsNotFound(err) {
			return r.Create(ctx, desired)
		}
		return err
	}

	current.Rules = desired.Rules
	current.Labels = desired.Labels
	return r.Update(ctx, current)
}

func (r *LLMDReconciler) createOrUpdateClusterRoleBinding(ctx context.Context, llmd *llmdv1alpha1.LLMD, desired *rbacv1.ClusterRoleBinding) error {
	current := &rbacv1.ClusterRoleBinding{}
	key := types.NamespacedName{Name: desired.Name}

	if err := r.Get(ctx, key, current); err != nil {
		if apierrors.IsNotFound(err) {
			return r.Create(ctx, desired)
	}
	return err
	}

	current.RoleRef = desired.RoleRef
	current.Subjects = desired.Subjects
	current.Labels = desired.Labels
	return r.Update(ctx, current)
}

func (r *LLMDReconciler) updateStatus(ctx context.Context, llmd *llmdv1alpha1.LLMD) error {
	targetNamespace := r.getTargetNamespace(llmd)

	// Check ModelService controller status
	if llmd.Spec.ModelServiceController.Enabled {
		ready, message := r.checkDeploymentStatus(ctx, "model-service-controller", targetNamespace)
		llmd.Status.ModelServiceController = llmdv1alpha1.ComponentStatus{
			Ready:       ready,
			Message:     message,
			LastUpdated: &metav1.Time{Time: time.Now()},
		}
	}

	// Check Redis status
	if llmd.Spec.Redis.Enabled {
		ready, message := r.checkDeploymentStatus(ctx, "redis", targetNamespace)
		llmd.Status.Redis = llmdv1alpha1.ComponentStatus{
			Ready:       ready,
			Message:     message,
			LastUpdated: &metav1.Time{Time: time.Now()},
		}
	}

	// Check ConfigMap presets status
	llmd.Status.ConfigMapPresets = []llmdv1alpha1.ConfigMapPresetStatus{}
	for _, presetName := range llmd.Spec.ConfigMapPresets {
		ready, message := r.checkConfigMapStatus(ctx, presetName, targetNamespace)
		llmd.Status.ConfigMapPresets = append(llmd.Status.ConfigMapPresets, llmdv1alpha1.ConfigMapPresetStatus{
			Name:    presetName,
			Ready:   ready,
			Message: message,
		})
	}

	// Determine overall phase
	allReady := true
	if llmd.Spec.ModelServiceController.Enabled && !llmd.Status.ModelServiceController.Ready {
		allReady = false
	}
	if llmd.Spec.Redis.Enabled && !llmd.Status.Redis.Ready {
		allReady = false
	}

	if allReady {
		llmd.Status.Phase = PhaseReady
	} else {
		llmd.Status.Phase = PhaseProgressing
	}

	llmd.Status.ObservedGeneration = llmd.Generation

	return nil
}

func (r *LLMDReconciler) checkDeploymentStatus(ctx context.Context, name, namespace string) (bool, string) {
	deployment := &appsv1.Deployment{}
	key := types.NamespacedName{Name: name, Namespace: namespace}

	if err := r.Get(ctx, key, deployment); err != nil {
		return false, fmt.Sprintf("Deployment not found: %v", err)
	}

	if deployment.Status.ReadyReplicas == *deployment.Spec.Replicas {
		return true, "Ready"
	}

	return false, fmt.Sprintf("Waiting for replicas: %d/%d ready", deployment.Status.ReadyReplicas, *deployment.Spec.Replicas)
}

func (r *LLMDReconciler) checkConfigMapStatus(ctx context.Context, name, namespace string) (bool, string) {
	cm := &corev1.ConfigMap{}
	key := types.NamespacedName{Name: name, Namespace: namespace}

	if err := r.Get(ctx, key, cm); err != nil {
		return false, fmt.Sprintf("ConfigMap not found: %v", err)
	}

	return true, "Ready"
}

func (r *LLMDReconciler) getLabels(llmd *llmdv1alpha1.LLMD, component string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "llmd",
		"app.kubernetes.io/instance":   llmd.Name,
		"app.kubernetes.io/component":  component,
		"app.kubernetes.io/managed-by": "llmd-operator",
	}
}

func (r *LLMDReconciler) updateCondition(llmd *llmdv1alpha1.LLMD, condType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               condType,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}

	meta.SetStatusCondition(&llmd.Status.Conditions, condition)
}

// SetupWithManager sets up the controller with the Manager.
func (r *LLMDReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&llmdv1alpha1.LLMD{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Complete(r)
}
