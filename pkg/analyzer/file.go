package analyzer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/rs/zerolog/log"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	listersnetv1 "k8s.io/client-go/listers/networking/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	kindIngress      = "Ingress"
	kindIngressClass = "IngressClass"
	kindList         = "List"
)

// StdinInputFile is the input file path that reads the resources from stdin.
const StdinInputFile = "-"

// typedObject holds the fields needed to route a manifest to the right Go type.
type typedObject struct {
	APIVersion string            `json:"apiVersion"`
	Kind       string            `json:"kind"`
	Items      []json.RawMessage `json:"items"`
}

// NewFromFile creates a new Analyzer from the Ingress and IngressClass resources
// stored in a file, instead of reading them from a Kubernetes cluster.
// The file can be YAML or JSON, holding one or more documents, each being a
// single resource or a List (e.g. the output of "kubectl get ingress -A -o yaml").
// When the file holds no IngressClass, one named after ingressClass and handled
// by controllerClass is assumed, so Ingresses using spec.ingressClassName still match.
func NewFromFile(path string, namespaces []string, controllerClass string, watchIngressWithoutClass bool, ingressClass string, ingressClassByName bool) (*Analyzer, error) {
	controllerClass, ingressClass = withClassDefaults(controllerClass, ingressClass)

	r, closeFn, err := openInputFile(path)
	if err != nil {
		return nil, err
	}
	defer closeFn()

	ingresses, ingressClasses, err := decodeResources(r)
	if err != nil {
		return nil, fmt.Errorf("reading resources from %q: %w", path, err)
	}

	if len(ingressClasses) == 0 {
		log.Info().Msgf("No IngressClass found in %q, assuming IngressClass %q is handled by controller %q", path, ingressClass, controllerClass)

		ingressClasses = append(ingressClasses, &netv1.IngressClass{
			ObjectMeta: metav1.ObjectMeta{Name: ingressClass},
			Spec:       netv1.IngressClassSpec{Controller: controllerClass},
		})
	}

	ingressIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	for _, ing := range ingresses {
		if len(namespaces) > 0 && !slices.Contains(namespaces, ing.Namespace) {
			continue
		}

		if err := ingressIndexer.Add(ing); err != nil {
			return nil, fmt.Errorf("indexing Ingress %s/%s: %w", ing.Namespace, ing.Name, err)
		}
	}

	ingressClassIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	for _, ic := range ingressClasses {
		if err := ingressClassIndexer.Add(ic); err != nil {
			return nil, fmt.Errorf("indexing IngressClass %s: %w", ic.Name, err)
		}
	}

	log.Info().Msgf("Loaded %d Ingress(es) and %d IngressClass(es) from %q", len(ingressIndexer.List()), len(ingressClasses), path)

	return &Analyzer{
		ingressClass:             ingressClass,
		controllerClass:          controllerClass,
		watchIngressWithoutClass: watchIngressWithoutClass,
		ingressClassByName:       ingressClassByName,
		ingressListers:           []listersnetv1.IngressLister{listersnetv1.NewIngressLister(ingressIndexer)},
		ingressClassLister:       listersnetv1.NewIngressClassLister(ingressClassIndexer),
	}, nil
}

func openInputFile(path string) (io.Reader, func(), error) {
	if path == StdinInputFile {
		return os.Stdin, func() {}, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("opening input file: %w", err)
	}

	return f, func() { _ = f.Close() }, nil
}

// decodeResources reads all YAML or JSON documents from r and returns the
// Ingresses and IngressClasses they hold. Other kinds are skipped.
func decodeResources(r io.Reader) ([]*netv1.Ingress, []*netv1.IngressClass, error) {
	var (
		ingresses      []*netv1.Ingress
		ingressClasses []*netv1.IngressClass
	)

	decoder := yaml.NewYAMLOrJSONDecoder(r, 4096)
	for {
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return nil, nil, fmt.Errorf("decoding document: %w", err)
		}

		if err := decodeObject(raw, &ingresses, &ingressClasses); err != nil {
			return nil, nil, err
		}
	}

	return ingresses, ingressClasses, nil
}

func decodeObject(raw json.RawMessage, ingresses *[]*netv1.Ingress, ingressClasses *[]*netv1.IngressClass) error {
	// Empty documents (e.g. a trailing "---") decode to null.
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil
	}

	var obj typedObject
	if err := json.Unmarshal(raw, &obj); err != nil {
		return fmt.Errorf("decoding object: %w", err)
	}

	switch {
	case strings.HasSuffix(obj.Kind, kindList):
		for _, item := range obj.Items {
			if err := decodeObject(item, ingresses, ingressClasses); err != nil {
				return err
			}
		}

	case obj.Kind == kindIngress:
		if obj.APIVersion != netv1.SchemeGroupVersion.String() {
			return fmt.Errorf("unsupported Ingress apiVersion %q (only %q is supported)", obj.APIVersion, netv1.SchemeGroupVersion.String())
		}

		var ing netv1.Ingress
		if err := json.Unmarshal(raw, &ing); err != nil {
			return fmt.Errorf("decoding Ingress: %w", err)
		}
		*ingresses = append(*ingresses, &ing)

	case obj.Kind == kindIngressClass:
		if obj.APIVersion != netv1.SchemeGroupVersion.String() {
			return fmt.Errorf("unsupported IngressClass apiVersion %q (only %q is supported)", obj.APIVersion, netv1.SchemeGroupVersion.String())
		}

		var ic netv1.IngressClass
		if err := json.Unmarshal(raw, &ic); err != nil {
			return fmt.Errorf("decoding IngressClass: %w", err)
		}
		*ingressClasses = append(*ingressClasses, &ic)

	default:
		log.Debug().Msgf("Skipping unsupported resource kind %q", obj.Kind)
	}

	return nil
}
