package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	coreInformer "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	coreLister "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type svcCtrller struct {
	clientset       kubernetes.Interface
	svcLister       coreLister.ServiceLister
	svcCachedSynced cache.InformerSynced
	queue           workqueue.RateLimitingInterface
}

func newSvcController(clientset kubernetes.Interface, svcInformer coreInformer.ServiceInformer) *svcCtrller {
	svcc := &svcCtrller{
		clientset:       clientset,
		svcLister:       svcInformer.Lister(),
		svcCachedSynced: svcInformer.Informer().HasSynced,
		queue:           workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "svc"),
	}
	svcInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			DeleteFunc: svcc.handleDel,
		},
	)
	return svcc
}

func (svcc *svcCtrller) handleDel(obj interface{}) {
	fmt.Println("Adding deleted service obj to the workqueue")
	svcc.queue.Add(obj)
}

func (svcc *svcCtrller) run(ch <-chan struct{}) {
	fmt.Println("starting controller")
	if !cache.WaitForCacheSync(ch, svcc.svcCachedSynced) {
		fmt.Print("error waiting for cache to be synced\n")
	}
	go wait.Until(svcc.worker, 1*time.Second, ch)
	<-ch
}

func (svcc *svcCtrller) worker() {
	for svcc.processItems() {

	}
}

func (svcc *svcCtrller) processItems() bool {
	item, shutdown := svcc.queue.Get()
	if shutdown {
		return false
	}
	defer svcc.queue.Forget(item)
	key, err := cache.MetaNamespaceKeyFunc(item)
	if err != nil {
		fmt.Printf("Getting key from cache failed %s\n", err.Error())
		return false
	}
	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		fmt.Printf("Getting ns and name from key failed %s\n", err.Error())
		return false
	}
	ctx := context.Background()
	dep, err := svcc.clientset.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
	if apierror.IsNotFound(err) {
		fmt.Printf("Deployment not found for the deleted service : %s. Hence discarding obj reconcilation\n", name)
		return true
	}
	fmt.Printf("Deployment found for the deleted service object : %s. Hence continuing object reconcilation\n", name)

	port := "80"
	for key, val := range dep.ObjectMeta.Labels {
		if key == "port" {
			port = val
		}
	}
	portInt, _ := strconv.Atoi(port)

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
	service, err := svcc.clientset.CoreV1().Services(ns).Create(ctx, &svc, metav1.CreateOptions{})
	if err != nil {
		fmt.Printf("Service creation failed %s\n", err.Error())
		return false

	}
	fmt.Printf("Successfully recreated service %s in %s ns\n", service.Name, service.Namespace)
	return true
}
