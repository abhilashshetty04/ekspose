package main

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	ntw "k8s.io/api/networking/v1"
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

	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dep.Name,
			Namespace: ns,
		},
		Spec: corev1.ServiceSpec{
			Selector: dep.Spec.Template.Labels,
			Ports: []corev1.ServicePort{
				{
					Name: "http",
					Port: 80,
				},
			},
		},
	}
	service, err := c.clientset.CoreV1().Services(ns).Create(ctx, &svc, metav1.CreateOptions{})
	if err != nil {
		fmt.Printf("Service creation failed %s\n", err.Error())
		return err

	}
	//create ingress
	ingerror := createIngress(ctx, c.clientset, service)
	if ingerror != nil {
		return ingerror
	}
	return nil
}

func createIngress(ctx context.Context, client kubernetes.Interface, svc *corev1.Service) error {
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
									Path:     "/",
									PathType: &pathtype,
									Backend: ntw.IngressBackend{
										Service: &ntw.IngressServiceBackend{
											Name: svc.Name,
											Port: ntw.ServiceBackendPort{
												Number: 80,
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
	_, err := client.NetworkingV1().Ingresses(svc.Namespace).Create(ctx, &ingress, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	return nil
}

/*func depLabel(dep appsv1.Deployment) map[string]string {
	return dep.Spec.Template.Labels
}*/

func (c *controller) handleAdd(obj interface{}) {
	fmt.Println("Add was called")
	c.queue.Add(obj)
}

func (c *controller) handleDel(obj interface{}) {
	fmt.Println("Del was called")
	c.queue.Add(obj)
}
