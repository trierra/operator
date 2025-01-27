/*
Copyright 2019 Openstorage.org

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
	v1alpha1 "github.com/libopenstorage/operator/pkg/apis/core/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// StorageNodeStatusLister helps list StorageNodeStatuses.
type StorageNodeStatusLister interface {
	// List lists all StorageNodeStatuses in the indexer.
	List(selector labels.Selector) (ret []*v1alpha1.StorageNodeStatus, err error)
	// StorageNodeStatuses returns an object that can list and get StorageNodeStatuses.
	StorageNodeStatuses(namespace string) StorageNodeStatusNamespaceLister
	StorageNodeStatusListerExpansion
}

// storageNodeStatusLister implements the StorageNodeStatusLister interface.
type storageNodeStatusLister struct {
	indexer cache.Indexer
}

// NewStorageNodeStatusLister returns a new StorageNodeStatusLister.
func NewStorageNodeStatusLister(indexer cache.Indexer) StorageNodeStatusLister {
	return &storageNodeStatusLister{indexer: indexer}
}

// List lists all StorageNodeStatuses in the indexer.
func (s *storageNodeStatusLister) List(selector labels.Selector) (ret []*v1alpha1.StorageNodeStatus, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.StorageNodeStatus))
	})
	return ret, err
}

// StorageNodeStatuses returns an object that can list and get StorageNodeStatuses.
func (s *storageNodeStatusLister) StorageNodeStatuses(namespace string) StorageNodeStatusNamespaceLister {
	return storageNodeStatusNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// StorageNodeStatusNamespaceLister helps list and get StorageNodeStatuses.
type StorageNodeStatusNamespaceLister interface {
	// List lists all StorageNodeStatuses in the indexer for a given namespace.
	List(selector labels.Selector) (ret []*v1alpha1.StorageNodeStatus, err error)
	// Get retrieves the StorageNodeStatus from the indexer for a given namespace and name.
	Get(name string) (*v1alpha1.StorageNodeStatus, error)
	StorageNodeStatusNamespaceListerExpansion
}

// storageNodeStatusNamespaceLister implements the StorageNodeStatusNamespaceLister
// interface.
type storageNodeStatusNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all StorageNodeStatuses in the indexer for a given namespace.
func (s storageNodeStatusNamespaceLister) List(selector labels.Selector) (ret []*v1alpha1.StorageNodeStatus, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.StorageNodeStatus))
	})
	return ret, err
}

// Get retrieves the StorageNodeStatus from the indexer for a given namespace and name.
func (s storageNodeStatusNamespaceLister) Get(name string) (*v1alpha1.StorageNodeStatus, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("storagenodestatus"), name)
	}
	return obj.(*v1alpha1.StorageNodeStatus), nil
}
