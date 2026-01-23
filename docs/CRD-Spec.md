# ExperimentTemplate CRD Specification

## Overview

ExperimentTemplate CRD는 AWS FIS (Fault Injection Service) experiment template을 Kubernetes native하게 관리하기 위한 Custom Resource입니다.

## API Version

- **Group**: `fis.fis.dksshddl.dev`
- **Version**: `v1alpha1`
- **Kind**: `ExperimentTemplate`

## Short Names

- `fisexp`
- `fistemplate`

## Spec Fields

### Required Fields

#### targets ([]TargetSpec)

실험 대상 pod들을 정의합니다. 최소 1개 이상 필요합니다.

```yaml
targets:
- name: nginx-pods              # Required: target의 고유 식별자
  namespace: default            # Optional: 대상 namespace (기본값: default)
  labelSelector:                # Required: pod 선택을 위한 label
    app: nginx
  selectionMode: ALL            # Optional: ALL, COUNT, PERCENT (기본값: ALL)
  count: 2                      # Optional: COUNT 모드일 때 선택할 pod 수
  percent: 50                   # Optional: PERCENT 모드일 때 선택할 비율
  targetContainerName: nginx    # Optional: 특정 container 지정
```

#### actions ([]ActionSpec)

실행할 chaos action들을 정의합니다. 최소 1개 이상 필요합니다.

```yaml
actions:
- name: cpu-stress              # Required: action의 고유 식별자
  description: "Apply CPU stress" # Optional: action 설명
  type: pod-cpu-stress          # Required: action 타입
  duration: 5m                  # Required: 실행 시간 (5m, 10m, 1h 등)
  target: nginx-pods            # Required: 적용할 target 이름
  parameters:                   # Optional: action별 파라미터
    percent: "80"
    workers: "2"
  startAfter:                   # Optional: 선행 action 이름들
  - previous-action
```

**지원하는 Action Types:**
- `pod-cpu-stress`: CPU stress 주입
- `pod-memory-stress`: Memory stress 주입
- `pod-io-stress`: Disk I/O stress 주입
- `pod-network-latency`: Network latency 주입
- `pod-network-packet-loss`: Network packet loss 주입
- `pod-delete`: Pod 삭제

### Optional Fields

#### description (string)

실험 template에 대한 설명입니다.

#### stopConditions ([]StopCondition)

실험을 중단할 조건들을 정의합니다.

```yaml
stopConditions:
- source: cloudwatch-alarm      # cloudwatch-alarm 또는 none
  value: "arn:aws:cloudwatch:ap-northeast-2:123456789012:alarm:my-alarm"
```

#### experimentOptions (ExperimentOptions)

실험 레벨 옵션들을 정의합니다.

```yaml
experimentOptions:
  accountTargeting: single-account           # single-account 또는 multi-account
  emptyTargetResolutionMode: fail            # fail 또는 skip
```

#### logConfiguration (LogConfiguration)

실험 로그 설정을 정의합니다.

```yaml
logConfiguration:
  logSchemaVersion: 2
  cloudWatchLogsConfiguration:
    logGroupArn: "arn:aws:logs:ap-northeast-2:123456789012:log-group:/aws/fis/experiments:*"
  s3Configuration:
    bucketName: "my-fis-logs-bucket"
    prefix: "fis-logs"
```

#### experimentReportConfiguration (ExperimentReportConfiguration)

실험 리포트 설정을 정의합니다.

```yaml
experimentReportConfiguration:
  preExperimentDuration: 20m
  postExperimentDuration: 20m
  dataSources:
    cloudWatchDashboards:
    - dashboardIdentifier: "arn:aws:cloudwatch::123456789012:dashboard/MyDashboard"
  outputs:
    s3Configuration:
      bucketName: "my-fis-reports-bucket"
      prefix: "fis-reports"
```

#### tags ([]Tag)

AWS 리소스에 적용할 tag들을 정의합니다.

```yaml
tags:
- key: Name
  value: "My Experiment"
- key: Environment
  value: "production"
```

## Status Fields

### templateId (string)

AWS FIS에서 생성된 experiment template ID입니다.

### phase (string)

현재 template의 상태입니다.

- `Pending`: 생성 대기 중
- `Creating`: AWS FIS에 생성 중
- `Ready`: 사용 가능
- `Failed`: 생성 실패
- `Deleting`: 삭제 중

### message (string)

현재 상태에 대한 추가 정보입니다.

### lastSyncTime (metav1.Time)

마지막으로 AWS FIS와 동기화된 시간입니다.

### conditions ([]metav1.Condition)

Kubernetes standard condition들입니다.

## Examples

### Simple CPU Stress

```yaml
apiVersion: fis.fis.dksshddl.dev/v1alpha1
kind: ExperimentTemplate
metadata:
  name: simple-cpu-stress
spec:
  description: "Simple CPU stress test"
  targets:
  - name: app-pods
    labelSelector:
      app: myapp
    selectionMode: ALL
  actions:
  - name: stress
    type: pod-cpu-stress
    duration: 5m
    target: app-pods
    parameters:
      percent: "80"
  stopConditions:
  - source: none
```

### Multi-Target with Sequential Actions

```yaml
apiVersion: fis.fis.dksshddl.dev/v1alpha1
kind: ExperimentTemplate
metadata:
  name: multi-target-test
spec:
  description: "Test multiple targets sequentially"
  targets:
  - name: frontend
    labelSelector:
      tier: frontend
    selectionMode: PERCENT
    percent: 50
  - name: backend
    labelSelector:
      tier: backend
    selectionMode: COUNT
    count: 2
  actions:
  - name: frontend-cpu
    type: pod-cpu-stress
    duration: 5m
    target: frontend
    parameters:
      percent: "70"
  - name: backend-memory
    type: pod-memory-stress
    duration: 5m
    target: backend
    parameters:
      percent: "80"
    startAfter:
    - frontend-cpu
```

### Full-Featured Example

전체 기능을 사용하는 예제는 `config/samples/full-featured-experiment.yaml`을 참고하세요.

## Controller Configuration

Controller는 다음 flag들을 지원합니다:

- `--fis-namespace`: FIS pod가 실행될 namespace (기본값: `default`)
- `--fis-role-arn`: AWS FIS가 사용할 IAM role ARN (TODO: 구현 예정)
- `--cluster-identifier`: EKS cluster identifier (TODO: 구현 예정)

## Notes

- `roleArn`은 spec에서 제거되었으며, controller 레벨에서 관리됩니다.
- `clusterIdentifier`는 controller 설정에서 가져옵니다.
- `kubernetesServiceAccount`는 controller가 자동으로 생성한 `fis-pod-sa`를 사용합니다.
- Duration 형식은 Kubernetes 스타일 (`5m`, `10m`, `1h`)을 사용하며, controller가 AWS FIS 형식 (`PT5M`)으로 변환합니다.
- Filter 기능은 TODO로 표시되어 있으며, 향후 구현 예정입니다.
