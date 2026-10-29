# Demo kustomize tree

The kustomizations the kustomize GIF (`docs/demo/kustomize.tape`) browses,
diffs, applies and deletes. `demo_reset` (docs/demo/env.sh) copies this
directory to `~/deploy` in the demo home. They are rendered by the demo's
fake kubectl (tools/demo/fakekube), which supports `resources`,
`namespace`, `images` (newTag) and `replicas` — enough for these files,
which are also valid for the real `kubectl kustomize`.
