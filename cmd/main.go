package main

import (
	"code.uber.internal/pkg/config"
	"code.uber.internal/pkg/constants"
	"code.uber.internal/pkg/controller"
	"code.uber.internal/pkg/generated/clientset/versioned"
	evireqinformers "code.uber.internal/pkg/generated/informers/externalversions"
	evreqlisters "code.uber.internal/pkg/generated/listers/evictionrequest/v1alpha1"
	"code.uber.internal/pkg/reconciler"
	"code.uber.internal/pkg/worker"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corev1listers "k8s.io/client-go/listers/core/v1"
)

func main() {
	fx.New(
		reconciler.Module,
		fx.Provide(
			config.NewClients,
			// Kubernetes informer factory and listers.
			newKubeInformerFactory,
			newPodLister,
			// EvictionRequest informer factory and listers.
			newEvictionRequestInformerFactory,
			newEvictionRequestLister,

			controller.New,
			worker.New,
			zap.NewDevelopment,
		),
		fx.Invoke(run),
	).Run()
}

func run(controller controller.Interface) {
	controller.Start()
}

func newEvictionRequestInformerFactory(evictionRequestClient versioned.Interface) evireqinformers.SharedInformerFactory {
	return evireqinformers.NewSharedInformerFactoryWithOptions(evictionRequestClient, constants.DefaultResyncInterval)
}

func newEvictionRequestLister(evictionRequestInformerFactory evireqinformers.SharedInformerFactory) evreqlisters.EvictionRequestLister {
	return evictionRequestInformerFactory.Evictionrequest().V1alpha1().EvictionRequests().Lister()
}

func newKubeInformerFactory(kubeClient kubernetes.Interface) informers.SharedInformerFactory {
	return informers.NewSharedInformerFactory(kubeClient, constants.DefaultResyncInterval)
}

func newPodLister(kubeInformerFactory informers.SharedInformerFactory) corev1listers.PodLister {
	return kubeInformerFactory.Core().V1().Pods().Lister()
}
