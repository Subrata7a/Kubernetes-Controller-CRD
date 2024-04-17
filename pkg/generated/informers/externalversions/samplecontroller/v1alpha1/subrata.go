/*
Copyright The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by informer-gen. DO NOT EDIT.

package v1alpha1

import (
	"context"
	samplecontrollerv1alpha1 "k8s-sample-controller/pkg/apis/samplecontroller/v1alpha1"
	versioned "k8s-sample-controller/pkg/generated/clientset/versioned"
	internalinterfaces "k8s-sample-controller/pkg/generated/informers/externalversions/internalinterfaces"
	v1alpha1 "k8s-sample-controller/pkg/generated/listers/samplecontroller/v1alpha1"
	time "time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
)

// SubrataInformer provides access to a shared informer and lister for
// Subratas.
type SubrataInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1alpha1.SubrataLister
}

type subrataInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
	namespace        string
}

// NewSubrataInformer constructs a new informer for Subrata type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewSubrataInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredSubrataInformer(client, namespace, resyncPeriod, indexers, nil)
}

// NewFilteredSubrataInformer constructs a new informer for Subrata type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredSubrataInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.SamplecontrollerV1alpha1().Subratas(namespace).List(context.TODO(), options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.SamplecontrollerV1alpha1().Subratas(namespace).Watch(context.TODO(), options)
			},
		},
		&samplecontrollerv1alpha1.Subrata{},
		resyncPeriod,
		indexers,
	)
}

func (f *subrataInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredSubrataInformer(client, f.namespace, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *subrataInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&samplecontrollerv1alpha1.Subrata{}, f.defaultInformer)
}

func (f *subrataInformer) Lister() v1alpha1.SubrataLister {
	return v1alpha1.NewSubrataLister(f.Informer().GetIndexer())
}