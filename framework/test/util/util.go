package util

import (
	"bytes"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

func LoadYamlObjects(scheme *runtime.Scheme, path string) ([]client.Object, error) {
	codecs := serializer.NewCodecFactory(scheme)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	items := bytes.Split(data, []byte("---\n"))
	objs := make([]client.Object, 0, len(items))
	for _, item := range items {
		item = bytes.TrimSpace(item)
		if len(item) == 0 {
			continue
		}
		obj, _, err := codecs.UniversalDeserializer().Decode(item, nil, nil)
		if err != nil {
			return nil, err
		}
		objs = append(objs, obj.(client.Object))
	}
	return objs, nil
}

func LoadYamlTestData[T any](receiver *T, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, receiver)
}
