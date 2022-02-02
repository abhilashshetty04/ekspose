package main

import (
	"context"
	"fmt"
	"time"

	apierror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	ntwinformer "k8s.io/client-go/informers/networking/v1"
	"k8s.io/client-go/kubernetes"
	ntwlister "k8s.io/client-go/listers/networking/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type ingCtrller struct {
	clientset      kubernetes.Interface
	ingLister      ntwlister.IngressLister
	ingCacheSynced cache.InformerSynced
	queue          workqueue.RateLimitingInterface
}

func newIngController(clientset kubernetes.Interface, ingInformer ntwinformer.IngressInformer) *ingCtrller {
	ingc := &ingCtrller{
		clientset:      clientset,
		ingLister:      ingInformer.Lister(),
		ingCacheSynced: ingInformer.Informer().HasSynced,
		queue:          workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "ingC"),
	}
	ingInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			DeleteFunc: ingc.handleDel,
		},
	)
	return ingc
}

func (ingc *ingCtrller) handleDel(obj interface{}) {
	fmt.Println("Adding deleted ingress obj to the workqueue")
	ingc.queue.Add(obj)
}

func (ingc *ingCtrller) run(ch <-chan struct{}) {
	fmt.Println("starting controller")
	if !cache.WaitForCacheSync(ch, ingc.ingCacheSynced) {
		fmt.Print("error waiting for cache to be synced\n")
	}
	go wait.Until(ingc.worker, 1*time.Second, ch)
	<-ch

}

func (ingc *ingCtrller) worker() {
	for ingc.processItems() {

	}
}

func (ingc *ingCtrller) processItems() bool {
	item, shutdown := ingc.queue.Get()
	if shutdown {
		return false
	}
	defer ingc.queue.Forget(item)
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
	service, err := ingc.clientset.CoreV1().Services(ns).Get(ctx, name, metav1.GetOptions{})
	if apierror.IsNotFound(err) {
		fmt.Printf("Service not found for the deleted ingress : %s. Hence discarding obj reconcilation\n", name)
		return true
	}

	fmt.Printf("Service found for the deleted ingress object : %s. Hence continuing object reconcilation\n", name)
	ingerror := createIngress(ctx, ingc.clientset, service)
	if ingerror != nil {
		fmt.Printf("Encountered error while recreting ingress obj:%s in ns: %s", name, ns)
		return false
	}
	fmt.Printf("Successfully recreted ingress obj:%s in ns: %s", name, ns)
	return true
}
