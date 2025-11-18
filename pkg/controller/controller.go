package controller

import (
	"context"
	"reflect"
	"time"

	"code.uber.internal/apis/evictionrequest/v1alpha1"
	"code.uber.internal/pkg/generated/clientset/versioned"
	evreqinformer "code.uber.internal/pkg/generated/informers/externalversions"
	"code.uber.internal/pkg/reconciler"
	"code.uber.internal/pkg/worker"
	"github.com/google/uuid"
	"go.uber.org/fx"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

const (
	_leaseName          = "eviction-request-controller"
	_leaseNamespace     = "default"
	_leaseDuration      = 15 * time.Second
	_leaseRenewDeadline = 10 * time.Second
	_leaseRetryPeriod   = 2 * time.Second
)

type Interface interface {
	Start()
}

type controller struct {
	lc fx.Lifecycle

	kubeClient            kubernetes.Interface
	evictionRequestClient versioned.Interface

	logger *zap.Logger

	reconciler reconciler.Interface
	worker     worker.Interface

	evictionRequestInformerFactory evreqinformer.SharedInformerFactory
	kubeInformerFactory            informers.SharedInformerFactory

	stopCh chan struct{}
}

type params struct {
	fx.In

	Lifecycle fx.Lifecycle

	Reconciler reconciler.Interface
	Worker     worker.Interface

	KubeClient            kubernetes.Interface
	EvictionRequestClient versioned.Interface

	Logger *zap.Logger

	EvictionRequestInformerFactory evreqinformer.SharedInformerFactory
	KubeInformerFactory            informers.SharedInformerFactory
}

// New creates a new Controller
func New(params params) Interface {
	return &controller{
		lc:                             params.Lifecycle,
		kubeClient:                     params.KubeClient,
		evictionRequestClient:          params.EvictionRequestClient,
		reconciler:                     params.Reconciler,
		logger:                         params.Logger,
		worker:                         params.Worker,
		evictionRequestInformerFactory: params.EvictionRequestInformerFactory,
		kubeInformerFactory:            params.KubeInformerFactory,
	}
}

// Start begins the controller with leader election and fx lifecycle management
func (c *controller) Start() {
	leaderConfig := c.createLeaderElectionConfig()
	c.registerLifecycleHooks(leaderConfig)
}

// createLeaderElectionConfig creates the leader election configuration
func (c *controller) createLeaderElectionConfig() leaderelection.LeaderElectionConfig {
	id := uuid.New().String()
	c.logger.Info("Leader election id", zap.String("id", id))

	lock := c.createResourceLock(id)
	callbacks := c.createLeaderCallbacks()

	return leaderelection.LeaderElectionConfig{
		Lock:          lock,
		LeaseDuration: _leaseDuration,
		RenewDeadline: _leaseRenewDeadline,
		RetryPeriod:   _leaseRetryPeriod,
		Callbacks:     callbacks,
	}
}

// createResourceLock creates the resource lock for leader election
func (c *controller) createResourceLock(id string) resourcelock.Interface {
	return &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      _leaseName,
			Namespace: _leaseNamespace,
		},
		Client: c.kubeClient.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: id,
		},
	}
}

// createLeaderCallbacks creates the leader election callbacks
func (c *controller) createLeaderCallbacks() leaderelection.LeaderCallbacks {
	return leaderelection.LeaderCallbacks{
		OnStartedLeading: func(ctx context.Context) {
			c.onStartedLeading(ctx)
		},
		OnStoppedLeading: func() {
			c.onStoppedLeading()
		},
	}
}

// onStartedLeading handles the logic when the controller becomes the leader
func (c *controller) onStartedLeading(ctx context.Context) {
	c.logger.Info("Started leading, setting up informers and workers")

	// Setup event handlers
	c.setupEventHandlers()

	// Start informers
	allSynced := c.startInformers()
	if !allSynced {
		c.logger.Error("Some informers failed to sync - controller may operate with incomplete or stale data")
	}

	// Start worker
	go c.worker.Start(ctx)
}

// onStoppedLeading handles the logic when the controller stops being the leader
func (c *controller) onStoppedLeading() {
	c.logger.Info("Stopped leading, shutting down informers")

	// Safely close the stop channel if it exists and hasn't been closed
	if c.stopCh != nil {
		select {
		case <-c.stopCh:
			// Channel is already closed, do nothing
			c.logger.Debug("Stop channel already closed")
		default:
			// Channel is open, close it
			close(c.stopCh)
			c.logger.Debug("Stop channel closed")
		}
	}
}

// setupEventHandlers sets up the event handlers for EvictionRequest informer
func (c *controller) setupEventHandlers() {
	evictionRequestInformer := c.evictionRequestInformerFactory.Evictionrequest().V1alpha1().EvictionRequests().Informer()

	_, _ = evictionRequestInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.handleEvictionRequestAdd,
		UpdateFunc: c.handleEvictionRequestUpdate,
	})
}

// handleEvictionRequestAdd handles EvictionRequest add events
func (c *controller) handleEvictionRequestAdd(obj interface{}) {
	evictionRequest, ok := obj.(*v1alpha1.EvictionRequest)
	if !ok {
		c.logger.Warn("Received non-EvictionRequest object in add handler", zap.Any("obj", obj))
		return
	}

	c.logger.Info("EvictionRequest added",
		zap.Any("eviction_request_spec", evictionRequest.Spec),
		zap.Any("eviction_request_status", evictionRequest.Status))
	c.worker.Enqueue(evictionRequest)
}

// handleEvictionRequestUpdate handles EvictionRequest update events
func (c *controller) handleEvictionRequestUpdate(oldObj, newObj interface{}) {
	oldEvictionRequest, ok := oldObj.(*v1alpha1.EvictionRequest)
	if !ok {
		c.logger.Warn("Received non-EvictionRequest object in update handler (old)", zap.Any("obj", oldObj))
		return
	}

	newEvictionRequest, ok := newObj.(*v1alpha1.EvictionRequest)
	if !ok {
		c.logger.Warn("Received non-EvictionRequest object in update handler (new)", zap.Any("obj", newObj))
		return
	}

	c.logger.Info("EvictionRequest updated",
		zap.Any("old_eviction_request_spec", oldEvictionRequest.Spec),
		zap.Any("old_eviction_request_status", oldEvictionRequest.Status),
		zap.Any("new_eviction_request_spec", newEvictionRequest.Spec),
		zap.Any("new_eviction_request_status", newEvictionRequest.Status))
	c.worker.Enqueue(newEvictionRequest)
}

// startInformers starts all informers and waits for cache sync
func (c *controller) startInformers() bool {
	c.stopCh = make(chan struct{})

	allSynced := true

	// Start kube informers
	c.kubeInformerFactory.Start(c.stopCh)
	kubeSyncMap := c.kubeInformerFactory.WaitForCacheSync(c.stopCh)
	if !c.verifyCacheSync("kube", kubeSyncMap) {
		c.logger.Error("Some kube informers failed to sync cache - controller may operate with stale data")
		allSynced = false
	}

	// Start eviction request informers
	c.evictionRequestInformerFactory.Start(c.stopCh)
	evictionRequestSyncMap := c.evictionRequestInformerFactory.WaitForCacheSync(c.stopCh)
	if !c.verifyCacheSync("eviction-request", evictionRequestSyncMap) {
		c.logger.Error("Some eviction-request informers failed to sync cache - controller may operate with stale data")
		allSynced = false
	}

	if allSynced {
		c.logger.Info("All informers started and synced")
		return true
	}
	return false
}

// verifyCacheSync checks if all informers in the sync map successfully synced
// Returns true if all informers synced, false otherwise
func (c *controller) verifyCacheSync(factoryName string, syncMap map[reflect.Type]bool) bool {
	allSynced := true
	for informerType, synced := range syncMap {
		if !synced {
			allSynced = false
			c.logger.Warn("Failed to sync informer cache",
				zap.String("factory", factoryName),
				zap.String("informer_type", informerType.Name()),
			)
		}
	}
	return allSynced
}

// registerLifecycleHooks registers the fx lifecycle hooks for the controller
func (c *controller) registerLifecycleHooks(leaderConfig leaderelection.LeaderElectionConfig) {
	leaderCtx, leaderCancel := context.WithCancel(context.Background())

	c.lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				c.logger.Info("Starting leader election loop")
				leaderelection.RunOrDie(leaderCtx, leaderConfig)
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			c.logger.Info("Stopping leader election loop")
			leaderCancel()
			return nil
		},
	})
}
