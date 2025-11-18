package status

import (
	"context"

	"code.uber.internal/apis/evictionrequest/v1alpha1"
	"code.uber.internal/pkg/constants"
	"code.uber.internal/pkg/generated/clientset/versioned"
	"go.uber.org/fx"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Interface interface {
	UpsertCondition(ctx context.Context, evictionRequest *v1alpha1.EvictionRequest, conditionType string, status metav1.ConditionStatus, reason, message string) error
	IncrementFailedEvictionCounter(ctx context.Context, evictionRequest *v1alpha1.EvictionRequest) error
}

type statusHandler struct {
	EvictionRequestClient versioned.Interface
	Logger                *zap.Logger
}

func New(params Params) Interface {
	return &statusHandler{
		EvictionRequestClient: params.EvictionRequestClient,
		Logger:                params.Logger,
	}
}

// Params holds parameters for status operations
type Params struct {
	fx.In

	EvictionRequestClient versioned.Interface
	Logger                *zap.Logger
}

// UpsertCondition adds or updates a condition in the eviction request status
func (s *statusHandler) UpsertCondition(ctx context.Context, evictionRequest *v1alpha1.EvictionRequest, conditionType string, status metav1.ConditionStatus, reason, message string) error {
	now := metav1.Now()
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
	}

	found := false
	for i, existing := range evictionRequest.Status.Conditions {
		if existing.Type == conditionType {
			if existing.Status != status {
				condition.LastTransitionTime = now
			} else {
				condition.LastTransitionTime = existing.LastTransitionTime
			}
			evictionRequest.Status.Conditions[i] = condition
			found = true
			break
		}
	}

	if !found {
		evictionRequest.Status.Conditions = append(evictionRequest.Status.Conditions, condition)
	}

	if evictionRequest.Status.EvictionRequestCancellationPolicy == "" {
		evictionRequest.Status.EvictionRequestCancellationPolicy = v1alpha1.Allow
	}

	if _, err := s.EvictionRequestClient.EvictionrequestV1alpha1().EvictionRequests(evictionRequest.Namespace).UpdateStatus(ctx, evictionRequest, metav1.UpdateOptions{}); err != nil {
		s.Logger.Error("Failed to update eviction request status", zap.Error(err))
		return err
	}

	return nil
}

// IncrementFailedEvictionCounter increments the failed eviction counter and persists it to the API server
func (s *statusHandler) IncrementFailedEvictionCounter(ctx context.Context, evictionRequest *v1alpha1.EvictionRequest) error {
	if evictionRequest.Status.PodEvictionStatus == nil {
		evictionRequest.Status.PodEvictionStatus = &v1alpha1.PodEvictionStatus{}
	}
	evictionRequest.Status.PodEvictionStatus.FailedAPIEvictionCounter++

	if evictionRequest.Status.EvictionRequestCancellationPolicy == "" {
		evictionRequest.Status.EvictionRequestCancellationPolicy = v1alpha1.Allow
	}

	if err := s.UpsertCondition(ctx, evictionRequest, constants.ConditionTypeEvicted, metav1.ConditionFalse, constants.ReasonEvictionFailed, "Failed to evict pod"); err != nil {
		s.Logger.Error("Failed to increment failed eviction counter", zap.Error(err))
		return err
	}

	return nil
}
