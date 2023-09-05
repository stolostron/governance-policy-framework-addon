package utils

import (
	"errors"
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
)

var ErrNoVersionedResource = errors.New("the resource version was not found")

// GVRFromGVK uses the discovery client to get the versioned resource and determines if the resource is namespaced. If
// the resource is not found or could not be retrieved, an error is always returned.
func GVRFromGVK(
	discoveryClient discovery.DiscoveryInterface, gvk schema.GroupVersionKind,
) (
	schema.GroupVersionResource, bool, error,
) {
	rsrcList, err := discoveryClient.ServerResourcesForGroupVersion(gvk.GroupVersion().String())
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return schema.GroupVersionResource{}, false, fmt.Errorf("%w: %s", ErrNoVersionedResource, gvk.String())
		}

		return schema.GroupVersionResource{}, false, err
	}

	for _, rsrc := range rsrcList.APIResources {
		if rsrc.Kind == gvk.Kind {
			gvr := schema.GroupVersionResource{
				Group:    gvk.Group,
				Version:  gvk.Version,
				Resource: rsrc.Name,
			}

			return gvr, rsrc.Namespaced, nil
		}
	}

	return schema.GroupVersionResource{}, false, fmt.Errorf(
		"%w: no matching kind was found: %s", ErrNoVersionedResource, gvk.String(),
	)
}
