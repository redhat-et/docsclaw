# Image volume debug log (2026-05-03)

## Problem

`image:` volume type is silently replaced with `emptyDir:` at pod
creation time on the sandbox cluster. No error returned — the API
server accepts the pod but strips the image volume source.

## Cluster comparison

| | Working (NERC) | Broken (sandbox) |
|---|---|---|
| **API** | api.ocp-beta-test.nerc.mghpcc.org | api.ocp.v7hjl.sandbox2288.opentlc.com |
| **OpenShift** | 4.20.8 | 4.20.14 (not the cause — see root cause below) |
| **CRI-O** | 1.33.6-2.rhaos4.20.git6d65309 | 1.33.9-2.rhaos4.20.gitb9ac835 |
| **kubelet** | v1.33.6 | v1.33.6 |
| **FeatureGate ImageVolume** | enabled | enabled |
| **Kubelet ImageVolume** | True | True |
| **SCC** | restricted-with-image-volumes | same |
| **kube-apiserver image** | sha256:1718a3dd... | sha256:bd47556c... |
| **Pod spec after creation** | `image:` preserved | `image:` → `emptyDir:` |

## What was verified

- FeatureGate CR shows `ImageVolume: enabled`
- Kubelet configz shows `ImageVolume: True`
- SCC `restricted-with-image-volumes` includes `image` in allowed
  volumes list
- ServiceAccount `docsclaw-agent` is bound to the SCC via
  RoleBinding
- No mutating webhook matches the test pods (Kagenti webhook
  requires `kagenti.io/type: agent` label on pods, which was
  removed)
- Creating a bare pod (not via Deployment) shows the same behavior
- The volume is replaced **immediately at creation time** (within
  1 second), before kubelet scheduling
- Same pod spec created on the NERC cluster preserves the `image:`
  volume correctly

## Reproduction

```bash
# On the sandbox cluster:
cat <<'EOF' | oc apply -n panni-docsclaw -f -
apiVersion: v1
kind: Pod
metadata:
  name: image-vol-test
spec:
  serviceAccountName: docsclaw-agent
  securityContext:
    runAsNonRoot: true
  containers:
    - name: debug
      image: ghcr.io/redhat-et/docsclaw:5229121
      command: ["sleep", "infinity"]
      securityContext:
        allowPrivilegeEscalation: false
        capabilities:
          drop: ["ALL"]
        runAsNonRoot: true
        seccompProfile:
          type: RuntimeDefault
      volumeMounts:
        - name: skill-test
          mountPath: /skills/test
          readOnly: true
  volumes:
    - name: skill-test
      image:
        reference: quay.io/skillimage/business/document-summarizer:1.0.0-testing
        pullPolicy: Always
EOF

# Check immediately:
oc get pod image-vol-test -o jsonpath='{.spec.volumes[0]}' | python3 -m json.tool
# Returns: {"emptyDir": {}, "name": "skill-test"}
# Expected: {"image": {"pullPolicy": "Always", "reference": "..."}, "name": "skill-test"}
```

## Initial questions for cluster admin (answered below)

1. ~~Is this a known regression between 4.20.8 and 4.20.14?~~
   Not a version regression — caused by the peer-pods webhook.
1. ~~Can you check kube-apiserver audit logs for the pod creation
   to see which admission plugin modifies the volume?~~
   Identified: `mwebhook.peerpods.io` mutating webhook.
1. ~~Are there any cluster-level admission policies or
   OPA/Gatekeeper rules that might strip unrecognized volume
   types?~~ No OPA/Gatekeeper, but the peer-pods mutating
   webhook has the same effect.

---

## Root cause analysis (2026-05-03)

### Finding: peer-pods webhook strips `image:` volumes

The OpenShift Sandboxed Containers operator installs a mutating
webhook (`mwebhook.peerpods.io`) that intercepts **all** pod
CREATE/UPDATE operations in user namespaces. This webhook
deserializes the pod spec using Kubernetes API types bundled with
its image. Because those types predate the `ImageVolumeSource`
struct (added in Kubernetes 1.31), the `image:` field is silently
dropped during deserialization. When the webhook returns the pod
spec, the volume has been replaced with the zero value: `emptyDir: {}`.

### Evidence

| Check | Sandbox (broken) | NERC (working) |
| --- | --- | --- |
| kube-apiserver `ImageVolume` flag | `ImageVolume=true` | `ImageVolume=true` |
| FeatureGate CR `ImageVolume` | enabled (default set) | enabled (default set) |
| Peer-pods webhook present | yes | **no** |

The kube-apiserver feature gate is correctly configured on **both**
clusters. The only meaningful difference is the presence of the
peer-pods webhook on the sandbox cluster.

### Webhook details

```text
Name:            mutating-webhook-configuration
Hook:            mwebhook.peerpods.io
Service:         peer-pods-webhook-svc (openshift-sandboxed-containers-operator)
Path:            /mutate-v1-pod
Image:           registry.redhat.io/openshift-sandboxed-containers/osc-cloud-api-adaptor-webhook-rhel9
objectSelector:  {} (matches ALL pods)
namespaceSelector: excludes openshift-* and kube-* namespaces only
failurePolicy:   Fail
```

The webhook has **no objectSelector** filter. Every pod created in
user namespaces passes through it, even pods that have nothing to
do with sandboxed containers or peer pods.

The webhook logs confirm it processes every pod creation:

```text
2026/05/03 17:42:05 [pod-mutator] CPU Request: 0, CPU Limit: 0, ...
```

### Mechanism

1. User creates a pod with `volumes[].image` in the spec
1. kube-apiserver receives the request and calls the peer-pods
   mutating webhook
1. The webhook deserializes the pod into its Go struct using
   client-go types that don't have `ImageVolumeSource`
1. The `image:` field is silently dropped (unknown fields are
   pruned during JSON unmarshaling with strict types)
1. The webhook returns the pod spec (possibly unmodified otherwise)
1. The zero-value `VolumeSource{}` serializes back as `emptyDir: {}`
1. kube-apiserver persists the mutated pod to etcd

This is a known class of webhook bugs: webhooks built with older
Kubernetes client libraries silently drop API fields added in newer
versions.

### Workarounds

**Option A: exclude namespace from the webhook (requires
cluster-admin)**

Add `panni-docsclaw` to the webhook's namespace exclusion list:

```bash
oc patch mutatingwebhookconfiguration mutating-webhook-configuration \
  --type='json' \
  -p='[{"op":"add","path":"/webhooks/0/namespaceSelector/matchExpressions/0/values/-","value":"panni-docsclaw"}]'
```

**Option B: use init container fallback**

Copy skill content from the OCI image into an emptyDir at pod
startup. Works on any cluster regardless of webhook configuration:

```yaml
initContainers:
  - name: load-skill
    image: quay.io/skillimage/business/document-summarizer:1.0.0-testing
    command: ["cp", "-r", "/.", "/skills/test/"]
    volumeMounts:
      - name: skill-test
        mountPath: /skills/test
volumes:
  - name: skill-test
    emptyDir: {}
```

**Option C: report the bug**

File a bug against the `openshift-sandboxed-containers` component
requesting either:

- An objectSelector so the webhook only matches pods using
  `kata-remote` RuntimeClass
- Updated client-go dependency that includes `ImageVolumeSource`

### Commands used in this investigation

```bash
# Verify kube-apiserver feature gates (both clusters)
oc get configmap config -n openshift-kube-apiserver -o yaml \
  | grep -i imagevolume

# List mutating webhooks matching pods
oc get mutatingwebhookconfigurations -o json | python3 -c "
import json, sys
data = json.load(sys.stdin)
for wh in data.get('items', []):
    for hook in wh.get('webhooks', []):
        for rule in hook.get('rules', []):
            if 'pods' in rule.get('resources', []):
                print(f'{wh[\"metadata\"][\"name\"]}: {hook[\"name\"]}')"

# Check webhook image
oc get deployment peer-pods-webhook \
  -n openshift-sandboxed-containers-operator \
  -o jsonpath='{.spec.template.spec.containers[0].image}'

# Confirm webhook processes pod creations
oc logs deployment/peer-pods-webhook \
  -n openshift-sandboxed-containers-operator --tail=5
```

### Verification

After applying workaround A (namespace exclusion), both a bare
pod and the full skill-discovery-test Deployment correctly
preserve `image:` volumes.

**Bare pod test:**

```bash
oc get pod image-vol-test -n panni-docsclaw \
  -o jsonpath='{.spec.volumes[0]}' | python3 -m json.tool
```

```json
{
    "image": {
        "pullPolicy": "Always",
        "reference": "quay.io/skillimage/business/document-summarizer:1.0.0-testing"
    },
    "name": "skill-test"
}
```

**Full deployment test (skill-discovery-test):**

```bash
oc rollout restart deployment/skill-discovery-test -n panni-docsclaw
POD=$(oc get pod -n panni-docsclaw -l app=skill-discovery-test \
  -o jsonpath='{.items[0].metadata.name}')
oc exec "$POD" -n panni-docsclaw -- ls -la /skills/document-summarizer/
```

```text
dr-xr-xr-x    1 root     root            40 May  4 01:40 .
drwxr-xr-t    4 root     root            52 May  4 01:40 ..
-rw-r--r--    1 501      dialout       1487 Apr 21 04:26 SKILL.md
-rw-r--r--    1 501      dialout        963 Apr 22 18:38 skill.yaml
```

Both `SKILL.md` and `skill.yaml` are present and readable from
the OCI image volume.

### SCC configuration for Kagenti pods

After restoring Kagenti labels (`kagenti.io/type: agent`) on pod
templates, the Kagenti webhook injects an init container and an
authbridge sidecar. These injected containers require capabilities
not present in `restricted-with-image-volumes`:

| Requirement | Source |
| --- | --- |
| `runAsUser: 0` | Kagenti `proxy-init` init container |
| `NET_ADMIN`, `NET_RAW` capabilities | Kagenti `proxy-init` init container |
| `runAsUser: 1337` | Kagenti authbridge sidecar |
| `csi` volume (`csi.spiffe.io`) | SPIRE agent socket |

OpenShift picks **one** SCC for the entire pod, so a single SCC
must allow both image volumes and Kagenti's injected containers.

**Fix applied:** added `image` to the `kagenti-authbridge` SCC's
allowed volumes and bound `docsclaw-agent` SA to it:

```bash
# Add image volume support to kagenti-authbridge SCC
oc patch scc kagenti-authbridge --type='json' \
  -p='[{"op":"add","path":"/volumes/-","value":"image"}]'

# Create ClusterRole and RoleBinding
oc create clusterrole use-kagenti-authbridge-scc \
  --verb=use \
  --resource=securitycontextconstraints \
  --resource-name=kagenti-authbridge

oc create rolebinding docsclaw-kagenti-scc \
  --clusterrole=use-kagenti-authbridge-scc \
  --serviceaccount=panni-docsclaw:docsclaw-agent \
  -n panni-docsclaw
```

After this fix, the pod creates successfully with Kagenti sidecar
injection and image volumes preserved. The Kagenti infrastructure
ConfigMaps (`spiffe-helper-config`, `envoy-config`) must be
provisioned separately as part of Kagenti namespace setup.
