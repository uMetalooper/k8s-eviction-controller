package eviction

import (
	"context"
	"errors"
	"fmt"

	"code.uber.internal/apis/evictionrequest/v1alpha1"
	"code.uber.internal/pkg/constants"
	"code.uber.internal/pkg/generated/clientset/versioned"
	"code.uber.internal/pkg/reconciler/status"
	"go.uber.org/fx"
	"go.uber.org/zap"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/listers/core/v1"
)

type Interface interface {
	Perform(ctx context.Context, evictionRequest *v1alpha1.EvictionRequest) error
}

type evictionPerformer struct {
	PodLister             v1.PodLister
	EvictionRequestClient versioned.Interface
	KubeClient            kubernetes.Interface
	StatusHandler         status.Interface
	Logger                *zap.Logger
}

func New(params params) Interface {
	return &evictionPerformer{
		PodLister:             params.PodLister,
		EvictionRequestClient: params.EvictionRequestClient,
		KubeClient:            params.KubeClient,
		StatusHandler:         params.StatusHandler,
		Logger:                params.Logger,
	}
}

type params struct {
	fx.In

	PodLister             v1.PodLister
	EvictionRequestClient versioned.Interface
	KubeClient            kubernetes.Interface
	StatusHandler         status.Interface
	Logger                *zap.Logger
}

// Perform executes the pod eviction logic for an eviction request
func (e *evictionPerformer) Perform(ctx context.Context, evictionRequest *v1alpha1.EvictionRequest) error {
	if evictionRequest.Spec.Target.PodRef == nil {
		e.Logger.Error("FailedPrecondition: EvictionRequest.Spec.Target.PodRef cannot be nil")
		e.StatusHandler.UpsertCondition(ctx, evictionRequest, constants.ConditionTypeReady, metav1.ConditionFalse, constants.ReasonPodNotFound, "EvictionRequest.Spec.Target.PodRef cannot be nil")
		return errors.New("pod reference cannot be nil")
	}

	pod, err := e.PodLister.Pods(evictionRequest.Namespace).Get(evictionRequest.Spec.Target.PodRef.Name)
	if apierrors.IsNotFound(err) {
		e.Logger.Warn("Pod in pod reference not found, skipping...")
		return nil
	}
	if err != nil {
		e.Logger.Error("Failed to get pod", zap.Error(err))
		return fmt.Errorf("failed to get pod: %w", err)
	}

	// Create eviction object
	eviction := &policyv1.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		},
		DeleteOptions: &metav1.DeleteOptions{},
	}

	// Perform eviction using Kubernetes clientset
	if err := e.KubeClient.CoreV1().Pods(pod.Namespace).EvictV1(ctx, eviction); err != nil {
		e.Logger.Error("Failed to evict pod", zap.Error(err))
		e.StatusHandler.IncrementFailedEvictionCounter(ctx, evictionRequest)
		return fmt.Errorf("failed to evict pod: %w", err)
	}

	e.Logger.Info("Pod evicted successfully", zap.String("target_pod_name", pod.Name))
	e.StatusHandler.UpsertCondition(ctx, evictionRequest, constants.ConditionTypeEvicted, metav1.ConditionTrue, constants.ReasonEvictionSucceeded, "Pod evicted successfully")

	return nil
}
