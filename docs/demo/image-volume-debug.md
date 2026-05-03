# Image volume debug log (2026-05-03)

## Problem

`image:` volume type is silently replaced with `emptyDir:` at pod
creation time on the sandbox cluster. No error returned — the API
server accepts the pod but strips the image volume source.

## Cluster comparison

| | Working (NERC) | Broken (sandbox) |
|---|---|---|
| **API** | api.ocp-beta-test.nerc.mghpcc.org | api.ocp.v7hjl.sandbox2288.opentlc.com |
| **OpenShift** | 4.20.8 | 4.20.14 |
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

## Questions for cluster admin

1. Is this a known regression between 4.20.8 and 4.20.14?
2. Can you check kube-apiserver audit logs for the pod creation to
   see which admission plugin modifies the volume?
3. Are there any cluster-level admission policies or OPA/Gatekeeper
   rules that might strip unrecognized volume types?
