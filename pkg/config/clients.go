package config

import (
	"os"

	"code.uber.internal/pkg/generated/clientset/versioned"
	"go.uber.org/fx"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	BuildConfigFromFlagsFn              = clientcmd.BuildConfigFromFlags
	NewForConfigEvictionRequestClientFn = versioned.NewForConfig
	NewForConfigKubeClientFn            = kubernetes.NewForConfig
)

// Result holds the Kubernetes clients
type Result struct {
	fx.Out

	KubeClient            kubernetes.Interface
	EvictionRequestClient versioned.Interface
	Config                *rest.Config
}

// NewClients creates and initializes Kubernetes clients from KUBECONFIG
func NewClients() (Result, error) {
	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		return Result{}, &MissingKubeconfigError{}
	}

	config, err := BuildConfigFromFlagsFn("", kubeconfigPath)
	if err != nil {
		return Result{}, err
	}

	evictionRequestClient, err := NewForConfigEvictionRequestClientFn(config)
	if err != nil {
		return Result{}, err
	}

	kubeClient, err := NewForConfigKubeClientFn(config)
	if err != nil {
		return Result{}, err
	}

	return Result{
		KubeClient:            kubeClient,
		EvictionRequestClient: evictionRequestClient,
		Config:                config,
	}, nil
}

// MissingKubeconfigError is returned when KUBECONFIG environment variable is not set
type MissingKubeconfigError struct{}

func (e *MissingKubeconfigError) Error() string {
	return "KUBECONFIG environment variable is not set"
}
