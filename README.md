# What is this?
This is one possible implementation of the eviction controller for the EvictionRequest defined in the [KEP-4563](
https://github.com/kubernetes/enhancements/issues/4563).

# Getting started
Start a Kind cluster with:
```bash
kind create cluster
```
Apply the CRD against the kind cluster:
```bash
kubectl apply -f config/crd/bases/evictionrequest.coordination.uber.com_evictionrequests.yaml
```
Start the controller:
```bash
go run cmd/main.go
```
Create a Pod:
```bash
kubectl apply -f examples/pod.yaml
```
Create an EvictionRequest:
```bash
POD_UID=$(kubectl get pod example-pod -o jsonpath="{.metadata.uid}")
cat examples/eviction-request.yaml | sed "s/POD_UID/$POD_UID/g" | kubectl apply -f -
```
# Appendix
## Re-generate CRD manifest file and clientsets/informers
Install `controller-gen` with:
```bash
go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest
```
Generate CRD manifest file:
```bash
controller-gen crd paths="./apis/..." output:crd:artifacts:config=config/crd/bases
```

Generate deepcopy
```bash
go get k8s.io/code-generator
go install k8s.io/code-generator/cmd/deepcopy-gen
deepcopy-gen --output-file zz_generated.deepcopy.go ./apis/evictionrequest/v1alpha1
```

Generate clientset
```bash
go install k8s.io/code-generator/cmd/client-gen
client-gen --output-dir pkg/generated/clientset --output-pkg code.uber.internal/pkg/generated/clientset --clientset-name versioned \
--input-base "$(cd apis && pwd -P)" --input evictionrequest/v1alpha1
```

Generate listers
```bash
go install k8s.io/code-generator/cmd/lister-gen
lister-gen --output-dir pkg/generated/listers --output-pkg code.uber.internal/pkg/generated/listers code.uber.internal/apis/evictionrequest/v1alpha1
```

Generate informers
```bash
go install k8s.io/code-generator/cmd/informer-gen
informer-gen --output-dir pkg/generated/informers --output-pkg code.uber.internal/pkg/generated/informers \                                      
--versioned-clientset-package code.uber.internal/pkg/generated/clientset/versioned --listers-package code.uber.internal/pkg/generated/listers code.uber.internal/apis/evictionrequest/v1alpha1
```