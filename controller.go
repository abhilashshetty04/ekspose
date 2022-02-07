package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	ntw "k8s.io/api/networking/v1"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	appsinformers "k8s.io/client-go/informers/apps/v1"
	"k8s.io/client-go/kubernetes"
	appslisters "k8s.io/client-go/listers/apps/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type controller struct {
	clientset      kubernetes.Interface
	depLister      appslisters.DeploymentLister
	depCacheSynced cache.InformerSynced
	queue          workqueue.RateLimitingInterface
}

func newController(clientset kubernetes.Interface, depInformer appsinformers.DeploymentInformer) *controller {
	c := &controller{
		clientset:      clientset,
		depLister:      depInformer.Lister(),
		depCacheSynced: depInformer.Informer().HasSynced,
		queue:          workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "ekspose"),
	}
	depInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.handleAdd,
			DeleteFunc: c.handleDel,
		},
	)
	return c
}

func (c *controller) handleAdd(obj interface{}) {
	fmt.Println("Add was called")
	c.queue.Add(obj)
}

func (c *controller) handleDel(obj interface{}) {
	fmt.Println("Del was called")
	c.queue.Add(obj)
}

func (c *controller) run(ch <-chan struct{}) {
	fmt.Println("starting controller")
	if !cache.WaitForCacheSync(ch, c.depCacheSynced) {
		fmt.Print("error waiting for cache to be synced\n")
	}
	go wait.Until(c.worker, 1*time.Second, ch)
	<-ch

}

func (c *controller) worker() {
	for c.processItems() {
	}
}

func (c *controller) processItems() bool {
	item, shutdown := c.queue.Get()
	if shutdown {
		return false
	}
	defer c.queue.Forget(item)
	key, err := cache.MetaNamespaceKeyFunc(item)
	if err != nil {
		fmt.Printf("Getting key from cache failed %s\n", err.Error())
	}
	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		fmt.Printf("Getting name and ns from cache failed %s\n", err.Error())
		return false
	}

	//Check if object was Added or deleted
	ctx := context.Background()
	_, err = c.clientset.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
	if apierror.IsNotFound(err) {
		fmt.Printf("Handle del event for %s in %s\n", name, ns)
		err := c.clientset.CoreV1().Services(ns).Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil {
			fmt.Printf("Couldnt delete service %s, error is %s\n", name, err)
			return false
		}
		fmt.Printf("deleted service %s from %s\n", name, ns)
		err = c.clientset.NetworkingV1().Ingresses(ns).Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil {
			fmt.Printf("Couldnt delete ingress %s, error is %s\n", name, err)
			return false
		}
		fmt.Printf("deleted ingress %s from %s\n", name, ns)
		return true
	}
	fmt.Printf("Handle Add event for %s in %s\n", name, ns)
	err = c.syncDeployment(ns, name)
	if err != nil {
		fmt.Printf("Syncing deployment failed %s\n", err.Error())
		return false
	}
	return true
}

func (c *controller) syncDeployment(ns, name string) error {
	//create service
	ctx := context.Background()

	dep, err := c.depLister.Deployments(ns).Get(name)
	if err != nil {
		fmt.Printf("Getting deployment obj failed %s\n", err.Error())
		return err
	}

	port := "80"
	if err != nil {
		fmt.Printf("Getting deployment obj failed %s\n", err.Error())
		return err
	}
	for key, val := range dep.ObjectMeta.Labels {
		if key == "port" {
			port = val
		}
	}
	portInt, _ := strconv.Atoi(port)
	/*if dep.Name == "nginxd" {
		fmt.Println(dep.Name)
		var ingReq string
		for key, val := range dep.ObjectMeta.Labels {
			if key == "ingReq" {
				ingReq = val
			}
		}
		fmt.Printf("Ingress resource for the deployment object is: %s\n", ingReq)
	}*/

	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dep.Name,
			Namespace: dep.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: dep.Spec.Template.Labels,
			Ports: []corev1.ServicePort{
				{
					Name: "http",
					Port: int32(portInt),
				},
			},
		},
	}

	service, err := c.clientset.CoreV1().Services(ns).Create(ctx, &svc, metav1.CreateOptions{})
	if apierror.IsAlreadyExists(err) {
		fmt.Printf("Service %s already exists\n", dep.Name)
		return nil
	}
	if err != nil {
		fmt.Printf("Service creation failed: %s\n", err.Error())
		return err
	}
	fmt.Printf("Created service %s in %s ns\n", service.Name, service.Namespace)

	ingReq := "notNeeded"
	for key, val := range dep.ObjectMeta.Labels {
		if key == "ingReq" {
			ingReq = val
		}
	}
	fmt.Printf("Ingress resource for the deployment object is: %s\n", ingReq)
	if ingReq == "needed" {
		//create ingress
		ingerror := createIngress(ctx, c.clientset, service)
		if ingerror != nil {
			return ingerror
		}
		return nil
	}
	return nil
}

func createIngress(ctx context.Context, client kubernetes.Interface, svc *corev1.Service) error {
	path := "/"
	port := "80"
	dep, err := client.AppsV1().Deployments(svc.Namespace).Get(ctx, svc.Name, metav1.GetOptions{})
	if err != nil {
		fmt.Printf("Getting deployment obj failed %s\n", err.Error())
		return err
	}
	for key, val := range dep.ObjectMeta.Labels {
		/*if key == "path" {
			path = val
		}*/
		if key == "port" {
			port = val
		}
	}
	if port == "8080" {
		path = "/api/v1/books"
	}
	portInt, _ := strconv.Atoi(port)
	pathtype := ntw.PathTypeExact
	ingress := ntw.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svc.Name,
			Namespace: svc.Namespace,
		},
		Spec: ntw.IngressSpec{
			Rules: []ntw.IngressRule{
				ntw.IngressRule{
					Host: svc.Name + ".abhilash.com",
					IngressRuleValue: ntw.IngressRuleValue{
						HTTP: &ntw.HTTPIngressRuleValue{
							Paths: []ntw.HTTPIngressPath{
								ntw.HTTPIngressPath{
									Path:     path,
									PathType: &pathtype,
									Backend: ntw.IngressBackend{
										Service: &ntw.IngressServiceBackend{
											Name: svc.Name,
											Port: ntw.ServiceBackendPort{
												Number: int32(portInt),
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	ing, err := client.NetworkingV1().Ingresses(svc.Namespace).Create(ctx, &ingress, metav1.CreateOptions{})
	if apierror.IsAlreadyExists(err) {
		fmt.Printf("Ingress %s already exists\n", ing.Name)
		return nil
	}
	if err != nil {
		fmt.Printf("Ingress creation failed: %s\n", err.Error())
		return err
	}
	fmt.Printf("Created ingress %s in %s ns\n", ing.Name, ing.Namespace)
	return nil
}
