package preview

import (
	"errors"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

const NamespaceCompositionPath = "platform/crossplane/compositions/namespace.yaml"

// ParseNamespaceComposition accepts one narrow, executable source change. The
// installer reconstructs the approved Composition from trusted local source;
// PR YAML is never copied into GitOps or applied to either cluster.
func ParseNamespaceComposition(content string) (int, error) {
	var doc struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
		Metadata   struct {
			Name string `yaml:"name"`
		} `yaml:"metadata"`
		Spec struct {
			CompositeTypeRef struct {
				APIVersion string `yaml:"apiVersion"`
				Kind       string `yaml:"kind"`
			} `yaml:"compositeTypeRef"`
			Mode     string `yaml:"mode"`
			Pipeline []struct {
				Step        string `yaml:"step"`
				FunctionRef struct {
					Name string `yaml:"name"`
				} `yaml:"functionRef"`
				Input struct {
					APIVersion string `yaml:"apiVersion"`
					Kind       string `yaml:"kind"`
					Mode       string `yaml:"mode"`
					PodBudget  int    `yaml:"podBudget"`
				} `yaml:"input"`
			} `yaml:"pipeline"`
		} `yaml:"spec"`
	}
	decoder := yaml.NewDecoder(strings.NewReader(content))
	decoder.KnownFields(true)
	if err := decoder.Decode(&doc); err != nil {
		return 0, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return 0, errors.New("Composition must contain exactly one document")
	}
	if doc.APIVersion != "apiextensions.crossplane.io/v1" || doc.Kind != "Composition" ||
		doc.Metadata.Name != "preview-namespace" ||
		doc.Spec.CompositeTypeRef.APIVersion != "preview.platform.example.org/v1alpha1" ||
		doc.Spec.CompositeTypeRef.Kind != "PreviewEnvironment" || doc.Spec.Mode != "Pipeline" ||
		len(doc.Spec.Pipeline) != 1 {
		return 0, errors.New("Composition identity or pipeline changed")
	}
	step := doc.Spec.Pipeline[0]
	if step.Step != "render-preview-resources" || step.FunctionRef.Name != "function-preview-resources" ||
		step.Input.APIVersion != "preview.fn.platform.example.org/v1alpha1" ||
		step.Input.Kind != "PreviewResourcesInput" || step.Input.Mode != "namespace" ||
		step.Input.PodBudget != 5 {
		return 0, errors.New("only namespace podBudget 4 to 5 is supported")
	}
	return step.Input.PodBudget, nil
}
