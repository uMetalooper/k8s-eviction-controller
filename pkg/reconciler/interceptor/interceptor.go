package interceptor

import (
	"context"
	"sort"
	"time"

	"code.uber.internal/apis/evictionrequest/v1alpha1"
	"code.uber.internal/pkg/generated/clientset/versioned"
	"code.uber.internal/pkg/reconciler/eviction"
	"go.uber.org/fx"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/listers/core/v1"
)

type Interface interface {
	Handle(ctx context.Context, evictionRequest *v1alpha1.EvictionRequest) error
}

type interceptorHandler struct {
	PodLister             v1.PodLister
	EvictionRequestClient versioned.Interface
	KubeClient            kubernetes.Interface
	Logger                *zap.Logger
	EvictionPerformer     eviction.Interface
}

func New(params params) Interface {
	return &interceptorHandler{
		PodLister:             params.PodLister,
		EvictionRequestClient: params.EvictionRequestClient,
		KubeClient:            params.KubeClient,
		Logger:                params.Logger,
		EvictionPerformer:     params.EvictionPerformer,
	}
}

type params struct {
	fx.In

	PodLister             v1.PodLister
	EvictionRequestClient versioned.Interface
	KubeClient            kubernetes.Interface
	Logger                *zap.Logger
	EvictionPerformer     eviction.Interface
}

// Handle processes interceptors for an eviction request
func (i *interceptorHandler) Handle(ctx context.Context, evictionRequest *v1alpha1.EvictionRequest) error {
	interceptors := i.sortInterceptorsByPriority(evictionRequest.Spec.Interceptors)

	// State 1: No active interceptor - select the highest priority
	if evictionRequest.Status.ActiveInterceptorClass == nil || *evictionRequest.Status.ActiveInterceptorClass == "" {
		return i.selectInitialInterceptor(ctx, evictionRequest, interceptors)
	}

	// State 2: Active interceptor exists and completed - select next highest priority
	if evictionRequest.Status.ActiveInterceptorCompleted {
		return i.handleCompletedInterceptor(ctx, evictionRequest, interceptors)
	}

	// State 3: Active interceptor exists and not completed - check for timeout
	return i.checkInterceptorTimeout(ctx, evictionRequest)
}

// sortInterceptorsByPriority sorts interceptors by priority (highest first) for consistent ordering
func (i *interceptorHandler) sortInterceptorsByPriority(interceptors []v1alpha1.Interceptor) []v1alpha1.Interceptor {
	sortedInterceptors := make([]v1alpha1.Interceptor, len(interceptors))
	copy(sortedInterceptors, interceptors)
	sort.Slice(sortedInterceptors, func(i, j int) bool {
		return sortedInterceptors[i].Priority > sortedInterceptors[j].Priority
	})
	return sortedInterceptors
}

// selectInitialInterceptor selects the highest priority interceptor when no active interceptor exists
func (i *interceptorHandler) selectInitialInterceptor(ctx context.Context, evictionRequest *v1alpha1.EvictionRequest, interceptors []v1alpha1.Interceptor) error {
	highestPriorityInterceptor := &interceptors[0]
	evictionRequest.Status.ActiveInterceptorClass = &highestPriorityInterceptor.InterceptorClass
	return i.updateEvictionRequestStatus(ctx, evictionRequest)
}

// handleCompletedInterceptor handles the case when the active interceptor has completed
func (i *interceptorHandler) handleCompletedInterceptor(ctx context.Context, evictionRequest *v1alpha1.EvictionRequest, interceptors []v1alpha1.Interceptor) error {
	currentIndex := i.findInterceptorIndex(interceptors, *evictionRequest.Status.ActiveInterceptorClass)

	// Select next interceptor (next in priority order)
	if currentIndex >= 0 && currentIndex+1 < len(interceptors) {
		return i.selectNextInterceptor(ctx, evictionRequest, interceptors, currentIndex)
	}

	// No more interceptors, proceed with direct eviction
	i.Logger.Info("All interceptors completed, proceeding with direct eviction")
	return i.EvictionPerformer.Perform(ctx, evictionRequest)
}

// findInterceptorIndex finds the index of the interceptor with the given class in the sorted list
func (i *interceptorHandler) findInterceptorIndex(interceptors []v1alpha1.Interceptor, interceptorClass string) int {
	for idx, interceptor := range interceptors {
		if interceptor.InterceptorClass == interceptorClass {
			return idx
		}
	}
	return -1
}

// selectNextInterceptor selects the next interceptor in priority order
func (i *interceptorHandler) selectNextInterceptor(ctx context.Context, evictionRequest *v1alpha1.EvictionRequest, interceptors []v1alpha1.Interceptor, currentIndex int) error {
	nextInterceptor := &interceptors[currentIndex+1]
	evictionRequest.Status.ActiveInterceptorClass = &nextInterceptor.InterceptorClass
	return i.updateEvictionRequestStatus(ctx, evictionRequest)
}

// checkInterceptorTimeout checks if the active interceptor has exceeded its deadline
func (i *interceptorHandler) checkInterceptorTimeout(ctx context.Context, evictionRequest *v1alpha1.EvictionRequest) error {
	if evictionRequest.Spec.HeartbeatDeadlineSeconds != nil && evictionRequest.Status.HeartbeatTime != nil {
		deadline := time.Duration(*evictionRequest.Spec.HeartbeatDeadlineSeconds) * time.Second
		if time.Since(evictionRequest.Status.HeartbeatTime.Time) > deadline {
			return i.markInterceptorAsCompleted(ctx, evictionRequest, deadline)
		}
	}

	// Interceptor is still active and within deadline, wait for progress
	i.Logger.Info("Waiting for interceptor progress", zap.String("interceptor_class", *evictionRequest.Status.ActiveInterceptorClass))
	return nil
}

// markInterceptorAsCompleted marks the active interceptor as completed due to timeout
func (i *interceptorHandler) markInterceptorAsCompleted(ctx context.Context, evictionRequest *v1alpha1.EvictionRequest, deadline time.Duration) error {
	i.Logger.Info("Interceptor deadline exceeded, marking as completed",
		zap.String("interceptor_class", *evictionRequest.Status.ActiveInterceptorClass),
		zap.Duration("deadline", deadline))
	evictionRequest.Status.ActiveInterceptorCompleted = true
	return i.updateEvictionRequestStatus(ctx, evictionRequest)
}

// updateEvictionRequestStatus updates the status of the eviction request
func (i *interceptorHandler) updateEvictionRequestStatus(ctx context.Context, evictionRequest *v1alpha1.EvictionRequest) error {
	_, err := i.EvictionRequestClient.EvictionrequestV1alpha1().EvictionRequests(evictionRequest.Namespace).UpdateStatus(ctx, evictionRequest, metav1.UpdateOptions{})
	if err != nil {
		i.Logger.Error("Failed to update eviction request status", zap.Error(err))
		return err
	}
	return nil
}
