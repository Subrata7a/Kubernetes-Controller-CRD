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

// Code generated by lister-gen. DO NOT EDIT.

package v1alpha1

import (
	v1alpha1 "SampleCRDControlle/pkg/apis/samplecontroller/v1alpha1"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// SccLister helps list Sccs.
// All objects returned here must be treated as read-only.
type SccLister interface {
	// List lists all Sccs in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.Scc, err error)
	// Sccs returns an object that can list and get Sccs.
	Sccs(namespace string) SccNamespaceLister
	SccListerExpansion
}

// sccLister implements the SccLister interface.
type sccLister struct {
	indexer cache.Indexer
}

// NewSccLister returns a new SccLister.
func NewSccLister(indexer cache.Indexer) SccLister {
	return &sccLister{indexer: indexer}
}

// List lists all Sccs in the indexer.
func (s *sccLister) List(selector labels.Selector) (ret []*v1alpha1.Scc, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.Scc))
	})
	return ret, err
}

// Sccs returns an object that can list and get Sccs.
func (s *sccLister) Sccs(namespace string) SccNamespaceLister {
	return sccNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// SccNamespaceLister helps list and get Sccs.
// All objects returned here must be treated as read-only.
type SccNamespaceLister interface {
	// List lists all Sccs in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.Scc, err error)
	// Get retrieves the Scc from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1alpha1.Scc, error)
	SccNamespaceListerExpansion
}

// sccNamespaceLister implements the SccNamespaceLister
// interface.
type sccNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all Sccs in the indexer for a given namespace.
func (s sccNamespaceLister) List(selector labels.Selector) (ret []*v1alpha1.Scc, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.Scc))
	})
	return ret, err
}

// Get retrieves the Scc from the indexer for a given namespace and name.
func (s sccNamespaceLister) Get(name string) (*v1alpha1.Scc, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("scc"), name)
	}
	return obj.(*v1alpha1.Scc), nil
}