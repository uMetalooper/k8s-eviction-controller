package worker

import (
	"context"
	"fmt"

	"code.uber.internal/apis/evictionrequest/v1alpha1"
	"code.uber.internal/pkg/reconciler"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const (
	_workerCount = 10
)

type Interface interface {
	Enqueue(obj interface{})
	Start(ctx context.Context)
	GetWorkqueue() workqueue.RateLimitingInterface
}

type pool struct {
	workqueue  workqueue.RateLimitingInterface
	reconciler reconciler.Interface
	logger     *zap.Logger
}

type params struct {
	fx.In

	Reconciler reconciler.Interface
	Logger     *zap.Logger
}

// New creates a new worker pool
func New(params params) Interface {

	return &pool{
		workqueue: workqueue.NewNamedRateLimitingQueue(
			workqueue.DefaultControllerRateLimiter(),
			"eviction-requests",
		),
		reconciler: params.Reconciler,
		logger:     params.Logger,
	}
}

// Enqueue adds an eviction request to the work queue
func (p *pool) Enqueue(obj interface{}) {
	evictionRequest, ok := obj.(*v1alpha1.EvictionRequest)
	if !ok {
		runtime.HandleError(fmt.Errorf("expected *v1alpha1.EvictionRequest but got %T", obj))
		return
	}

	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		runtime.HandleError(fmt.Errorf("error obtaining key for object %v: %w", obj, err))
		return
	}

	p.logger.Debug("Enqueuing eviction request",
		zap.String("key", key),
		zap.String("namespace", evictionRequest.Namespace),
		zap.String("name", evictionRequest.Name),
	)

	p.workqueue.Add(evictionRequest.DeepCopy())
}

// Start begins the worker pool with the specified number of workers
func (p *pool) Start(ctx context.Context) {
	defer runtime.HandleCrash()
	defer p.workqueue.ShutDown()

	p.logger.Info("Starting worker pool", zap.Int("worker_count", _workerCount))

	for i := 0; i < _workerCount; i++ {
		workerID := i
		go p.runWorker(ctx, workerID)
	}

	<-ctx.Done()
	p.logger.Info("Shutting down worker pool")
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the workqueue.
func (p *pool) runWorker(ctx context.Context, workerID int) {
	for p.processNextWorkItem(ctx, workerID) {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the reconciler.
func (p *pool) processNextWorkItem(ctx context.Context, workerID int) bool {
	obj, shutdown := p.workqueue.Get()
	if shutdown {
		return false
	}

	// We wrap this block in a func so we can defer p.workqueue.Done.
	err := func(obj interface{}) error {
		defer p.workqueue.Done(obj)

		evictionRequest, ok := obj.(*v1alpha1.EvictionRequest)
		if !ok {
			// As the item in the workqueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			p.workqueue.Forget(obj)
			runtime.HandleError(fmt.Errorf("expected *v1alpha1.EvictionRequest in workqueue but got %#v", obj))
			return nil
		}

		key, err := cache.MetaNamespaceKeyFunc(evictionRequest)
		if err != nil {
			p.workqueue.Forget(obj)
			runtime.HandleError(fmt.Errorf("error obtaining key for eviction request: %w", err))
			return nil
		}

		if err := p.syncHandler(ctx, evictionRequest, workerID); err != nil {
			// Put the item back on the workqueue to handle any transient errors.
			p.workqueue.AddRateLimited(obj)
			return fmt.Errorf("error syncing '%s': %w, requeuing", key, err)
		}

		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		p.workqueue.Forget(obj)
		p.logger.Debug("Successfully synced", zap.String("key", key), zap.Int("worker_id", workerID))
		return nil
	}(obj)

	if err != nil {
		runtime.HandleError(err)
		return true
	}

	return true
}

// syncHandler processes a single item from the workqueue
func (p *pool) syncHandler(ctx context.Context, evictionRequest *v1alpha1.EvictionRequest, workerID int) error {
	p.logger.Info("Processing eviction request",
		zap.String("namespace", evictionRequest.Namespace),
		zap.String("name", evictionRequest.Name),
		zap.Int("worker_id", workerID),
		zap.Any("spec", evictionRequest.Spec),
		zap.Any("status", evictionRequest.Status),
	)

	err := p.reconciler.ReconcileEvictionRequest(ctx, evictionRequest)
	if err != nil {
		p.logger.Error("Failed to reconcile eviction request",
			zap.String("namespace", evictionRequest.Namespace),
			zap.String("name", evictionRequest.Name),
			zap.Int("worker_id", workerID),
			zap.Error(err),
		)
		return err
	}
	p.logger.Info("Successfully synced eviction request",
		zap.String("namespace", evictionRequest.Namespace),
		zap.String("name", evictionRequest.Name),
		zap.Int("worker_id", workerID),
		zap.Any("spec", evictionRequest.Spec),
		zap.Any("status", evictionRequest.Status),
	)

	return nil
}

// GetWorkqueue returns the underlying workqueue (useful for testing)
func (p *pool) GetWorkqueue() workqueue.RateLimitingInterface {
	return p.workqueue
}
