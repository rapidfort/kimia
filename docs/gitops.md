# GitOps Integration

Integration guides for popular GitOps and CI/CD platforms.

---

## ArgoCD Workflows

Complete ArgoCD Workflow example with Kimia:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Workflow
metadata:
  generateName: kimia-build-
  namespace: argo
spec:
  entrypoint: build-and-deploy
  serviceAccountName: argo-workflow
  securityContext:
    runAsNonRoot: true
    runAsUser: 1000
    fsGroup: 1000
  
  templates:
  - name: build-and-deploy
    steps:
    - - name: build
        template: kimia-build
    - - name: deploy
        template: deploy-image
  
  - name: kimia-build
    inputs:
      artifacts:
      - name: source
        path: /workspace
        git:
          repo: "https://github.com/myorg/myapp.git"
          revision: "{{workflow.parameters.git-revision}}"
    container:
      image: ghcr.io/rapidfort/kimia:latest
      args:
        - "--context=/workspace"
        - "--dockerfile=Dockerfile"
        - "--destination=myregistry.io/myapp:{{workflow.uid}}"
        - "--build-arg=BUILD_DATE={{workflow.creationTimestamp}}"
        - "--build-arg=GIT_COMMIT={{workflow.parameters.git-commit}}"
        - "--label=workflow.id={{workflow.name}}"
        - "--cache=true"
        - "--push-retry=3"
      volumeMounts:
      - name: docker-config
        mountPath: /home/kimia/.docker
      - name: cache
        mountPath: /home/kimia/.local/share/containers
      securityContext:
        runAsUser: 1000
        allowPrivilegeEscalation: true
        capabilities:
          drop: [ALL]
          add: [SETUID, SETGID]
      resources:
        requests:
          memory: "2Gi"
          cpu: "1"
        limits:
          memory: "8Gi"
          cpu: "4"
  
  - name: deploy-image
    container:
      image: bitnami/kubectl:latest
      command: [sh, -c]
      args:
        - |
          kubectl set image deployment/myapp \
            myapp=myregistry.io/myapp:{{workflow.uid}} \
            -n production
  
  volumes:
  - name: docker-config
    secret:
      secretName: registry-credentials
  - name: cache
    emptyDir: {}
```

---

## Flux Integration

### Flux Image Update Automation

```yaml
apiVersion: image.toolkit.fluxcd.io/v1beta1
kind: ImageUpdateAutomation
metadata:
  name: kimia-automation
  namespace: flux-system
spec:
  interval: 5m
  sourceRef:
    kind: GitRepository
    name: myapp-repo
  git:
    checkout:
      ref:
        branch: main
    commit:
      author:
        email: fluxcdbot@example.com
        name: fluxcdbot
      messageTemplate: |
        Automated image update
        Built with Kimia: {{ .AutomationObject }}
  update:
    path: ./config
    strategy: Setters
---
apiVersion: batch/v1
kind: CronJob
metadata:
  name: kimia-builder
  namespace: flux-system
spec:
  schedule: "0 2 * * *"  # Daily at 2 AM
  jobTemplate:
    spec:
      template:
        spec:
          securityContext:
            runAsNonRoot: true
            runAsUser: 1000
            fsGroup: 1000
          containers:
          - name: kimia
            image: ghcr.io/rapidfort/kimia:latest
            args:
              - --context=https://github.com/myorg/myapp.git
              - --git-branch=main
              - --dockerfile=Dockerfile
              - --destination=myregistry.io/myapp:$(date +%Y%m%d-%H%M%S)
              - --destination=myregistry.io/myapp:latest
            securityContext:
              allowPrivilegeEscalation: true
              capabilities:
                drop: [ALL]
                add: [SETUID, SETGID]
            volumeMounts:
            - name: docker-config
              mountPath: /home/kimia/.docker
          volumes:
          - name: docker-config
            secret:
              secretName: registry-credentials
          restartPolicy: OnFailure
```

---

## Tekton Pipeline

Complete Tekton Pipeline with Kimia:

```yaml
apiVersion: tekton.dev/v1beta1
kind: Pipeline
metadata:
  name: kimia-build-pipeline
spec:
  params:
  - name: git-url
    type: string
    description: Git repository URL
  - name: git-revision
    type: string
    description: Git revision to build
    default: main
  - name: image-name
    type: string
    description: Target image name
  - name: image-tag
    type: string
    description: Target image tag
    default: latest
  
  workspaces:
  - name: shared-workspace
  - name: docker-config
  
  tasks:
  - name: fetch-repository
    taskRef:
      name: git-clone
    workspaces:
    - name: output
      workspace: shared-workspace
    params:
    - name: url
      value: $(params.git-url)
    - name: revision
      value: $(params.git-revision)
  
  - name: build-push
    taskRef:
      name: kimia-build
    runAfter:
    - fetch-repository
    workspaces:
    - name: source
      workspace: shared-workspace
    - name: dockerconfig
      workspace: docker-config
    params:
    - name: IMAGE
      value: "$(params.image-name):$(params.image-tag)"
    - name: DOCKERFILE
      value: ./Dockerfile
    - name: CONTEXT
      value: .
---
apiVersion: tekton.dev/v1beta1
kind: Task
metadata:
  name: kimia-build
spec:
  params:
  - name: IMAGE
    description: Reference of the image to build
  - name: DOCKERFILE
    default: ./Dockerfile
  - name: CONTEXT
    default: .
  - name: EXTRA_ARGS
    default: ""
  
  workspaces:
  - name: source
  - name: dockerconfig
    optional: true
  
  steps:
  - name: build-and-push
    image: ghcr.io/rapidfort/kimia:latest
    workingDir: $(workspaces.source.path)
    securityContext:
      runAsUser: 1000
      allowPrivilegeEscalation: true
      capabilities:
        drop: [ALL]
        add: [SETUID, SETGID]
    script: |
      #!/bin/sh
      set -e
      kimia \
        --context=$(params.CONTEXT) \
        --dockerfile=$(params.DOCKERFILE) \
        --destination=$(params.IMAGE) \
        --cache=true \
        $(params.EXTRA_ARGS)
    volumeMounts:
    - name: docker-config
      mountPath: /home/kimia/.docker
    env:
    - name: DOCKER_CONFIG
      value: /home/kimia/.docker
  
  volumes:
  - name: docker-config
    secret:
      secretName: $(workspaces.dockerconfig.bound ? workspaces.dockerconfig.claim.name : "empty-secret")
      optional: true
```

---

## Jenkins Pipeline

Complete Jenkins Pipeline with Kimia:

```groovy
pipeline {
  agent {
    kubernetes {
      yaml '''
apiVersion: v1
kind: Pod
metadata:
  labels:
    jenkins: agent
spec:
  securityContext:
    runAsNonRoot: true
    runAsUser: 1000
    fsGroup: 1000
  containers:
  - name: kimia
    image: ghcr.io/rapidfort/kimia:latest
    command:
    - cat
    tty: true
    securityContext:
      allowPrivilegeEscalation: true
      capabilities:
        drop: [ALL]
        add: [SETUID, SETGID]
    volumeMounts:
    - name: docker-config
      mountPath: /home/kimia/.docker
    resources:
      requests:
        memory: "2Gi"
        cpu: "1"
      limits:
        memory: "8Gi"
        cpu: "4"
  volumes:
  - name: docker-config
    secret:
      secretName: registry-credentials
'''
    }
  }
  
  environment {
    REGISTRY = 'myregistry.io'
    IMAGE_NAME = 'myapp'
    GIT_COMMIT_SHORT = sh(script: "git rev-parse --short HEAD", returnStdout: true).trim()
  }
  
  stages {
    stage('Checkout') {
      steps {
        checkout scm
      }
    }
    
    stage('Build and Push') {
      steps {
        container('kimia') {
          sh '''
            kimia \
              --context=. \
              --dockerfile=Dockerfile \
              --destination=${REGISTRY}/${IMAGE_NAME}:${BUILD_NUMBER} \
              --destination=${REGISTRY}/${IMAGE_NAME}:${GIT_COMMIT_SHORT} \
              --destination=${REGISTRY}/${IMAGE_NAME}:latest \
              --build-arg=VERSION=${BUILD_NUMBER} \
              --build-arg=GIT_COMMIT=${GIT_COMMIT_SHORT} \
              --label=jenkins.build=${BUILD_NUMBER} \
              --label=git.commit=${GIT_COMMIT_SHORT} \
              --cache=true \
              --push-retry=3 \
              --verbosity=info
          '''
        }
      }
    }
    
    stage('Deploy') {
      when {
        branch 'main'
      }
      steps {
        sh """
          kubectl set image deployment/${IMAGE_NAME} \
            ${IMAGE_NAME}=${REGISTRY}/${IMAGE_NAME}:${BUILD_NUMBER} \
            -n production
          kubectl rollout status deployment/${IMAGE_NAME} -n production
        """
      }
    }
  }
  
  post {
    success {
      echo "Build successful! Images pushed:"
      echo " - ${REGISTRY}/${IMAGE_NAME}:${BUILD_NUMBER}"
      echo " - ${REGISTRY}/${IMAGE_NAME}:${GIT_COMMIT_SHORT}"
      echo " - ${REGISTRY}/${IMAGE_NAME}:latest"
    }
    failure {
      echo "Build failed. Check logs for details."
    }
  }
}
```

---

## GitHub Actions

```yaml
name: Build with Kimia

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

env:
  REGISTRY: ghcr.io
  IMAGE_NAME: ${{ github.repository }}

jobs:
  build:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write
    
    steps:
      - name: Checkout code
        uses: actions/checkout@v4
      
      - name: Set up kubectl
        uses: azure/setup-kubectl@v3
      
      - name: Configure Kubernetes
        run: |
          echo "${{ secrets.KUBECONFIG }}" > $HOME/.kube/config
      
      - name: Create registry secret
        run: |
          kubectl create secret docker-registry registry-credentials \
            --docker-server=${{ env.REGISTRY }} \
            --docker-username=${{ github.actor }} \
            --docker-password=${{ secrets.GITHUB_TOKEN }} \
            --dry-run=client -o yaml | kubectl apply -f -
      
      - name: Build and push with Kimia
        run: |
          cat <<EOF | kubectl apply -f -
          apiVersion: batch/v1
          kind: Job
          metadata:
            name: kimia-build-${{ github.run_number }}
          spec:
            ttlSecondsAfterFinished: 600
            template:
              spec:
                restartPolicy: Never
                securityContext:
                  runAsNonRoot: true
                  runAsUser: 1000
                  fsGroup: 1000
                containers:
                - name: kimia
                  image: ghcr.io/rapidfort/kimia:latest
                  args:
                    - --context=https://github.com/${{ github.repository }}.git
                    - --git-revision=${{ github.sha }}
                    - --destination=${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}:${{ github.sha }}
                    - --destination=${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}:latest
                    - --build-arg=VERSION=${{ github.run_number }}
                    - --build-arg=GIT_COMMIT=${{ github.sha }}
                    - --label=build.number=${{ github.run_number }}
                    - --cache=true
                  securityContext:
                    allowPrivilegeEscalation: true
                    capabilities:
                      drop: [ALL]
                      add: [SETUID, SETGID]
                  volumeMounts:
                  - name: docker-config
                    mountPath: /home/kimia/.docker
                volumes:
                - name: docker-config
                  secret:
                    secretName: registry-credentials
          EOF
          
          kubectl wait --for=condition=complete job/kimia-build-${{ github.run_number }} --timeout=600s
          kubectl logs job/kimia-build-${{ github.run_number }}
```

---

## GitLab CI

```yaml
variables:
  KIMIA_IMAGE: ghcr.io/rapidfort/kimia:latest

stages:
  - build
  - deploy

build:
  stage: build
  image: bitnami/kubectl:latest
  script:
    # Create registry credentials
    - kubectl create secret docker-registry registry-credentials
        --docker-server=$CI_REGISTRY
        --docker-username=$CI_REGISTRY_USER
        --docker-password=$CI_REGISTRY_PASSWORD
        --dry-run=client -o yaml | kubectl apply -f -
    
    # Create and run build job
    - |
      cat <<EOF | kubectl apply -f -
      apiVersion: batch/v1
      kind: Job
      metadata:
        name: kimia-build-$CI_PIPELINE_ID
      spec:
        ttlSecondsAfterFinished: 600
        template:
          spec:
            restartPolicy: Never
            securityContext:
              runAsNonRoot: true
              runAsUser: 1000
              fsGroup: 1000
            containers:
            - name: kimia
              image: $KIMIA_IMAGE
              args:
                - --context=$CI_REPOSITORY_URL
                - --git-revision=$CI_COMMIT_SHA
                - --destination=$CI_REGISTRY_IMAGE:$CI_COMMIT_SHA
                - --destination=$CI_REGISTRY_IMAGE:latest
                - --build-arg=VERSION=$CI_PIPELINE_ID
                - --build-arg=GIT_COMMIT=$CI_COMMIT_SHORT_SHA
                - --label=pipeline.id=$CI_PIPELINE_ID
                - --cache=true
              securityContext:
                allowPrivilegeEscalation: true
                capabilities:
                  drop: [ALL]
                  add: [SETUID, SETGID]
              volumeMounts:
              - name: docker-config
                mountPath: /home/kimia/.docker
            volumes:
            - name: docker-config
              secret:
                secretName: registry-credentials
      EOF
    
    # Wait for job completion
    - kubectl wait --for=condition=complete job/kimia-build-$CI_PIPELINE_ID --timeout=600s
    - kubectl logs job/kimia-build-$CI_PIPELINE_ID

deploy:
  stage: deploy
  image: bitnami/kubectl:latest
  script:
    - kubectl set image deployment/myapp myapp=$CI_REGISTRY_IMAGE:$CI_COMMIT_SHA -n production
    - kubectl rollout status deployment/myapp -n production
  only:
    - main
```

---

[Back to Main README](../README.md) | [Examples](examples.md) | [Comparison](comparison.md)
