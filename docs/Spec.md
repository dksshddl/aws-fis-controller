1. create IAM role/policy properly based on requirements of FIS on EKS.

2. create CRD automatically if addon is installed.
- customer can define their fis experiemts using this CRD.

3. manage lifecyle of this CRD using kubernetes jobs
- jobs will be created if CRD is submiited.
- each CRD shows it's status, creation time and etc..

4. implement kubectl plugins to use fis controller easily
```
# like kubectl rollback show ~
kubectl fis [status|create|show|delete|describe] experiments ~
```


skills
- kubebuilder
- aws-go-sdk
- kubeernetes

requirements
- we need to specify proper IAM role policy to use this controller
- customer may use IRSA/Pod Identity for authorization.


# FIS controlelr reference

## 1. how it works
Step 1. FIS service create FIS pod in the cluster directly
Step 2. After creating FIS pod, it creates ephemeral container into Target Pod
Step 3. ephemeral container inject any fault which confiure in experiments.
Step 4. FIS pod monitor ephemeral container and also FIS monitor FIS pod.

## Service Account
[] AWSFIS aws:eks:pod 작업 사용 - Kubernetes 서비스 계정 구성 - https://docs.aws.amazon.com/ko_kr/fis/latest/userguide/eks-pod-actions.html#configure-service-account

### RBAC
```yaml
kind: ServiceAccount
apiVersion: v1
metadata:
  namespace: default
  name: myserviceaccount
---
kind: Role
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  namespace: default
  name: role-experiments
rules:
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: [ "get", "create", "patch", "delete"]
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["create", "list", "get", "delete", "deletecollection"]
- apiGroups: [""]
  resources: ["pods/ephemeralcontainers"]
  verbs: ["update"]
- apiGroups: [""]
  resources: ["pods/exec"]
  verbs: ["create"]
- apiGroups: ["apps"]
  resources: ["deployments"]
  verbs: ["get"]

---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: bind-role-experiments
  namespace: default
subjects:
- kind: ServiceAccount
  name: myserviceaccount
  namespace: default
- apiGroup: rbac.authorization.k8s.io
  kind: User
  name: fis-experiment
roleRef:
  kind: Role
  name: role-experiments
  apiGroup: rbac.authorization.k8s.io
```

### Pod Identity
```sh
aws eks create-access-entry \
                 --principal-arn arn:aws:iam::123456789012:role/fis-experiment-role \
                 --username fis-experiment \
                 --cluster-name my-cluster
```

## pod container 
public ecr - https://gallery.ecr.aws/aws-fis/aws-fis-pod
private ecr - refer to https://docs.aws.amazon.com/fis/latest/userguide/eks-pod-actions.html#eks-pod-container-images


