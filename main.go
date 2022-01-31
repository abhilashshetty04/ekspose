package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	kube_config := flag.String("kubeconfig", "/home/ashetty/.kube/config", "Location to Kubeconfig")
	config, err := clientcmd.BuildConfigFromFlags("", *kube_config)
	if err != nil {
		fmt.Printf("Error %s building config from flag\n", err.Error())
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Printf("Error %s building clientset from config\n", err.Error())
		os.Exit(1)

	}
	ch := make(chan struct{})
	informerfactory := informers.NewSharedInformerFactory(clientset, 30*time.Second)
	depInfomer := informerfactory.Apps().V1().Deployments()
	c := newController(clientset, depInfomer)
	informerfactory.Start(ch)
	c.run(ch)
}
