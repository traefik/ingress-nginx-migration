package analyzer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/labels"
	kinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/listers/networking/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const resyncPeriod = 5 * time.Minute

type Analyzer struct {
	k8sClient     *kubernetes.Clientset
	ingressLister v1.IngressLister
}

func New(ctx context.Context, kubeconfig string, namespace string) (*Analyzer, error) {
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
		return nil, fmt.Errorf("creating k8s client: %w", err)
	}

	k8sFactory := kinformers.NewSharedInformerFactoryWithOptions(k8sClient, resyncPeriod, kinformers.WithNamespace(namespace))

	// Getting the informer will make the cache get populated and usable with listers.
	k8sFactory.Networking().V1().Ingresses().Informer()
	k8sFactory.Networking().V1().IngressClasses().Informer()

	k8sFactory.Start(ctx.Done())

	ctxWithTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	for t, ok := range k8sFactory.WaitForCacheSync(ctxWithTimeout.Done()) {
		if !ok {
			return nil, fmt.Errorf("timed out waiting for K8s caches to sync %s", t.String())
		}
	}

	return &Analyzer{
		k8sClient:     k8sClient,
		ingressLister: k8sFactory.Networking().V1().Ingresses().Lister(),
	}, nil
}

func (a *Analyzer) Report() (Report, error) {
	ingresses, err := a.ingressLister.List(labels.Everything())
	if err != nil {
		return Report{}, fmt.Errorf("listing Ingresses: %w", err)
	}

	return computeReport(ingresses), nil
}
