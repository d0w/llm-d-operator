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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  This is scaffolding for you to own.
// NOTE: json tags are required.  Any new fields you add must have json:"-" or json:"fieldName" tags for the fields to be serialized.

// LLMDSpec defines the desired state of LLMD
// LLMD is a meta-operator that deploys llm-d infrastructure components
type LLMDSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// ModelServiceController configures the llm-d ModelService controller deployment
	// +optional
	ModelServiceController ModelServiceControllerSpec `json:"modelServiceController,omitempty"`

	// Redis configures the Redis deployment for KV cache and smart routing
	// +optional
	Redis RedisSpec `json:"redis,omitempty"`

	// Gateway configures the Gateway API resources for external access
	// +optional
	Gateway GatewaySpec `json:"gateway,omitempty"`

	// ConfigMapPresets defines which configuration presets to create
	// +optional
	ConfigMapPresets []string `json:"configMapPresets,omitempty"`

	// Monitoring configures metrics collection and monitoring
	// +optional
	Monitoring MonitoringSpec `json:"monitoring,omitempty"`

	// Namespace specifies the target namespace for llm-d components
	// If not specified, uses the same namespace as the LLMD resource
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// ModelServiceControllerSpec defines the ModelService controller configuration
type ModelServiceControllerSpec struct {
	// Enabled determines whether to deploy the ModelService controller
	// +kubebuilder:default=true
	Enabled bool `json:"enabled"`

	// Image specifies the ModelService controller image
	// +optional
	Image string `json:"image,omitempty"`

	// Replicas specifies the number of controller replicas
	// +kubebuilder:default=1
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Resources specifies resource requirements for the controller
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// NodeSelector specifies node selection constraints
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Tolerations specifies pod tolerations
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
}

// RedisSpec defines the Redis configuration
type RedisSpec struct {
	// Enabled determines whether to deploy Redis
	// +kubebuilder:default=true
	Enabled bool `json:"enabled"`

	// External configures external Redis connection
	// +optional
	External ExternalRedisSpec `json:"external,omitempty"`

	// Internal configures internal Redis deployment
	// +optional
	Internal InternalRedisSpec `json:"internal,omitempty"`
}

// ExternalRedisSpec defines external Redis configuration
type ExternalRedisSpec struct {
	// Host is the Redis server host
	Host string `json:"host,omitempty"`

	// Port is the Redis server port
	// +kubebuilder:default=6379
	Port int32 `json:"port,omitempty"`

	// Password is the Redis password
	// +optional
	Password string `json:"password,omitempty"`

	// PasswordSecret references a secret containing the Redis password
	// +optional
	PasswordSecret *corev1.SecretKeySelector `json:"passwordSecret,omitempty"`
}

// InternalRedisSpec defines internal Redis deployment configuration
type InternalRedisSpec struct {
	// Image specifies the Redis image
	// +optional
	Image string `json:"image,omitempty"`

	// Resources specifies resource requirements for Redis
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Persistence configures Redis persistence
	// +optional
	Persistence RedisPersistenceSpec `json:"persistence,omitempty"`
}

// RedisPersistenceSpec defines Redis persistence configuration
type RedisPersistenceSpec struct {
	// Enabled determines whether to enable Redis persistence
	// +kubebuilder:default=true
	Enabled bool `json:"enabled"`

	// Size specifies the persistent volume size
	// +kubebuilder:default="8Gi"
	Size string `json:"size,omitempty"`

	// StorageClass specifies the storage class
	// +optional
	StorageClass string `json:"storageClass,omitempty"`
}

// GatewaySpec defines the Gateway configuration
type GatewaySpec struct {
	// Enabled determines whether to create Gateway resources
	// +kubebuilder:default=true
	Enabled bool `json:"enabled"`

	// ClassName specifies the Gateway class name
	// +kubebuilder:default="kgateway"
	ClassName string `json:"className,omitempty"`

	// Host specifies the hostname for external access
	// +optional
	Host string `json:"host,omitempty"`

	// ServiceType specifies the Gateway service type
	// +kubebuilder:default="NodePort"
	// +kubebuilder:validation:Enum=LoadBalancer;NodePort;ClusterIP
	ServiceType string `json:"serviceType,omitempty"`
}

// MonitoringSpec defines the monitoring configuration
type MonitoringSpec struct {
	// Enabled determines whether to enable monitoring
	// +kubebuilder:default=true
	Enabled bool `json:"enabled"`

	// ServiceMonitor configures ServiceMonitor resources
	// +optional
	ServiceMonitor ServiceMonitorSpec `json:"serviceMonitor,omitempty"`
}

// ServiceMonitorSpec defines ServiceMonitor configuration
type ServiceMonitorSpec struct {
	// Enabled determines whether to create ServiceMonitor resources
	// +kubebuilder:default=true
	Enabled bool `json:"enabled"`

	// Labels to add to ServiceMonitor resources
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Interval specifies the scrape interval
	// +kubebuilder:default="15s"
	Interval string `json:"interval,omitempty"`
}

// LLMDStatus defines the observed state of LLMD
type LLMDStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Conditions represent the latest available observations of the LLMD state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Phase represents the current phase of the LLMD deployment
	// +optional
	Phase string `json:"phase,omitempty"`

	// ModelServiceController shows the status of the ModelService controller
	// +optional
	ModelServiceController ComponentStatus `json:"modelServiceController,omitempty"`

	// Redis shows the status of the Redis deployment
	// +optional
	Redis ComponentStatus `json:"redis,omitempty"`

	// Gateway shows the status of the Gateway resources
	// +optional
	Gateway ComponentStatus `json:"gateway,omitempty"`

	// ConfigMapPresets shows the status of ConfigMap presets
	// +optional
	ConfigMapPresets []ConfigMapPresetStatus `json:"configMapPresets,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// ComponentStatus represents the status of a component
type ComponentStatus struct {
	// Ready indicates whether the component is ready
	Ready bool `json:"ready"`

	// Message provides additional information about the component status
	// +optional
	Message string `json:"message,omitempty"`

	// LastUpdated shows when the status was last updated
	// +optional
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`
}

// ConfigMapPresetStatus represents the status of a ConfigMap preset
type ConfigMapPresetStatus struct {
	// Name is the name of the preset
	Name string `json:"name"`

	// Ready indicates whether the preset is ready
	Ready bool `json:"ready"`

	// Message provides additional information
	// +optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Controller Ready",type=boolean,JSONPath=`.status.modelServiceController.ready`
// +kubebuilder:printcolumn:name="Redis Ready",type=boolean,JSONPath=`.status.redis.ready`
// +kubebuilder:printcolumn:name="Gateway Ready",type=boolean,JSONPath=`.status.gateway.ready`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// LLMD is the Schema for the llmds API
// LLMD is a meta-operator that deploys and manages llm-d infrastructure components
type LLMD struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LLMDSpec   `json:"spec,omitempty"`
	Status LLMDStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// LLMDList contains a list of LLMD
type LLMDList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LLMD `json:"items"`
}
