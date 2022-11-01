package main

import (
	"context"
	"fmt"
	"time"

	appsV1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	appsInformers "k8s.io/client-go/informers/apps/v1"
	"k8s.io/client-go/kubernetes"
	applisters "k8s.io/client-go/listers/apps/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type controller struct {
	clientset      kubernetes.Interface
	depLister      applisters.DeploymentLister
	depCacheSynced cache.InformerSynced
	queue          workqueue.RateLimitingInterface
}

func newController(clientset kubernetes.Interface, depInformer appsInformers.DeploymentInformer) *controller {
	c := &controller{
		clientset:      clientset,
		depLister:      depInformer.Lister(),
		depCacheSynced: depInformer.Informer().HasSynced,
		queue:          workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "ekspose"),
	}

	depInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.handleAdd,
			DeleteFunc: c.handleDelete,
		},
	)
	return c
}

func (c *controller) run(ch chan struct{}) {
	fmt.Println("Starting the Controller")
	if !cache.WaitForCacheSync(ch, c.depCacheSynced) {
		fmt.Println("Error waiting for Cache sync ")
	}
	go wait.Until(c.worker, 1*time.Minute, ch)
	<-ch
}

func (c *controller) worker() {
	for c.processItem() {

	}

}

func (c *controller) processItem() bool {
	item, shutdown := c.queue.Get()
	if shutdown {
		fmt.Println("Error Getting item from queue")
		return false
	}
	defer c.queue.Forget(item)
	key, err := cache.MetaNamespaceKeyFunc(item)
	if err != nil {
		fmt.Printf("Error getting Key - %v", err)
		return false
	}
	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		fmt.Printf("Error Splitting Key - %v", err)
		return false
	}
	err = c.syncDeployment(ns, name)
	if err != nil {
		//retry logic instead of failing on the first attempt failure
		fmt.Printf("Error Sync deployment  - %v", err)
		return false
	}
	return true
}

func (c *controller) syncDeployment(ns string, name string) error {
	//create servcice
	ctx := context.Background()
	dep, err := c.depLister.Deployments(ns).Get(name)
	if err != nil {
		fmt.Printf("Error Getting the deployment  - %v", err)
		return err
	}
	svc := coreV1.Service{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      dep.Name,
			Namespace: ns,
		},
		Spec: coreV1.ServiceSpec{
			Selector: c.depLabels(*dep),
			Ports: []coreV1.ServicePort{
				{
					Name: "http",
					Port: 80,
				},
			},
		},
	}
	_, err = c.clientset.CoreV1().Services(ns).Create(ctx, &svc, metaV1.CreateOptions{})
	if err != nil {
		fmt.Printf(" creating service  - %v \n", err)
		//return err
	}
	//create ingress

	return nil
}

func (c *controller) depLabels(dep appsV1.Deployment) map[string]string {
	return dep.Spec.Template.Labels
}

func (c *controller) handleAdd(obj interface{}) {
	fmt.Println("Add was called")
	c.queue.Add(obj)
}

func (c *controller) handleDelete(obj interface{}) {
	fmt.Println("Delete was called")
	c.queue.Add(obj)
}
