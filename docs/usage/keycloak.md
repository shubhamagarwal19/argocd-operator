# Usage

Note: This feature is currently supported only in openshift and is currently work-in-progress.

The following example shows the most minimal valid manifest to create a new Argo CD cluster with keycloak as Single sign-on provider.

```yaml
apiVersion: argoproj.io/v1alpha1
kind: ArgoCD
metadata:
  name: example-argocd
  labels:
    example: basic
spec:
  sso:
    provider: keycloak
```

## Create

Create a new Argo CD cluster in the `argocd` namespace using the provided basic example.

```bash
oc create -n argocd -f examples/argocd-basic.yaml
```

The above configuration creates a keycloak instance and its relevant resources along with the Argo CD resources. Users can login into the keycloak console using the below commands.

Get the Keycloak Route URL for Login.

```bash
oc -n argocd get route sso
```

Get the Keycloak Username and Password for Login.

```bash
oc -n argocd extract secret/sso-secret --to=-
```
