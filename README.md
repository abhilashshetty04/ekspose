Initial design reference:
https://www.youtube.com/watch?v=lzoWSfvE2yA&list=PLh4KH3LtJvRQ43JAwwjvTnsVOMp0WKnJO

Ekspose - A custom controller that makes a deployment accessible from outside. Reconciles any unforseen object deletion as well. Thereby, making sure application is always accessible.

This project implements 3 controllers sharing an informer. Theres a controller watching Deployment, Service and ingress resource events across namespace. Controller pod runs in ekspose ns

Each controller use specific verbs for the objects they are are resposible for. Hence service account mounted on pod should have necessary RBAC permissions:

Deployment obj: List, watch
Service obj: List, Watch, Add, Del
Ingress obj: List, Watch, Add, Del

We would need to create service account which can be used in deloyment manifest of the controller. Along with cluster role as we must watch accross ns and cluster role binding object to bind role with service account.
 
Please find all these manifest along with controller deployment manifest in 

https://github.com/abhilashshetty04/ekspose/tree/main/manifests

controller image when drafting this readme was abhilashshetty04/ekspose:0.1.4. Please visit following url and use image accordingly in controller deploy manifest for latest fixes and features.

https://hub.docker.com/repository/docker/abhilashshetty04/ekspose

Controller depends on deployment labels when creating ports, path for service and ingress. Following are examples of labels in deployment.

1. Every deployment requires service for its pods to be accisible atleast inside cluster. However, its not same with ingress resource. All application is not meant to be accessed from outside. Hence following label is required if ingress is required.

ingReq: needed

Example :
https://github.com/abhilashshetty04/gorest/blob/main/manifests/restapi/deployment-rest.yaml#L7

Ingress resource created by the controller is hostbased. Currenlt host/fqdn has svc.Name + ".abhilash.com" patterm. implementation is here. Make modification if necessary.
https://github.com/abhilashshetty04/ekspose/blob/main/controller.go#L219

2. Controller would create svc and ingress using 80 as port number. However this can be modified by using lablel in deployment obj. Visit below url.

https://github.com/abhilashshetty04/gorest/blob/main/manifests/restapi/deployment-rest.yaml#L9

Scenarios supported:

1. When controller pod starts to run after initial cache sync it will get Updates for all existing deployments. When processing an item if service is already present controller will log that service is already present and item will be discareded from the queue. If there is a deployment for which service is not present it will created the service. Ing creation depends upon if ingReq is set to "needed" in deploy obj.

2. When a deployment (irrespective of ns) is created while controller is running. Service will be created. By default port number is set to 80 unless specified other TCP port using deployment labels.  Ing creation depends upon if ingReq is set to "needed" in deploy obj. If ingress is not required ingReq label is not required,

3. When a deployment is deleted, controller will remove the associated service and Ingress object from the same name space.

4. Ingress and Service controller subscribes to only Delete action. When Service controller gets a deleted service obj from the queue. It checks if deployment still exists for that service in same ns. If it does, It will recreate the service or else it will not recreate it.

5. When Ingress controller gets a deleted ing obj from the queue. It checks if service still exists for that ingress in same ns. If it does, It will recreate the ingress or else it will not recreate it.

6. Since deletion of ingress and svc wf after deployment delete is called will add respective obj to ing/service controlller's queue. code does check if deployement obj exists or not and logs accordingly. Meaning, service/ingress controller can differentiate if svc/ing deletion was from user or from controller.

Steps to deploy controller:

Prerequisite: Make sure there is an ingress controller (eg. Nginx) installed. 

Refer official nginx docs: https://docs.nginx.com/nginx-ingress-controller/installation/

git clone https://github.com/abhilashshetty04/ekspose

cd manifests

1. create ekspose namespace

kubectl create namespace ekspose

2. Create service account for controller pod:

kubectl apply -f sa.yaml
https://github.com/abhilashshetty04/ekspose/blob/main/manifests/sa.yaml

3. Create cluster role which would allow access on deployment, svc and ing to required verbs across namespace.

kubectl apply -f clrole.yaml
https://github.com/abhilashshetty04/ekspose/blob/main/manifests/clrole.yaml

4. Create cluster role binding of role with service account which would assign required permissions to pod service account.

kubectl apply -f clrolebind.yaml
https://github.com/abhilashshetty04/ekspose/blob/main/manifests/clrolebind.yaml

5. Create the controller deployment object. Note service account is specified in the Spec. Please use latest image before applying.

kubectl apply -f https://github.com/abhilashshetty04/ekspose/blob/main/manifests/deployment.yaml

Testing the controller:

There is a sample deploy file:

https://github.com/abhilashshetty04/ekspose/blob/main/manifests/deploy.yaml

If you want to test the application access. It is required that connection comes to the ingress controllers. In public cloud Loadbalancerwill be there which would forward the traffic to controllers. In on prem kubeadm cluster loadbalancer needs to be installed.

I used HAProxy for reverse proxy with following config. Note that http backend server are kube worker nodes where ingress controllers run. So When any  connection comes to HAProxy on port 80 it is forwarded to worker nodes whose port 80 is mapped and forwarded to ing controllers.

In HAProxy config: vim /etc/haproxy/haproxy.cfg
frontend http_front
        bind *:80
        stats uri /haproxy?stats
        default_backend http_back

backend http_back
        balance roundrobin
        server kube-node-2 192.168.122.75:80
        server kube-node-3 192.168.122.100:80

K8s Cluster:

abhilash@kube-node-1:~$ kubectl get nodes -o wide
NAME          STATUS   ROLES                  AGE   VERSION   INTERNAL-IP       EXTERNAL-IP   OS-IMAGE             KERNEL-VERSION     CONTAINER-RUNTIME
kube-node-1   Ready    control-plane,master   54d   v1.23.0   192.168.122.50    <none>        Ubuntu 20.04.3 LTS   5.4.0-97-generic   docker://20.10.12
kube-node-2   Ready    <none>                 54d   v1.23.0   192.168.122.75    <none>        Ubuntu 20.04.3 LTS   5.4.0-97-generic   docker://20.10.12
kube-node-3   Ready    <none>                 54d   v1.23.0   192.168.122.100   <none>        Ubuntu 20.04.3 LTS   5.4.0-97-generic   docker://20.10.12
  
abhilash@kube-node-1:~$ kubectl get pods -n nginx-ingress -o wide
NAME                  READY   STATUS    RESTARTS        AGE   IP             NODE          NOMINATED NODE   READINESS GATES
nginx-ingress-k7896   1/1     Running   17 (6h7m ago)   17d   10.244.2.218   kube-node-3   <none>           <none>
nginx-ingress-xwmnn   1/1     Running   17 (6h7m ago)   17d   10.244.1.235   kube-node-2   <none>           <none>












