// Copyright Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controllers

import (
	"fmt"
	"k8s.io/client-go/informers"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	informersv1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	// The ID/name for the certificate chain in kubernetes generic secret.
	GenericScrtCert = "cert"
	// The ID/name for the private key in kubernetes generic secret.
	GenericScrtKey = "key"
	// The ID/name for the CA certificate in kubernetes generic secret.
	GenericScrtCaCert = "cacert"

	// The ID/name for the certificate chain in kubernetes tls secret.
	TLSSecretCert = "tls.crt"
	// The ID/name for the k8sKey in kubernetes tls secret.
	TLSSecretKey = "tls.key"
	// The ID/name for the CA certificate in kubernetes tls secret
	TLSSecretCaCert = "ca.crt"

	// GatewaySdsCaSuffix is the suffix of the sds resource name for root CA. All resource
	// names for gateway root certs end with "-cacert".
	GatewaySdsCaSuffix = "-cacert"
)

type CredentialsController struct {
	secrets      informersv1.SecretInformer
	secretLister listersv1.SecretLister

	mu sync.RWMutex
}

func NewCredentialsController(client *kubernetes.Clientset) *CredentialsController {
	kubeInformer := informers.NewSharedInformerFactory(client, 0)
	informer := kubeInformer.InformerFor(&v1.Secret{}, func(k kubernetes.Interface, resync time.Duration) cache.SharedIndexInformer {
		return informersv1.NewFilteredSecretInformer(
			k, metav1.NamespaceAll, resync, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
			func(options *metav1.ListOptions) {
				// We only care about TLS certificates and docker config for Wasm image pulling.
				// Unfortunately, it is not as simple as selecting type=kubernetes.io/tls and type=kubernetes.io/dockerconfigjson.
				// Because of legacy reasons and supporting an extra ca.crt, we also support generic types.
				// Its also likely users have started to use random types and expect them to continue working.
				// This makes the assumption we will never care about Helm secrets or SA token secrets - two common
				// large Secrets in clusters.
				// This is a best effort optimization only; the code would behave correctly if we watched all Secrets.
				options.FieldSelector = fields.AndSelectors(
					fields.OneTermNotEqualSelector("type", "helm.sh/release.v1"),
					fields.OneTermNotEqualSelector("type", string(v1.SecretTypeServiceAccountToken)),
				).String()
			},
		)
	})

	return &CredentialsController{
		secrets:      informerAdapter{listersv1.NewSecretLister(informer.GetIndexer()), informer},
		secretLister: listersv1.NewSecretLister(informer.GetIndexer()),
	}
}

func (s *CredentialsController) GetDockerCredential(name, namespace string) ([]byte, error) {
	k8sSecret, err := s.secretLister.Secrets(namespace).Get(name)
	if err != nil || k8sSecret == nil {
		return nil, fmt.Errorf("secret %v/%v not found", namespace, name)
	}
	if k8sSecret.Type != v1.SecretTypeDockerConfigJson {
		return nil, fmt.Errorf("type of secret %v/%v is not %v", namespace, name, v1.SecretTypeDockerConfigJson)
	}
	if cred, found := k8sSecret.Data[v1.DockerConfigJsonKey]; found {
		return cred, nil
	}
	return nil, fmt.Errorf("cannot find docker config at secret %v/%v", namespace, name)
}

func (s *CredentialsController) AddEventHandler(f func(name string, namespace string)) {
	handler := func(obj interface{}) {
		scrt, ok := obj.(*v1.Secret)
		if !ok {
			if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
				if cast, ok := tombstone.Obj.(*v1.Secret); ok {
					scrt = cast
				} else {
					log.Errorf("Failed to convert to tombstoned secret object: %v", obj)
					return
				}
			} else {
				log.Errorf("Failed to convert to secret object: %v", obj)
				return
			}
		}
		f(scrt.Name, scrt.Namespace)
	}
	s.secrets.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				handler(obj)
			},
			UpdateFunc: func(old, cur interface{}) {
				handler(cur)
			},
			DeleteFunc: func(obj interface{}) {
				handler(obj)
			},
		})
}

// informerAdapter allows treating a generic informer as an informersv1.SecretInformer
type informerAdapter struct {
	listersv1.SecretLister
	cache.SharedIndexInformer
}

func (s informerAdapter) Informer() cache.SharedIndexInformer {
	return s.SharedIndexInformer
}

func (s informerAdapter) Lister() listersv1.SecretLister {
	return s.SecretLister
}
