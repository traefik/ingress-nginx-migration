package analyzer

import (
	"context"
	"errors"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/labels"
	kinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	listersnetv1 "k8s.io/client-go/listers/networking/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	resyncPeriod          = 5 * time.Minute
	defaultControllerName = "k8s.io/ingress-nginx"
)

type Analyzer struct {
	k8sClient          *kubernetes.Clientset
	controllerClass    string
	ingressListers     []listersnetv1.IngressLister
	ingressClassLister listersnetv1.IngressClassLister
}

// FIXME not sure if we should start the informers in the new, this made the code untestable.
func New(ctx context.Context, kubeconfig string, namespaces []string, controllerClass string) (*Analyzer, error) {
	// When namespaces list is empty all namespaces are listed.
	if len(namespaces) == 0 {
		namespaces = []string{v1.NamespaceAll}
	}

	// When controller class is empty, we use the default one.
	if controllerClass == "" {
		controllerClass = defaultControllerName
	}

	// Creates the Kubernetes client.
	var (
		err       error
		k8sClient *kubernetes.Clientset
	)
	config, err := rest.InClusterConfig()
	if err != nil && !errors.Is(err, rest.ErrNotInCluster) {
		return nil, fmt.Errorf("creating in cluster config: %w", err)
	}
	if err != nil {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("creating config from flags: %w", err)
		}
	}

	k8sClient, err = kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("creating k8s client from config: %w", err)
	}

	// Initialize IngressClass listers.
	clusterFactory := kinformers.NewSharedInformerFactoryWithOptions(k8sClient, resyncPeriod)
	clusterFactory.Networking().V1().IngressClasses().Lister()

	clusterFactory.Start(ctx.Done())
	for t, ok := range clusterFactory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return nil, fmt.Errorf("timed out waiting for K8s caches to sync %s", t.String())
		}
	}

	// Initialize Ingress listers per namespace.
	var ingressListers []listersnetv1.IngressLister
	for _, namespace := range namespaces {
		namespaceFactory := kinformers.NewSharedInformerFactoryWithOptions(k8sClient, resyncPeriod, kinformers.WithNamespace(namespace))
		namespaceFactory.Networking().V1().Ingresses().Informer()

		ingressListers = append(ingressListers, namespaceFactory.Networking().V1().Ingresses().Lister())

		namespaceFactory.Start(ctx.Done())
		for t, ok := range namespaceFactory.WaitForCacheSync(ctx.Done()) {
			if !ok {
				return nil, fmt.Errorf("timed out waiting for K8s caches to sync %s", t.String())
			}
		}
	}

	return &Analyzer{
		k8sClient:          k8sClient,
		controllerClass:    controllerClass,
		ingressListers:     ingressListers,
		ingressClassLister: clusterFactory.Networking().V1().IngressClasses().Lister(),
	}, nil
}

// Report generates and returns the analysis report.
func (a *Analyzer) Report() (Report, error) {
	ingressClasses, err := a.ingressClassLister.List(labels.Everything())
	if err != nil {
		return Report{}, fmt.Errorf("listing IngressClasses: %w", err)
	}

	var ingresses []*netv1.Ingress
	for _, ingressLister := range a.ingressListers {
		nsIngresses, err := ingressLister.List(labels.Everything())
		if err != nil {
			return Report{}, fmt.Errorf("listing Ingresses: %w", err)
		}

		ingresses = append(ingresses, nsIngresses...)
	}

	return computeReport(ingressClasses, ingresses, a.controllerClass), nil
}
