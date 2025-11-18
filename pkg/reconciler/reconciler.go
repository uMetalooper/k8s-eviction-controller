package reconciler

import (
	"context"

	"code.uber.internal/apis/evictionrequest/v1alpha1"
	"code.uber.internal/pkg/generated/clientset/versioned"
	"code.uber.internal/pkg/reconciler/eviction"
	"code.uber.internal/pkg/reconciler/interceptor"
	"go.uber.org/fx"
	"go.uber.org/zap"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/listers/core/v1"
)

type Interface interface {
	ReconcileEvictionRequest(ctx context.Context, evictionRequest *v1alpha1.EvictionRequest) error
}
type params struct {
	fx.In

	PodLister             v1.PodLister
	EvictionRequestClient versioned.Interface
	KubeClient            kubernetes.Interface
	Logger                *zap.Logger
	InterceptorHandler    interceptor.Interface
	EvictionPerformer     eviction.Interface
}

// Reconciler reconciles EvictionRequest resources
type reconciler struct {
	// pod lister
	podLister v1.PodLister

	// eviction request client
	evictionRequestClient versioned.Interface
	kubeClient            kubernetes.Interface

	// misc
	logger *zap.Logger

	interceptorHandler interceptor.Interface
	evictionPerformer  eviction.Interface
}

// New creates a new Reconciler
func New(params params) Interface {
	return &reconciler{
		podLister:             params.PodLister,
		evictionRequestClient: params.EvictionRequestClient,
		kubeClient:            params.KubeClient,
		logger:                params.Logger,
		interceptorHandler:    params.InterceptorHandler,
		evictionPerformer:     params.EvictionPerformer,
	}
}

// ReconcileEvictionRequest is the main reconciliation loop for EvictionRequest resources
func (r *reconciler) ReconcileEvictionRequest(ctx context.Context, evictionRequest *v1alpha1.EvictionRequest) error {
	pod, err := r.podLister.Pods(evictionRequest.Namespace).Get(evictionRequest.Spec.Target.PodRef.Name)
	if apierrors.IsNotFound(err) {
		r.logger.Info("Pod in pod reference not found, skipping...")
		return nil
	}
	if err != nil {
		r.logger.Error("Failed to get pod", zap.Error(err))
		return err
	}

	// Verify pod UID matches
	if string(pod.UID) != evictionRequest.Spec.Target.PodRef.UID {
		r.logger.Warn("Pod UID mismatch", zap.String("expected", evictionRequest.Spec.Target.PodRef.UID), zap.String("actual", string(pod.UID)))
		return nil
	}

	if evictionRequest.Status.EvictionRequestCancellationPolicy == "" {
		evictionRequest.Status.EvictionRequestCancellationPolicy = v1alpha1.Allow
		_, err := r.evictionRequestClient.EvictionrequestV1alpha1().EvictionRequests(evictionRequest.Namespace).UpdateStatus(ctx, evictionRequest, metav1.UpdateOptions{})
		if err != nil {
			r.logger.Error("Failed to update eviction request status", zap.Error(err))
			return err
		}
	}

	// Handle interceptors
	if len(evictionRequest.Spec.Interceptors) > 0 {
		return r.interceptorHandler.Handle(ctx, evictionRequest)
	}

	// No interceptors, proceed with eviction
	return r.evictionPerformer.Perform(ctx, evictionRequest)
}
