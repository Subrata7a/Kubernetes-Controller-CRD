/*
Copyright 2017 The Kubernetes Authors.

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

package controller

import (
	"context"
	"fmt"
	"golang.org/x/time/rate"
	"k8s.io/apimachinery/pkg/util/intstr"
	"strconv"

	"log"
	"time"

	samplev1alpha1 "SampleCRDControlle/pkg/apis/samplecontroller/v1alpha1"
	clientset "SampleCRDControlle/pkg/generated/clientset/versioned"
	samplescheme "SampleCRDControlle/pkg/generated/clientset/versioned/scheme"
	informers "SampleCRDControlle/pkg/generated/informers/externalversions/samplecontroller/v1alpha1"
	listers "SampleCRDControlle/pkg/generated/listers/samplecontroller/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	appsinformers "k8s.io/client-go/informers/apps/v1"
	corev1informers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	appslisters "k8s.io/client-go/listers/apps/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
)

const controllerAgentName = "sample-controller"

const (
	// SuccessSynced is used as part of the Event 'reason' when a Scc is synced
	SuccessSynced = "Synced"
	// ErrResourceExists is used as part of the Event 'reason' when a Scc fails
	// to sync due to a Deployment of the same name already existing.
	ErrResourceExists = "ErrResourceExists"

	// MessageResourceExists is the message used for Events when a resource
	// fails to sync due to a Deployment already existing
	MessageResourceExists = "Resource %q already exists and is not managed by Scc"
	// MessageResourceSynced is the message used for an Event fired when a Scc
	// is synced successfully
	MessageResourceSynced = "Scc synced successfully"
)

// Controller is the controller implementation for Scc resources
type Controller struct {
	// kubeclientset is a standard kubernetes clientset
	kubeclientset kubernetes.Interface
	// sampleclientset is a clientset for our own API group
	sampleclientset clientset.Interface

	// Deployment
	deploymentsLister appslisters.DeploymentLister
	deploymentsSynced cache.InformerSynced

	// Service
	serviceLister corev1lister.ServiceLister
	serviceSynced cache.InformerSynced

	// Scc Resource
	sccsLister listers.SccLister
	sccsSynced cache.InformerSynced

	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue workqueue.RateLimitingInterface

	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder
}

// NewController returns a new sample controller
func NewController(
	ctx context.Context,
	kubeclientset kubernetes.Interface,
	sampleclientset clientset.Interface,

	deploymentInformer appsinformers.DeploymentInformer,
	serviceInformer corev1informers.ServiceInformer,

	SccInformer informers.SccInformer) *Controller {
	logger := klog.FromContext(ctx)

	// Create event broadcaster
	// Add sample-controller types to the default Kubernetes Scheme so Events can be
	// logged for sample-controller types.
	utilruntime.Must(samplescheme.AddToScheme(scheme.Scheme))
	logger.V(4).Info("Creating event broadcaster")

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartStructuredLogging(0)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})
	ratelimiter := workqueue.NewMaxOfRateLimiter(
		workqueue.NewItemExponentialFailureRateLimiter(5*time.Millisecond, 1000*time.Second),
		&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(50), 300)},
	)

	controller := &Controller{
		// ClientSet
		kubeclientset:   kubeclientset,
		sampleclientset: sampleclientset,

		// Deployment
		deploymentsLister: deploymentInformer.Lister(),
		deploymentsSynced: deploymentInformer.Informer().HasSynced,

		//Service
		serviceLister: serviceInformer.Lister(),
		serviceSynced: serviceInformer.Informer().HasSynced,

		// Scc Resource
		sccsLister: SccInformer.Lister(),
		sccsSynced: SccInformer.Informer().HasSynced,

		workqueue: workqueue.NewRateLimitingQueue(ratelimiter),
		recorder:  recorder,
	}

	logger.Info("Setting up event handlers")
	// Set up an event handler for when Scc resources change
	SccInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueScc,
		UpdateFunc: func(old, new interface{}) {
			controller.enqueueScc(new)
		},
	})

	// Set up an event handler for when Deployment resources change. This
	// handler will lookup the owner of the given Deployment, and if it is
	// owned by a Scc resource then the handler will enqueue that Scc resource for
	// processing. This way, we don't need to implement custom logic for
	// handling Deployment resources. More info on this pattern:
	// https://github.com/kubernetes/community/blob/8cafef897a22026d42f5e5bb3f104febe7e29830/contributors/devel/controllers.md
	deploymentInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.handleObject,
		UpdateFunc: func(old, new interface{}) {
			newDepl := new.(*appsv1.Deployment)
			oldDepl := old.(*appsv1.Deployment)
			if newDepl.ResourceVersion == oldDepl.ResourceVersion {
				// Periodic resync will send update events for all known Deployments.
				// Two different versions of the same Deployment will always have different RVs.
				return
			}
			controller.handleObject(new)
		},
		DeleteFunc: controller.handleObject,
	})

	// Set up an event handler to handle Service
	serviceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.handleObject,
		UpdateFunc: func(old, new interface{}) {
			oldSvc := old.(*corev1.Service)
			newSvc := new.(*corev1.Service)
			if oldSvc.ResourceVersion == newSvc.ResourceVersion {
				return
			}
			controller.handleObject(new)
		},
		DeleteFunc: controller.handleObject,
	})
	return controller
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *Controller) Run(ctx context.Context, workers int) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()
	logger := klog.FromContext(ctx)

	// Start the informer factories to begin populating the informer caches
	logger.Info("Starting Scc controller")

	// Wait for the caches to be synced before starting workers
	logger.Info("Waiting for informer caches to sync")

	if ok := cache.WaitForCacheSync(ctx.Done(), c.deploymentsSynced, c.serviceSynced, c.sccsSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	logger.Info("Starting workers", "count", workers)
	// Launch two workers to process Scc resources
	for i := 0; i < workers; i++ {
		go wait.UntilWithContext(ctx, c.runWorker, time.Second)
	}

	logger.Info("Started workers")
	<-ctx.Done()
	logger.Info("Shutting down workers")

	return nil
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *Controller) runWorker(ctx context.Context) {
	for c.processNextWorkItem(ctx) {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *Controller) processNextWorkItem(ctx context.Context) bool {
	obj, shutdown := c.workqueue.Get()
	logger := klog.FromContext(ctx)

	if shutdown {
		return false
	}

	// We wrap this block in a func. So we can defer c.workqueue.Done.
	err := func(obj interface{}) error {
		// We call Done here so the workqueue knows we have finished
		// processing this item. We also must remember to call Forget if we
		// do not want this work item being re-queued. For example, we do
		// not call Forget if a transient error occurs, instead the item is
		// put back on the workqueue and attempted again after a back-off
		// period.
		defer c.workqueue.Done(obj)
		var key string
		var ok bool
		// We expect strings to come off the workqueue. These are of the
		// form namespace/name. We do this as the delayed nature of the
		// workqueue means the items in the informer cache may actually be
		// more up to date that when the item was initially put onto the
		// workqueue.
		if key, ok = obj.(string); !ok {
			// As the item in the workqueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			c.workqueue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		// Run the syncHandler, passing it the namespace/name string of the
		// Scc resource to be synced.
		//fmt.Println(ctx)
		//	fmt.Println(key)
		if err := c.syncHandler(ctx, key); err != nil {
			// Put the item back on the workqueue to handle any transient errors.
			c.workqueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(obj)
		logger.Info("Successfully synced", "resourceName", key)
		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(err)
		return true
	}
	return true
}

func generateDeploymentName(sccName string, sccDeploymentName string, resourceCreationTimestamp int64) string {
	deploymentName := fmt.Sprintf("%s-%s-%s", sccName, sccDeploymentName, strconv.FormatInt(resourceCreationTimestamp, 10))
	if sccDeploymentName == "" {
		deploymentName = fmt.Sprintf("%s-%s", sccName, strconv.FormatInt(resourceCreationTimestamp, 10))
	}
	return deploymentName
}
func generateServiceName(sccName string, sccServiceName string, resourceCreationTimestamp int64) string {
	deploymentName := fmt.Sprintf("%s-%s-%s", sccName, sccServiceName, strconv.FormatInt(resourceCreationTimestamp, 10))
	if sccServiceName == "" {
		deploymentName = fmt.Sprintf("%s-%s", sccName, strconv.FormatInt(resourceCreationTimestamp, 10))
	}
	return deploymentName
}

func isReplicasChanged(sccReplicas int32, deploymentReplicas int32) bool {
	if sccReplicas != 0 && sccReplicas != deploymentReplicas {
		return true
	}
	return false
}
func isDeploymentNameChanged(sccDeploymentName string, deploymentName string) bool {
	if sccDeploymentName != "" && sccDeploymentName != deploymentName {
		return true
	}
	return false
}
func isDeploymentImageChanged(sccDeploymentImage string, deploymentImage string) bool {
	if sccDeploymentImage != "" && sccDeploymentImage != deploymentImage {
		return true
	}
	return false
}

func isServiceNameChanged(sccServiceName string, serviceName string) bool {
	if sccServiceName != "" && sccServiceName != serviceName {
		return true
	}
	return false
}
func isServicePortChanged(sccServicePort int32, servicePort int32) bool {
	if sccServicePort != 0 && sccServicePort != servicePort {
		return true
	}
	return false
}

// syncHandler compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the Scc resource
// with the current status of the resource.
func (c *Controller) syncHandler(ctx context.Context, key string) error {

	// Convert the namespace/name string into a distinct namespace and name
	logger := klog.LoggerWithValues(klog.FromContext(ctx), "resourceName", key)

	namespace, name, err := cache.SplitMetaNamespaceKey(key)

	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	// Get the Scc resource with this namespace/name
	Scc, err := c.sccsLister.Sccs(namespace).Get(name)

	if err != nil {
		//The Scc resource may no longer exist, in which case we stop processing.
		if errors.IsNotFound(err) {
			utilruntime.HandleError(fmt.Errorf("Scc '%s' in work queue no longer exists", key))
			return nil
		}
		return err
	}

	// Get Resource CreationTimestamp
	resourceCreationTimestamp := Scc.CreationTimestamp.Unix()
	// Deployment Name
	deploymentName := generateDeploymentName(Scc.Name, Scc.Spec.DeploymentConfig.Name, resourceCreationTimestamp)

	// Check DeletionPolicy
	if Scc.Spec.DeletionPolicy == "" {
		Scc.Spec.DeletionPolicy = "WipeOut"
	}

	// If DeletionPolicy is WipeOut, add owner reference
	updateDeployment := newDeployment(Scc, deploymentName)
	if Scc.Spec.DeletionPolicy == "WipeOut" {
		updateDeployment.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
			*metav1.NewControllerRef(Scc, samplev1alpha1.SchemeGroupVersion.WithKind("Scc")),
		}
	}

	// Get the deployment with the name specified in Scc.spec
	deployment, err := c.deploymentsLister.Deployments(Scc.ObjectMeta.Namespace).Get(deploymentName)
	// If the resource doesn't exist, we'll create it
	if errors.IsNotFound(err) {
		deployment, err = c.kubeclientset.AppsV1().Deployments(Scc.ObjectMeta.Namespace).Create(context.TODO(), updateDeployment, metav1.CreateOptions{})
		if err != nil {
			log.Println(err)
			return err
		}
		log.Printf("\ndeployment %s created .....\n", deploymentName)
	}

	// If the Deployment is not controlled by this Scc resource, we should log
	// a warning to the event recorder and return error msg.
	if Scc.Spec.DeletionPolicy == "WipeOut" && !metav1.IsControlledBy(deployment, Scc) {
		msg := fmt.Sprintf(MessageResourceExists, deploymentName)
		c.recorder.Event(Scc, corev1.EventTypeWarning, ErrResourceExists, msg)
		return fmt.Errorf("%s", msg)
	}

	// If this number of the replicas on the Scc resource is specified, and the
	// number does not equal the current desired replicas on the Deployment, we
	// should update the Deployment resource.
	if isReplicasChanged(*Scc.Spec.DeploymentConfig.Replicas, *deployment.Spec.Replicas) {
		logger.V(4).Info("Update deployment resource", "currentReplicas", *Scc.Spec.DeploymentConfig.Replicas, "desiredReplicas", *deployment.Spec.Replicas)
		deployment, err = c.kubeclientset.AppsV1().Deployments(Scc.ObjectMeta.Namespace).Update(context.TODO(), updateDeployment, metav1.UpdateOptions{})
		if err != nil {
			fmt.Println(err)
		}
	}

	// If Deployment Name Change ------------------------------------------------
	if isDeploymentNameChanged(deploymentName, deployment.ObjectMeta.Name) {
		logger.V(4).Info("Update deployment resource", "currentName", deploymentName, "desiredName", deployment.ObjectMeta.Name)
		deployment, err = c.kubeclientset.AppsV1().Deployments(Scc.ObjectMeta.Namespace).Update(context.TODO(), updateDeployment, metav1.UpdateOptions{})
		if err != nil {
			fmt.Println(err)
		}
	}

	// If Deployment Image Change ------------------------------------------------
	if isDeploymentImageChanged(Scc.Spec.DeploymentConfig.Image, deployment.Spec.Template.Spec.Containers[0].Image) {
		logger.V(4).Info("Update deployment resource", "currentImage", Scc.Spec.DeploymentConfig.Image, "desiredImage", deployment.Spec.Template.Spec.Containers[0].Image)
		deployment, err = c.kubeclientset.AppsV1().Deployments(Scc.ObjectMeta.Namespace).Update(context.TODO(), updateDeployment, metav1.UpdateOptions{})
		if err != nil {
			fmt.Println(err)
		}
	}

	// Service Get-----------------------------------------------------------------------------
	Scc, err = c.sccsLister.Sccs(namespace).Get(name)
	if err != nil {
		// The Scc resource may no longer exist, in which case we stop
		// processing.
		if errors.IsNotFound(err) {
			utilruntime.HandleError(fmt.Errorf("Scc '%s' in work queue no longer exists", key))
			return nil
		}
		return err
	}

	// Service Name
	serviceName := generateServiceName(Scc.Name, Scc.Spec.ServiceConfig.Name, resourceCreationTimestamp)

	// Get the service port
	servicePort := Scc.Spec.ServiceConfig.Port
	if servicePort == 0 {
		utilruntime.HandleError(fmt.Errorf("Service Port is not provided by user"))
		return nil
	}

	// If TargetPort is not defined by User, set the TargetPort as same as Port
	serviceTargetPort := Scc.Spec.ServiceConfig.TargetPort
	if Scc.Spec.ServiceConfig.TargetPort == 0 {
		serviceTargetPort = servicePort
	}

	// If deletion Policy is WipeOut, then set the owner Reference
	updateService := newService(Scc, serviceName, serviceTargetPort)
	if Scc.Spec.DeletionPolicy == "WipeOut" {
		updateService.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
			*metav1.NewControllerRef(Scc, samplev1alpha1.SchemeGroupVersion.WithKind("Scc")),
		}
	}

	// Get the service with the name specified in Scc.spec
	service, err := c.kubeclientset.CoreV1().Services(Scc.ObjectMeta.Namespace).Get(context.TODO(), serviceName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		// Create the service
		service, err = c.kubeclientset.CoreV1().Services(Scc.ObjectMeta.Namespace).Create(context.TODO(), updateService, metav1.CreateOptions{})
		if err != nil {
			log.Println(err)
			return err
		}
		log.Printf("\nservice %s created .....\n", serviceName)
	}

	if Scc.Spec.DeletionPolicy == "WipeOut" && !metav1.IsControlledBy(service, Scc) {
		msg := fmt.Sprintf(MessageResourceExists, serviceName)
		c.recorder.Event(Scc, corev1.EventTypeWarning, ErrResourceExists, msg)
		return fmt.Errorf("%s", msg)
	}

	// If Service Name Change, update the service
	if isServiceNameChanged(serviceName, service.ObjectMeta.Name) {
		logger.V(4).Info("Update Service resource", "currentName", serviceName, "desiredName", service.ObjectMeta.Name)
		service, err = c.kubeclientset.CoreV1().Services(Scc.ObjectMeta.Namespace).Update(context.TODO(), updateService, metav1.UpdateOptions{})
		if err != nil {
			fmt.Println(err)
		}
	}

	// If Service Port Change, update the service
	if isServicePortChanged(servicePort, service.Spec.Ports[0].Port) {
		logger.V(4).Info("Update Service resource", "currentName", servicePort, "desiredName", service.Spec.Ports[0].Port)
		service, err = c.kubeclientset.CoreV1().Services(Scc.ObjectMeta.Namespace).Update(context.TODO(), updateService, metav1.UpdateOptions{})
		if err != nil {
			fmt.Println(err)
		}
	}

	err = c.updateScc(Scc, updateDeployment)
	if err != nil {
		return err
	}

	return nil
}

func (c *Controller) updateScc(Scc *samplev1alpha1.Scc, deployment *appsv1.Deployment) error {
	// NEVER modify objects from the store. It's a read-only, local cache.
	// You can use DeepCopy() to make a deep copy of original object and modify this copy
	// Or create a copy manually for better performance
	SccCopy := Scc.DeepCopy()
	SccCopy.Status.AvailableReplicas = deployment.Status.AvailableReplicas
	// If the CustomResourceSubresources feature gate is not enabled,
	// we must use Update instead of UpdateStatus to update the Status block of the Scc resource.
	// UpdateStatus will not allow changes to the Spec of the resource,
	// which is ideal for ensuring nothing other than resource status has been updated.
	_, err := c.sampleclientset.SamplecontrollerV1alpha1().Sccs(Scc.ObjectMeta.Namespace).Update(context.TODO(), SccCopy, metav1.UpdateOptions{})
	return err
}

// enqueueScc takes a Scc resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than Scc.
func (c *Controller) enqueueScc(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.workqueue.Add(key)
}

// handleObject will take any resource implementing metav1.Object and attempt
// to find the Scc resource that 'owns' it. It does this by looking at the
// objects metadata.ownerReferences field for an appropriate OwnerReference.
// It then enqueues that Scc resource to be processed. If the object does not
// have an appropriate OwnerReference, it will simply be skipped.
func (c *Controller) handleObject(obj interface{}) {
	var object metav1.Object
	var ok bool
	logger := klog.FromContext(context.Background())
	if object, ok = obj.(metav1.Object); !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object, invalid type"))
			return
		}
		object, ok = tombstone.Obj.(metav1.Object)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object tombstone, invalid type"))
			return
		}
		logger.V(4).Info("Recovered deleted object", "resourceName", object.GetName())
	}
	logger.V(4).Info("Processing object", "object", klog.KObj(object))
	if ownerRef := metav1.GetControllerOf(object); ownerRef != nil {
		// If this object is not owned by a Scc, we should not do anything more
		// with it.
		if ownerRef.Kind != "Scc" {
			return
		}

		Scc, err := c.sccsLister.Sccs(object.GetNamespace()).Get(ownerRef.Name)
		if err != nil {
			logger.V(4).Info("Ignore orphaned object", "object", klog.KObj(object), "Scc", ownerRef.Name)
			return
		}
		c.enqueueScc(Scc)
		return
	}
}

// newDeployment creates a new Deployment for an Scc resource. It also sets
// the appropriate OwnerReferences on the resource so handleObject can discover
// the Scc resource that 'owns' it.
func newDeployment(Scc *samplev1alpha1.Scc, deploymentName string) *appsv1.Deployment {

	deployment := &appsv1.Deployment{}
	labels := map[string]string{
		"app": "nginx",
	}
	deployment.Labels = labels
	deployment.TypeMeta.Kind = "Deployment"

	deployment.ObjectMeta.Name = deploymentName
	deployment.ObjectMeta.Namespace = Scc.ObjectMeta.Namespace

	deployment.Spec.Replicas = Scc.Spec.DeploymentConfig.Replicas
	deployment.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: labels,
	}
	deployment.Spec.Template.ObjectMeta = metav1.ObjectMeta{
		Labels: labels,
	}
	deployment.Spec.Template.Spec = corev1.PodSpec{
		Containers: []corev1.Container{
			{
				Name:  "nginx",
				Image: Scc.Spec.DeploymentConfig.Image,
				Ports: []corev1.ContainerPort{
					{
						ContainerPort: Scc.Spec.ServiceConfig.Port,
					},
				},
			},
		},
	}
	return deployment
}

func newService(Scc *samplev1alpha1.Scc, serviceName string, serviceTargetPort int32) *corev1.Service {

	labels := map[string]string{
		"app": "nginx",
	}
	service := &corev1.Service{}
	service.Labels = labels

	service.TypeMeta = metav1.TypeMeta{
		Kind: "Service",
	}

	service.ObjectMeta = metav1.ObjectMeta{
		Name:      serviceName,
		Namespace: Scc.ObjectMeta.Namespace,
	}

	service.Spec = corev1.ServiceSpec{
		Type:     Scc.Spec.ServiceConfig.Type,
		Selector: labels,
	}
	service.Spec.Ports = []corev1.ServicePort{
		corev1.ServicePort{
			Port:       Scc.Spec.ServiceConfig.Port,
			TargetPort: intstr.FromInt32(serviceTargetPort),
			NodePort:   Scc.Spec.ServiceConfig.NodePort,
		},
	}
	return service
}
