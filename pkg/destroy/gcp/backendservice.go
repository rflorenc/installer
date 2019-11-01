package gcp

import (
	"github.com/pkg/errors"

	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
)

func (o *ClusterUninstaller) listBackendServices() ([]cloudResource, error) {
	return o.listBackendServicesWithFilter("items(name),nextPageToken", o.clusterIDFilter(), nil)
}

// listBackendServicesWithFilter lists backend services in the project that satisfy the filter criteria.
// The fields parameter specifies which fields should be returned in the result, the filter string contains
// a filter string passed to the API to filter results. The filterFunc is a client-side filtering function
// that determines whether a particular result should be returned or not.
func (o *ClusterUninstaller) listBackendServicesWithFilter(fields string, filter string, filterFunc func(*compute.BackendService) bool) ([]cloudResource, error) {
	o.Logger.Debugf("Listing backend services")
	result := []cloudResource{}
	ctx, cancel := o.contextWithTimeout()
	defer cancel()
	req := o.computeSvc.RegionBackendServices.List(o.ProjectID, o.Region).Fields(googleapi.Field(fields))
	if len(filter) > 0 {
		req = req.Filter(filter)
	}
	err := req.Pages(ctx, func(list *compute.BackendServiceList) error {
		for _, backendService := range list.Items {
			if filterFunc == nil || filterFunc != nil && filterFunc(backendService) {
				o.Logger.Debugf("Found backend service: %s", backendService.Name)
				result = append(result, cloudResource{
					key:      backendService.Name,
					name:     backendService.Name,
					typeName: "backendservice",
				})
			}
		}
		return nil
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list backend services")
	}
	return result, nil
}

func (o *ClusterUninstaller) deleteBackendService(item cloudResource) error {
	o.Logger.Debugf("Deleting backend service %s", item.name)
	ctx, cancel := o.contextWithTimeout()
	defer cancel()
	op, err := o.computeSvc.RegionBackendServices.Delete(o.ProjectID, o.Region, item.name).RequestId(o.requestID(item.typeName, item.name)).Context(ctx).Do()
	if err != nil && !isNoOp(err) {
		o.resetRequestID(item.typeName, item.name)
		return errors.Wrapf(err, "failed to delete backend service %s", item.name)
	}
	if op != nil && op.Status == "DONE" && isErrorStatus(op.HttpErrorStatusCode) {
		o.resetRequestID(item.typeName, item.name)
		return errors.Errorf("failed to delete backend service %s with error: %s", item.name, operationErrorMessage(op))
	}
	return nil
}

// destroyBackendServices removes backend services with a name prefixed by the
// cluster's infra ID.
func (o *ClusterUninstaller) destroyBackendServices() error {
	backendServices, err := o.listBackendServices()
	if err != nil {
		return err
	}
	found := cloudResources{}
	errs := []error{}
	for _, backendService := range backendServices {
		found.insert(backendService)
		err := o.deleteBackendService(backendService)
		if err != nil {
			errs = append(errs, err)
		}
	}
	deleted := o.setPendingItems("backendservice", found)
	for _, item := range deleted {
		o.Logger.Infof("Deleted backend service %s", item.name)
	}
	return aggregateError(errs, len(found))
}
