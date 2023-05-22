# ytt-controller
A YTT Carvel controller. It can fetch YTT files from:
1. Flux Sources (GitRepository/OCIRepository/Bucket)
2. ConfigMap/Secret

process those files programmatically invoking Carvel `ytt` and store the output in its Status section.
[Sveltos addon-manager](https://github.com/projectsveltos/addon-manager) can then be used to deploy the output of the ytt-controller in all selected managed clusters.

## Install

```bash
kubectl apply -f https://raw.githubusercontent.com/gianlucam76/ytt-controller/main/manifest/manifest.yaml
```

or if you want a specific version

```bash
kubectl apply -f https://raw.githubusercontent.com/gianlucam76/ytt-controller/<tag>/manifest/manifest.yaml
```

## Using Flux GitRepository

For instance, this Github repository https://github.com/gianlucam76/ytt-examples contains ytt files. 
You can use Flux to sync from it and then simply post this [YttSource](https://github.com/gianlucam76/ytt-controller/blob/main/api/v1alpha1/yttsource_types.go) CRD instance.
The ytt-controller will detect when Flux has synced the repo (and anytime there is a change), will programatically invoke ytt and store the outcome in its Status.Resources field.

```yaml
apiVersion: extension.projectsveltos.io/v1alpha1
kind: YttSource
metadata:
  name: yttsource-flux
spec:
  namespace: flux-system
  name: flux-system
  kind: GitRepository
  path: ./deployment/
```

```yaml
apiVersion: extension.projectsveltos.io/v1alpha1
kind: YttSource
metadata:
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"extension.projectsveltos.io/v1alpha1","kind":"YttSource","metadata":{"annotations":{},"name":"yttsource-flux","namespace":"default"},"spec":{"kind":"GitRepository","name":"flux-system","namespace":"flux-system","path":"./deployment/"}}
  creationTimestamp: "2023-05-22T06:20:01Z"
  generation: 1
  name: yttsource-flux
  namespace: default
  resourceVersion: "91726"
  uid: c7ae3485-37a9-447a-9fa4-7d21a129ccf6
spec:
  kind: GitRepository
  name: flux-system
  namespace: flux-system
  path: ./deployment/
status:
  resources: |
    apiVersion: v1
    kind: Service
    metadata:
      name: sample-app
      labels:
        environment: staging
    spec:
      selector:
        app: sample-app
      ports:
      - protocol: TCP
        port: 80
        targetPort: 8080
    ---
    apiVersion: apps/v1
    kind: Deployment
    metadata:
      name: sample-app
      labels:
        environment: staging
    spec:
      replicas: 1
      selector:
        matchLabels:
          environment: staging
      template:
        metadata:
          labels:
            environment: staging
        spec:
          containers:
          - name: sample-app
            image: nginx:latest
            imagePullPolicy: IfNotPresent
            ports:
            - containerPort: 8080
    ---
    apiVersion: v1
    kind: Secret
    metadata:
      name: application-settings
    stringData:
      app_mode: staging
      certificates: /etc/ssl/staging
      db_user: staging-user
      db_password: staging-password
```

## Using ConfigMap/Secret

YttSource can also reference ConfigMap/Secret. For instance, we can create a ConfigMap whose BinaryData section contains ytt files.

```bash
tar -czf ytt.tar.gz -C ~mgianluc/go/src/github.com/gianlucam76/ytt-examples/deployment .
kubectl create configmap ytt --from-file=ytt.tar.gz=ytt.tar.gz 
```

Then we can have YttSource reference this ConfigMap instance

```yaml
apiVersion: extension.projectsveltos.io/v1alpha1
kind: YttSource
metadata:
  name: yttsource-sample
spec:
  namespace: default
  name: ytt
  kind: ConfigMap
  path: ./
```

and the controller will programmatically execute ytt and store the outcome in Status.Results.

```yaml
apiVersion: extension.projectsveltos.io/v1alpha1
kind: YttSource
metadata:
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"extension.projectsveltos.io/v1alpha1","kind":"YttSource","metadata":{"annotations":{},"name":"yttsource-sample","namespace":"default"},"spec":{"kind":"ConfigMap","name":"ytt","namespace":"default","path":"./"}}
  creationTimestamp: "2023-05-22T06:27:31Z"
  generation: 1
  name: yttsource-sample
  namespace: default
  resourceVersion: "94517"
  uid: 4b0b4efb-57b4-4ffd-ab32-dc56fee21a09
spec:
  kind: ConfigMap
  name: ytt
  namespace: default
  path: ./
status:
  resources: |
    apiVersion: v1
    kind: Service
    metadata:
      name: sample-app
      labels:
        environment: staging
    spec:
      selector:
        app: sample-app
      ports:
      - protocol: TCP
        port: 80
        targetPort: 8080
    ---
    apiVersion: apps/v1
    kind: Deployment
    metadata:
      name: sample-app
      labels:
        environment: staging
    spec:
      replicas: 1
      selector:
        matchLabels:
          environment: staging
      template:
        metadata:
          labels:
            environment: staging
        spec:
          containers:
          - name: sample-app
            image: nginx:latest
            imagePullPolicy: IfNotPresent
            ports:
            - containerPort: 8080
    ---
    apiVersion: v1
    kind: Secret
    metadata:
      name: application-settings
    stringData:
      app_mode: staging
      certificates: /etc/ssl/staging
      db_user: staging-user
      db_password: staging-password
```

At this point [Sveltos addon-manager](https://github.com/projectsveltos/addon-manager) to use the output of the ytt-controller and deploy those resources in all selected managed clusters.
