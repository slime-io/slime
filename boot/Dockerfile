FROM quay.io/operator-framework/helm-operator:v1.10.0

COPY watches.yaml ${HOME}/watches.yaml
COPY helm-charts/ ${HOME}/helm-charts/

Entrypoint ["/usr/local/bin/helm-operator", "run", "--watches-file=./watches.yaml", "--reconcile-period=0"]

