package analyzer

import (
	"context"
	"fmt"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/labels"
	kinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	listersnetv1 "k8s.io/client-go/listers/networking/v1"
)

const (
	resyncPeriod           = 5 * time.Minute
	defaultAnnotationValue = "nginx"
	defaultControllerName  = "k8s.io/ingress-nginx"
)

// Analyzer analyzes IngressClass/Ingress resources and generates a report.
type Analyzer struct {
	ingressClass             string
	controllerClass          string
	watchIngressWithoutClass bool
	ingressClassByName       bool

	clusterFactory kinformers.SharedInformerFactory
	nsFactories    []kinformers.SharedInformerFactory

	ingressListers     []listersnetv1.IngressLister
	ingressClassLister listersnetv1.IngressClassLister

	reportMu sync.RWMutex
	report   Report
}

// New creates a new Analyzer.
func New(k8sClient *kubernetes.Clientset, namespaces []string, controllerClass string, watchIngressWithoutClass bool, ingressClass string, ingressClassByName bool) (*Analyzer, error) {
	// When namespaces list is empty all namespaces are listed.
	if len(namespaces) == 0 {
		namespaces = []string{v1.NamespaceAll}
	}

	// When controller class is empty, we use the default one.
	if controllerClass == "" {
		controllerClass = defaultControllerName
	}

	// When ingress class is empty, we use the default one.
	if ingressClass == "" {
		ingressClass = defaultAnnotationValue
	}

	// Initialize IngressClass listers.
	clusterFactory := kinformers.NewSharedInformerFactoryWithOptions(k8sClient, resyncPeriod)
	clusterFactory.Networking().V1().IngressClasses().Lister()

	// Initialize Ingress listers per namespace.
	var (
		nsFactories    []kinformers.SharedInformerFactory
		ingressListers []listersnetv1.IngressLister
	)
	for _, namespace := range namespaces {
		nsFactory := kinformers.NewSharedInformerFactoryWithOptions(k8sClient, resyncPeriod, kinformers.WithNamespace(namespace))
		nsFactory.Networking().V1().Ingresses().Informer()

		nsFactories = append(nsFactories, nsFactory)
		ingressListers = append(ingressListers, nsFactory.Networking().V1().Ingresses().Lister())
	}

	return &Analyzer{
		ingressClass:             ingressClass,
		controllerClass:          controllerClass,
		watchIngressWithoutClass: watchIngressWithoutClass,
		ingressClassByName:       ingressClassByName,
		clusterFactory:           clusterFactory,
		nsFactories:              nsFactories,
		ingressListers:           ingressListers,
		ingressClassLister:       clusterFactory.Networking().V1().IngressClasses().Lister(),
	}, nil
}

// Start starts the analyzer informers and waits for their caches to sync.
// This method blocks until the caches are synced or the context is done.
func (a *Analyzer) Start(ctx context.Context) error {
	// Start cluster-wide informers.
	a.clusterFactory.Start(ctx.Done())
	for t, ok := range a.clusterFactory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return fmt.Errorf("timed out waiting for cluster caches to sync %s", t.String())
		}
	}

	// Start namespaced informers.
	for _, nsFactory := range a.nsFactories {
		nsFactory.Start(ctx.Done())
		for t, ok := range nsFactory.WaitForCacheSync(ctx.Done()) {
			if !ok {
				return fmt.Errorf("timed out waiting for namespace caches to sync %s", t.String())
			}
		}
	}

	return nil
}

// GenerateReport generates the analysis report.
func (a *Analyzer) GenerateReport() error {
	ingressClasses, err := a.ingressClassLister.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("listing IngressClasses: %w", err)
	}

	var ingresses []*netv1.Ingress
	for _, ingressLister := range a.ingressListers {
		nsIngresses, err := ingressLister.List(labels.Everything())
		if err != nil {
			return fmt.Errorf("listing Ingresses: %w", err)
		}

		ingresses = append(ingresses, nsIngresses...)
	}

	report := a.computeReport(ingressClasses, ingresses)

	a.reportMu.Lock()
	a.report = report
	a.reportMu.Unlock()

	return nil
}

// Report returns the analysis report.
func (a *Analyzer) Report() Report {
	a.reportMu.RLock()
	defer a.reportMu.RUnlock()

	return a.report
}
