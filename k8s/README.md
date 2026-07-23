# Kubernetes manifests

Ported from `../controlplane-api/k8s/` (the Node/Elysia source) for the Go
backend, with env-var parity fixes (`APP_ENV`/`APP_NAME` instead of
`NODE_ENV`).

## Layout

```
k8s/
├── namespace.yaml            # controlplane namespace
├── configmap.yaml            # non-secret API env vars
├── secret.example.yaml       # template — copy to secret.yaml, fill in real values, never commit it
├── api/
│   ├── deployment.yaml       # controlplane-api (2 replicas)
│   ├── service.yaml
│   └── ingress.yaml          # host: controlplane.local
├── postgres/
│   ├── statefulset.yaml
│   ├── service.yaml
│   └── secret.example.yaml   # template — copy to secret.yaml
└── redis/
    ├── deployment.yaml
    └── service.yaml
```

## Apply

```bash
cp k8s/secret.example.yaml k8s/secret.yaml
cp k8s/postgres/secret.example.yaml k8s/postgres/secret.yaml
# edit both secret.yaml files with real values (min 32-char JWT secrets)

kubectl apply -f k8s/namespace.yaml
kubectl apply -f k8s/ -R
```

`k8s/secret.yaml` and `k8s/**/secret.yaml` are gitignored — never commit
real secrets.

## Not here yet

The frontend `web` Deployment/Service aren't ported yet. The Next.js
Dockerfile and compose service (Phase 6) are done — see
`apps/frontend/Dockerfile` and `compose.yaml`'s `web` service — porting that
to a k8s Deployment/Service/Ingress alongside `api/` is a follow-up.
