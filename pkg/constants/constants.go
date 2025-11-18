package constants

import "time"

const (
	// DefaultResyncInterval is the default interval for resync
	DefaultResyncInterval = 1 * time.Hour

	// ConditionTypeReady is the condition type for the EvictionRequest resource
	ConditionTypeReady = "Ready"
	// ConditionTypeIntercepting is the condition type for the EvictionRequest resource
	ConditionTypeIntercepting = "Intercepting"
	// ConditionTypeEvicted is the condition type for the EvictionRequest resource
	ConditionTypeEvicted = "Evicted"

	// ReasonPodNotFound is the reason for the EvictionRequest resource
	ReasonPodNotFound = "PodNotFound"
	// ReasonInterceptorsReady is the reason for the EvictionRequest resource
	ReasonInterceptorsReady = "InterceptorsReady"
	// ReasonInterceptorsNotReady is the reason for the EvictionRequest resource
	ReasonInterceptorsNotReady = "InterceptorsNotReady"
	// ReasonEvictionSucceeded is the reason for the EvictionRequest resource
	ReasonEvictionSucceeded = "EvictionSucceeded"
	// ReasonEvictionFailed is the reason for the EvictionRequest resource
	ReasonEvictionFailed = "EvictionFailed"
)
