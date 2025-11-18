package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EvictionRequestType is the type of eviction request.
// +enum
type EvictionRequestType string

const (
	// Soft type attempts to evict the target gracefully.
	// Each active interceptor is given unlimited time to resolve the eviction request, provided
	// that it responds periodically (see .spec.heartbeatDeadlineSeconds). This means there is no
	// deadline for a single interceptor, or for the eviction request as a whole.
	//
	// For pod targets, the eviction request controller will call the eviction API endpoint when
	// there are no more interceptors. This call may not succeed due to PodDisruptionBudgets, which
	// may block the pod termination (see .status.podEvictionStatus.failedAPIEvictionCounter). A
	// successful soft eviction request should ideally result in the pod being terminated gracefully.
	Soft EvictionRequestType = "Soft"
)

// EvictionTarget contains a reference to an object that should be evicted.
// Only one target (PodRef, *TBD*) is required.
// +k8s:deepcopy-gen=true
type EvictionTarget struct {
	// PodRef references a pod that is subject to eviction/termination.
	// Only one target (PodRef, *TBD*) is required.
	// This field is immutable.
	// +kubebuilder:validation:Optional
	PodRef *LocalPodReference `json:"podRef,omitempty"`
}

// Requester identifies the entity that is requesting the eviction.
// +k8s:deepcopy-gen=true
type Requester struct {
	// Name must be RFC-1123 DNS subdomain identifying the requester (e.g.
	// foo.example.com).
	// Name of the requester.
	// This field is required.
	// +kubebuilder:validation:Required
	Name string `json:"name"`
}

// EvictionRequestSpec defines the desired state of EvictionRequest
// +k8s:deepcopy-gen=true
type EvictionRequestSpec struct {
	// Valid types are Soft.
	// The default value is Soft.
	//
	// Soft type attempts to evict the target gracefully.
	// Each active interceptor is given unlimited time to resolve the eviction request, provided
	// that it responds periodically (see .spec.heartbeatDeadlineSeconds). This means there is no
	// deadline for a single interceptor, or for the eviction request as a whole.
	//
	// For pod targets, the eviction request controller will call the /evict API endpoint when
	// there are no more interceptors. This call may not succeed due to PodDisruptionBudgets, which
	// may block the pod termination (see .status.podEvictionStatus.failedAPIEvictionCounter). A
	// successful soft eviction request should ideally result in the pod being terminated gracefully.
	//
	// This field is immutable.
	// +kubebuilder:validation:Required
	// +kubebuilder:default=Soft
	Type EvictionRequestType `json:"type"`

	// Target contains a reference to an object (e.g. a pod) that should be evicted.
	// This field is immutable.
	// +kubebuilder:validation:Required
	Target EvictionTarget `json:"target"`

	// At least one requester is required when creating an eviction request.
	// A requester is also required for the eviction request to be processed.
	// Empty list indicates that the eviction request should be canceled.
	//
	// This field cannot be modified if the .status.evictionRequestCancellationPolicy field is
	// set to `Forbid`.
	// It also cannot be modified once the eviction request has been completed (Complete condition is
	// True).
	// +kubebuilder:validation:Optional
	// +patchMergeKey=name
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=name
	Requesters []Requester `json:"requesters,omitempty" patchStrategy:"merge" patchMergeKey:"name"`

	// Interceptors reference interceptors that respond to this eviction request.
	// Interceptors should observe and communicate through the EvictionRequest API to help with
	// the graceful eviction of a target (e.g. termination of a pod).
	//
	// This field does not need to be set and is resolved when the EvictionRequest object is created
	// on admission. It can be populated from multiple sources:
	// - Pod's .spec.evictionInterceptors
	//
	// The maximum length of the interceptors list is 300. The number of interceptors is limited to
	// 50 in the 9900-10099 interval and to 250 outside of this interval.
	// This field is immutable.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxItems=300
	// +patchMergeKey=interceptorClass
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=interceptorClass
	Interceptors []Interceptor `json:"interceptors,omitempty" patchStrategy:"merge" patchMergeKey:"interceptorClass"`

	// HeartbeatDeadlineSeconds is a maximum amount of time an interceptor should take to report on
	// an eviction progress by updating the .status.heartbeatTime.
	// If the .status.heartbeatTime is not updated within the duration of
	// HeartbeatDeadlineSeconds, the eviction request is passed over to the next interceptor with the
	// highest priority. If there is none, the pod is evicted using the Eviction API.
	//
	// The minimum value is 600 (10m) and the maximum value is 86400 (24h).
	// The default value is 1800 (30m).
	// This field is required and immutable.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=600
	// +kubebuilder:validation:Maximum=86400
	// +kubebuilder:default=1800
	HeartbeatDeadlineSeconds *int32 `json:"heartbeatDeadlineSeconds"`
}

// LocalPodReference contains enough information to locate the referenced pod inside the same namespace.
// +k8s:deepcopy-gen=true
type LocalPodReference struct {
	// Name of the pod.
	// This field is required.
	// +kubebuilder:validation:Required
	Name string `json:"name"`
	// UID of the pod.
	// This field is required.
	// +kubebuilder:validation:Required
	UID string `json:"uid"`
}

// Interceptor allows you to identify the interceptor responding to the EvictionRequest.
// Interceptors should observe and communicate through the EvictionRequest API to help with
// the graceful eviction of a target (e.g. termination of a pod).
// +k8s:deepcopy-gen=true
type Interceptor struct {
	// InterceptorClass must be RFC-1123 DNS subdomain identifying the interceptor (e.g.
	// bar.example.com).
	// This field must be unique for each interceptor.
	// This field is required.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Format=hostname
	InterceptorClass string `json:"interceptorClass"`

	// Priority for this InterceptorClass. Higher priorities are selected first by the eviction
	// request controller. The interceptor that is the managing controller should set the value of
	// this field to 10000 to allow both for preemption or fallback registration by other
	// interceptors.
	//
	// Priorities 9900-10099 are reserved for interceptors with a class that has the same parent
	// domain as the controller interceptor. Duplicate priorities are not allowed in this interval.
	//
	// The number of interceptors is limited to 50 in the 9900-10099 interval and to 250
	// outside of this interval.
	// The minimum value is 0 and the maximum value is 100000.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100000
	Priority int32 `json:"priority"`

	// Role of the interceptor. The "controller" value is reserved for the managing controller of
	// the pod. The role can send additional signal to other interceptors if they should preempt
	// this interceptor or not.
	// +kubebuilder:validation:Optional
	Role *string `json:"role,omitempty"`
}

// EvictionRequestStatus represents the most recently observed status of the eviction request.
// Populated by the current interceptor and eviction request controller.
// +k8s:deepcopy-gen=true
type EvictionRequestStatus struct {
	// Conditions can be used by interceptors to share additional information about the eviction
	// request.
	// See EvictionRequestConditionType for eviction request specific conditions.
	// +kubebuilder:validation:Optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// Message is a human readable message indicating details about the eviction request.
	// This may be an empty string.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=32768
	Message string `json:"message"`

	// Interceptors of the ActiveInterceptorClass can adopt this eviction request by updating the
	// HeartbeatTime or orphan/complete it by setting ActiveInterceptorCompleted to true.
	// This field is managed by Kubernetes. It is cleared once the eviction request has completed.
	// +kubebuilder:validation:Optional
	ActiveInterceptorClass *string `json:"activeInterceptorClass,omitempty"`

	// ActiveInterceptorCompleted should be set to true when the interceptor of the
	// ActiveInterceptorClass has fully or partially completed (may result in pod termination).
	// This field can also be set to true if no interceptor is available.
	// If this field is true, there is no additional interceptor available, and the evicted pod is
	// still running, it will be evicted using the Eviction API.
	// +kubebuilder:validation:Optional
	ActiveInterceptorCompleted bool `json:"activeInterceptorCompleted,omitempty"`

	// HeartbeatTime is the time at which the eviction process was reported to be in progress by
	// the interceptor.
	// Cannot be set to the future time (after taking time skew into account).
	// +kubebuilder:validation:Optional
	HeartbeatTime *metav1.Time `json:"heartbeatTime,omitempty"`

	// ExpectedInterceptorFinishTime is the time at which the eviction process step is expected to
	// end for the current interceptor and its class.
	// May be empty if no estimate can be made.
	// +kubebuilder:validation:Optional
	ExpectedInterceptorFinishTime *metav1.Time `json:"expectedInterceptorFinishTime,omitempty"`

	// EvictionRequestCancellationPolicy should be set to Forbid by the interceptor if it is not possible
	// to cancel (delete) the eviction request.
	// When this value is Forbid, DELETE requests of this EvictionRequest object will not be accepted
	// while the pod exists.
	// This field is not reset by the eviction request controller when selecting an interceptor.
	// Changes to this field should always be reconciled by the active interceptor.
	//
	// Valid policies are Allow and Forbid.
	// The default value is Allow.
	//
	// Allow policy allows cancellation of this eviction request.
	// The EvictionRequest can be deleted before the Pod is fully terminated.
	//
	// Forbid policy forbids cancellation of this eviction request.
	// The EvictionRequest can't be deleted until the Pod is fully terminated.
	//
	// This field is required.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=Allow;Forbid
	// +kubebuilder:default=Allow
	EvictionRequestCancellationPolicy EvictionRequestCancellationPolicy `json:"evictionRequestCancellationPolicy"`

	// Pod-specific status that is populated during pod eviction.
	// +kubebuilder:validation:Optional
	PodEvictionStatus *PodEvictionStatus `json:"podEvictionStatus,omitempty"`
}

// EvictionRequestConditionType is a valid value for EvictionRequestCondition.Type
type EvictionRequestConditionType string

// These are built-in conditions of an eviction request.
const (
	// EvictionRequestComplete means that the eviction request is no longer being processed by any
	// eviction interceptor. This may be either because the pod has been terminated or deleted, or
	// because the eviction request has been canceled.
	EvictionRequestComplete EvictionRequestConditionType = "Complete"
)

// EvictionRequestCancellationPolicy defines the cancellation policy for eviction requests.
// +enum
type EvictionRequestCancellationPolicy string

const (
	// Allow policy allows cancellation of this eviction request.
	// The EvictionRequest can be deleted before the target is fully evicted (e.g. before the pod is
	// fully terminated).
	Allow EvictionRequestCancellationPolicy = "Allow"
	// Forbid policy forbids cancellation of this eviction request.
	// The EvictionRequest can't be deleted until the target is fully evicted (e.g. until the pod is
	// fully terminated).
	Forbid EvictionRequestCancellationPolicy = "Forbid"
)

// PodEvictionStatus is the status of the pod eviction.
// +k8s:deepcopy-gen=true
type PodEvictionStatus struct {
	// The number of unsuccessful attempts to evict the referenced pod via the API-initiated eviction,
	// e.g. due to a PodDisruptionBudget.
	// This is set by the eviction controller after all the interceptors have completed.
	// The minimum value is 0, and subsequent updates can only increase it.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=0
	FailedAPIEvictionCounter int32 `json:"failedAPIEvictionCounter"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=evreq
// +kubebuilder:printcolumn:name="Pod",type="string",JSONPath=".spec.target.podRef.name",description="Target pod for eviction"
// +kubebuilder:printcolumn:name="ActiveInterceptor",type="string",JSONPath=".status.activeInterceptorClass",description="Current active interceptor"
// +kubebuilder:printcolumn:name="Heartbeat",type="date",JSONPath=".status.heartbeatTime",description="Last heartbeat"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:rbac:groups=evictionrequest.o2.uberinternal.com,resources=evictionrequests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=evictionrequest.o2.uberinternal.com,resources=evictionrequests/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=evictionrequest.o2.uberinternal.com,resources=evictionrequests/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups="",resources=pods/eviction,verbs=create
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +genclient
// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// EvictionRequest is the Schema for the evictionrequests API
type EvictionRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the eviction request specification.
	// https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// This field is required.
	// +required
	Spec EvictionRequestSpec `json:"spec"`
	// Status represents the most recently observed status of the eviction request.
	// Populated by the current interceptor and eviction request controller.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// +optional
	Status EvictionRequestStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// EvictionRequestList contains a list of EvictionRequest
type EvictionRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EvictionRequest `json:"items"`
}
