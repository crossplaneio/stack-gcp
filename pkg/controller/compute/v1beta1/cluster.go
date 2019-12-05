/*
Copyright 2019 The Crossplane Authors.

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

package v1beta1

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"k8s.io/client-go/tools/clientcmd"

	"github.com/pkg/errors"
	container "google.golang.org/api/container/v1beta1"
	"google.golang.org/api/option"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/crossplaneio/crossplane-runtime/apis/core/v1alpha1"
	runtimev1alpha1 "github.com/crossplaneio/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplaneio/crossplane-runtime/pkg/logging"
	"github.com/crossplaneio/crossplane-runtime/pkg/meta"
	"github.com/crossplaneio/crossplane-runtime/pkg/resource"

	"github.com/crossplaneio/stack-gcp/apis/compute/v1beta1"
	gcpv1alpha3 "github.com/crossplaneio/stack-gcp/apis/v1alpha3"
	gcp "github.com/crossplaneio/stack-gcp/pkg/clients"
	gke "github.com/crossplaneio/stack-gcp/pkg/clients/container"
)

const (
	controllerName    = "gke.compute.gcp.crossplane.io"
	finalizer         = "finalizer." + controllerName
	clusterNamePrefix = "gke-"

	requeueOnWait   = 30 * time.Second
	requeueOnSucces = 2 * time.Minute

	updateErrorMessageFormat         = "failed to update cluster object: %s"
	erroredClusterErrorMessageFormat = "gke cluster is in %s state with message: %s"
)

// Error strings.
const (
	errGetProvider         = "cannot get Provider"
	errGetProviderSecret   = "cannot get Provider Secret"
	errNewClient           = "cannot create new GKE cluster client"
	errManagedUpdateFailed = "cannot update GKECluster custom resource"
	errNotCluster          = "managed resource is not a GKECluster"
	errUpdateCR            = "cannot update GKECluster custom resource"
	errGetCluster          = "cannot get GKE cluster"
	errCreateCluster       = "cannot create GKE cluster"
	errUpdateCluster       = "cannot update GKE cluster"
	errDeleteCluster       = "cannot delete GKE cluster"
)

// Amounts of time we wait before requeuing a reconcile.
const (
	aLongWait = 60 * time.Second
)

// Error strings
const (
	errUpdateManagedStatus = "cannot update managed resource status"
)

var (
	log           = logging.Logger.WithName("controller." + controllerName)
	ctx           = context.Background()
	result        = reconcile.Result{}
	resultRequeue = reconcile.Result{Requeue: true}
)

// GKEClusterController is responsible for adding the GKECluster
// controller and its corresponding reconciler to the manager with any runtime configuration.
type GKEClusterController struct{}

// SetupWithManager creates a new Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func (c *GKEClusterController) SetupWithManager(mgr ctrl.Manager) error {
	r := resource.NewManagedReconciler(mgr,
		resource.ManagedKind(v1beta1.GKEClusterGroupVersionKind),
		resource.WithExternalConnecter(&gkeConnecter{kube: mgr.GetClient(), newServiceFn: container.NewService}))

	name := strings.ToLower(fmt.Sprintf("%s.%s", v1beta1.GKEClusterKindAPIVersion, v1beta1.Group))

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		For(&v1beta1.GKECluster{}).
		Complete(r)
}

type gkeConnecter struct {
	kube         client.Client
	newServiceFn func(ctx context.Context, opts ...option.ClientOption) (*container.Service, error)
}

func (c *gkeConnecter) Connect(ctx context.Context, mg resource.Managed) (resource.ExternalClient, error) {
	i, ok := mg.(*v1beta1.GKECluster)
	if !ok {
		return nil, errors.New(errNotCluster)
	}

	p := &gcpv1alpha3.Provider{}
	if err := c.kube.Get(ctx, meta.NamespacedNameOf(i.Spec.ProviderReference), p); err != nil {
		return nil, errors.Wrap(err, errGetProvider)
	}

	s := &corev1.Secret{}
	n := types.NamespacedName{Namespace: p.Spec.Secret.Namespace, Name: p.Spec.Secret.Name}
	if err := c.kube.Get(ctx, n, s); err != nil {
		return nil, errors.Wrap(err, errGetProviderSecret)
	}

	gke, err := c.newServiceFn(ctx,
		option.WithCredentialsJSON(s.Data[p.Spec.Secret.Key]),
		option.WithScopes(container.CloudPlatformScope))
	return &external{cluster: *gke, projectID: p.Spec.ProjectID, kube: c.kube}, errors.Wrap(err, errNewClient)
}

type external struct {
	kube      client.Client
	cluster   container.Service
	projectID string
}

func (e *external) Observe(ctx context.Context, mg resource.Managed) (resource.ExternalObservation, error) {
	cr, ok := mg.(*v1beta1.GKECluster)
	if !ok {
		return resource.ExternalObservation{}, errors.New(errNotCluster)
	}

	existing, err := e.cluster.Projects.Locations.Clusters.Get(meta.GetExternalName(mg)).Context(ctx).Do()
	if err != nil {
		return resource.ExternalObservation{}, errors.Wrap(resource.Ignore(gcp.IsErrorNotFound, err), errGetCluster)
	}

	cr.Status.AtProvider = gke.GenerateObservation(*existing)
	currentSpec := cr.Spec.ForProvider.DeepCopy()
	gke.LateInitializeSpec(&cr.Spec.ForProvider, *existing)
	if !reflect.DeepEqual(currentSpec, &cr.Spec.ForProvider) {
		if err := e.kube.Update(ctx, cr); err != nil {
			return resource.ExternalObservation{}, errors.Wrap(err, errManagedUpdateFailed)
		}
	}
	switch cr.Status.AtProvider.Status {
	case v1beta1.ClusterStateRunning:
		cr.Status.SetConditions(v1alpha1.Available())
		resource.SetBindable(cr)
	case v1beta1.ClusterStateProvisioning:
		cr.Status.SetConditions(v1alpha1.Creating())
	case v1beta1.ClusterStateUnspecified, v1beta1.ClusterStateDegraded, v1beta1.ClusterStateError, v1beta1.ClusterStateReconciling:
		cr.Status.SetConditions(v1alpha1.Unavailable())
	}

	upToDate, _ := gke.IsUpToDate(&cr.Spec.ForProvider, *existing)

	return resource.ExternalObservation{
		ResourceExists:    true,
		ResourceUpToDate:  upToDate,
		ConnectionDetails: connectionDetails(existing),
	}, nil
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (resource.ExternalCreation, error) {
	i, ok := mg.(*v1beta1.GKECluster)
	if !ok {
		return resource.ExternalCreation{}, errors.New(errNotCluster)
	}

	cluster := gke.GenerateCluster(i.Spec.ForProvider)
	create := &container.CreateClusterRequest{
		Cluster: cluster,
	}
	if _, err := e.cluster.Projects.Locations.Clusters.Create("TODO", create).Context(ctx).Do(); err != nil {
		return resource.ExternalCreation{}, errors.Wrap(err, errCreateCluster)
	}

	// TODO(hasheddan): go ahead and propagate username / password here if set in spec?
	return resource.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, mg resource.Managed) (resource.ExternalUpdate, error) {
	i, ok := mg.(*v1beta1.GKECluster)
	if !ok {
		return resource.ExternalUpdate{}, errors.New(errNotCluster)
	}

	// We have to get the cluster again here
	existing, err := e.cluster.Projects.Locations.Clusters.Get(meta.GetExternalName(mg)).Context(ctx).Do()
	if err != nil {
		return resource.ExternalUpdate{}, errors.Wrap(err, errGetCluster)
	}

	upToDate, updateFn := gke.IsUpToDate(&i.Spec.ForProvider, *existing)
	if upToDate {
		return resource.ExternalUpdate{}, nil
	}

	_, err = updateFn(e.cluster, ctx)
	return resource.ExternalUpdate{}, errors.Wrap(err, errUpdateCluster)
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) error {
	i, ok := mg.(*v1beta1.GKECluster)
	if !ok {
		return errors.New(errNotCluster)
	}
	i.SetConditions(runtimev1alpha1.Deleting())

	_, err := e.cluster.Projects.Locations.Clusters.Delete(meta.GetExternalName(mg)).Context(ctx).Do()
	if gcp.IsErrorNotFound(err) {
		return nil
	}
	return errors.Wrap(err, errDeleteCluster)
}

// connectionSecret return secret object for cluster instance
func connectionDetails(cluster *container.Cluster) resource.ConnectionDetails {
	config, err := gke.GenerateClientConfig(cluster)
	if err != nil {
		return nil
	}
	rawConfig, err := clientcmd.Write(config)
	if err != nil {
		return nil
	}
	cd := resource.ConnectionDetails{
		runtimev1alpha1.ResourceCredentialsSecretEndpointKey:   []byte(config.Clusters[cluster.Name].Server),
		runtimev1alpha1.ResourceCredentialsSecretUserKey:       []byte(config.AuthInfos[cluster.Name].Username),
		runtimev1alpha1.ResourceCredentialsSecretPasswordKey:   []byte(config.AuthInfos[cluster.Name].Password),
		runtimev1alpha1.ResourceCredentialsSecretCAKey:         config.Clusters[cluster.Name].CertificateAuthorityData,
		runtimev1alpha1.ResourceCredentialsSecretClientCertKey: config.AuthInfos[cluster.Name].ClientCertificateData,
		runtimev1alpha1.ResourceCredentialsSecretClientKeyKey:  config.AuthInfos[cluster.Name].ClientKeyData,
		runtimev1alpha1.ResourceCredentialsSecretKubeconfigKey: rawConfig,
	}
	return cd
}
