# k2 keyboard cheatsheet

Defaults. Change them in `~/.k2/config.yaml` under the name in the Config
column (see `config.example.yaml`). Hints in k2 always show the keys you
set. A clash is rejected with a warning and that action keeps its default.

**The prefix.** Pane and tab keys are tmux/screen style: press `ctrl+a`
(`keys.prefix`), let go, then press one key, e.g. `ctrl+a v` splits right.
The status line shows `prefix ctrl+a …` and the legend lists the keys while
k2 waits (1.5s; `esc` cancels). Works in tables, viewers and the in-pane
vim / shell. In k2's own text inputs (`:` line, `/` filter, kubectl line,
path field, popups) `ctrl+a` stays "go to line start"; leave the input first
(`esc`), or pick a prefix that is not an editing key (`keys.prefix: ctrl+\`).
Press the prefix twice to send a literal `ctrl+a` to vim / the shell.

## Global

| Key | Config | What it does |
|---|---|---|
| `ctrl+a` | `keys.prefix` | prefix for the pane and tab keys (also in vim / shell); twice = literal (formerly `keys.editor_prefix`) |
| `Z`, `ctrl+a z` | `keys.fullscreen`, `keys.zoom` | zoom the focused pane / back to the split |
| `!` | `keys.last_error` | the last error in full, in a popup: `j/k` scroll, `enter` opens it as a log, `a` opens every error this session, `[` `]` step through them, `y` copies, `esc` closes (also `ctrl+a !`) |
| `?` | `keys.help` | help popup for the current view: navigation first, then the keys of this screen (Kustomize, forwards, logs …); `j/k` `pgup/pgdown` scroll, `?`, `esc` or `q` close. In text inputs `?` is typed as usual; in vim / shell `ctrl+a ?` |
| `R` | `keys.readonly` | toggle read-only for this session: browse, filter, describe, YAML, logs and context/namespace still work; apply, delete, edit (cluster and local Kustomize / folder open), shell, scale, restart, kubectl submit and other changes are blocked. Lock icon + READ-ONLY beside the logo. Running editors, shells and port-forwards are not killed |
| `Q`, `ctrl+c` | `keys.quit` | quit k2 (exports the focused pane's kubeconfig) |
| `:` | `keys.command` | open the command line |
| `q`, `esc`, `H` | `keys.back` | back one screen (the previous `:` view or drill, or home), or clear a filter. With rows selected, the first press clears them (`cleared N selections`); another press does the usual back or filter clear |
| `~` | `keys.kind` | open kind clusters (the home shortcut is unset by default; bind `keys.home` if you want one) |
| `left`, `h` | `keys.left` | back too, but ignored while filtering |
| `enter`, `right`, `l` | `keys.select` | open / choose the selected row |

## Panes and tabs (after the prefix)

| Key | Config | What it does |
|---|---|---|
| `ctrl+a h`, `ctrl+a "` | `keys.split_down` | new pane below (stacked) |
| `ctrl+a v`, `ctrl+a %` | `keys.split_right` | new pane to the right (side by side) |
| `ctrl+a o` | `keys.next_pane` | next pane in the tab, round robin |
| — (unbound) | `keys.prev_pane` | previous pane in the tab, round robin; no default key (use the arrows after the prefix) |
| `ctrl+a ;` | `keys.last_pane` | back to the previously focused pane (toggles) |
| `ctrl+a x` | `keys.close_pane` | close the focused pane (asks in a popup; red when vim / a shell runs there) |
| `ctrl+a t`, `ctrl+a c` | `keys.new_tab` | new tab on the focused pane's target |
| `ctrl+a w`, `ctrl+a &` | `keys.close_tab` | close the tab (asks in a popup; red when vim / a shell runs there) |
| `ctrl+a ,` | `keys.rename_tab` | rename the tab in a popup (`enter` saves, `esc` cancels, empty = automatic name) |
| `]`, `ctrl+a n`, `ctrl+a ]` | `keys.next_tab` | next tab (wraps around); plain `]` is typed in text inputs and goes to vim / the shell, `ctrl+a n` works there |
| `[`, `ctrl+a p`, `ctrl+a [` | `keys.prev_tab` | previous tab (wraps around); same for `[` |
| `ctrl+a 1` | `keys.tab_1` | go to tab 1 |
| `ctrl+a 2` | `keys.tab_2` | go to tab 2 |
| `ctrl+a 3` | `keys.tab_3` | go to tab 3 |
| `ctrl+a 4` | `keys.tab_4` | go to tab 4 |
| `ctrl+a 5` | `keys.tab_5` | go to tab 5 |
| `ctrl+a 6` | `keys.tab_6` | go to tab 6 |
| `ctrl+a 7` | `keys.tab_7` | go to tab 7 |
| `ctrl+a 8` | `keys.tab_8` | go to tab 8 |
| `ctrl+a 9` | `keys.tab_9` | go to tab 9 |
| `ctrl+left`, `ctrl+a left` | `keys.pane_left` | focus the pane to the left (ctrl+arrows: no prefix; also in vim / shell) |
| `ctrl+right`, `ctrl+a right` | `keys.pane_right` | focus the pane to the right |
| `ctrl+up`, `ctrl+a up` | `keys.pane_up` | focus the pane above |
| `ctrl+down`, `ctrl+a down` | `keys.pane_down` | focus the pane below |

After `ctrl+a` plus an arrow, more arrows within 1 s keep moving (tmux
style, each press renews the window); any other key ends it and works as
usual. The sidebar is never focused.

`ctrl+d` (`keys.delete`) on **home**, with no prefix: closes the focused
pane when the tab has another one, and does not ask. The last pane of the
tab closes the tab the same way. The only tab quits, like `shift+q`.
Off home, `ctrl+d` still deletes (pods, resources, forwards, a helm
release) and still asks, even when other panes or tabs are open.

**tmux users:** after the prefix these keys mean what they mean in tmux:
`"` `%` split, `c` new tab (window), `&` close tab, `,` rename tab, `n` /
`p` next / previous tab, `o` next pane, `;` last pane, `z` zoom, `x` close
pane, the arrows (repeatable) and `1`-`9` (k2 counts tabs from 1). k2's own
`h` `v` `t` `w` work as well.

The config value of a pane or tab key is the key after the prefix
(`keys.split_right: v`). An entry written `direct KEY` binds KEY on its own,
without the prefix, as a global key: `keys.next_pane: [o, "direct O"]`,
`keys.tab_1: ["1", direct alt+1]`.

## Sidebar and command mode

The sidebar never takes focus: its targets open in the focused
pane with these keys or `:` commands.
It sits on the left unless `ui.sidebar_position` says `right`, `top` or
`bottom`; at the top or bottom it is a strip a few rows high, on the
right it lines up with the right edge.
`ui.logo` picks the logo at the top: `wordmark` (default, the pixel
`k2`) or `mountain` (a small pixel mountain over `k2`).
`ui.logo_twinkle` (on by default) blinks a few pixels in the theme's bright color; `false` keeps the logo still.
Forwards too long for it scroll sideways (`ui.forward_scroll: false`
cuts them with `…`).

| Key | Config | What it does |
|---|---|---|
| `C` | `keys.kubeconfig` | pick a kubeconfig (`:kubeconfig`, `:kube`, `:kc`, `:c`) |
| `c` | `keys.context` | pick a context (`:context`, `:ctx`, `:x`). Returns to the previous view afterwards, like namespace |
| `n` | `keys.namespace` | pick a namespace / OpenShift project (`:namespace`, `:ns`, `:project`, `:projects`, `:proj`, `:o`). From a pod list, resource list, pulse, helm or stern, k2 returns to that view afterwards (back does too); from home it returns home |
| `K` | `keys.kustomize` | kustomize browser (`:kustomize`, `:kust`, `:kustomization`, `:k`) |
| `U` | `keys.kubectl` | kubectl view (`:kubectl`, `:shell`, `:sh`) |
| `f` | `keys.forwards` | running port-forwards (`:forwards`, `:pf`, `:fwd`, `:fwds`, `:f`) |
| `b` | `keys.favorites` | favorites picker: pinned context + namespace (+ view) (`:fav`, `:favorites`); every screen, not in text inputs |
| `B` | `keys.cmd_favorites` | favorite commands popup (`:cmds`, `:commands`): saved `:` commands such as `logs app=web -c main`, run with one key; every screen, not in text inputs |
| `ctrl+s` | `keys.cmd_save` | in the `:` line: save the typed command as a favorite without running it (`:cmds add <command>` does the same) |
| `ctrl+t` | `keys.theme` | popup of themes for the current context (`:theme`), previewed as you move, `esc` restores; the choice is written to `ui.context_themes`. Not `ctrl+shift+t`: the terminal takes that chord, and k2 does not receive ctrl+shift+letter |
| `ctrl+l` | `keys.login` | paste a new token for the current context (`:login`); it replaces that user's `token` in the kubeconfig, or the `tokenFile` when the token lives in a file |
| `ctrl+p` | `keys.profile` | save the open panes as a profile (`:profile save name`). `k2 --profile name` or `:profile name` restores them. Files live in `~/.k2/profiles/` |
| — | — | update k2 from the shell: `k2 --update` (or `k2 -U`) runs the repo's install.sh (`.deb` or `.rpm`, needs bash and curl or wget; `K2_VERSION=<tag>` picks a release); it never starts the UI |
| — | — | drop every tab and pane and open one home pane (`:fresh`, or `k2 --fresh` at start). The focused cluster, port-forwards and favorites stay |
| — | — | owner tree of every object of a kind (`:xray deploy`, `:xray sts db`; deploy, sts, ds, rs, job, cronjob, pod, svc) |
| — | — | logs of many pods, stern-like (`:logs app=web`, `:logs ^web- --since=10m -c app`; options `--since`, `--tail`, `-c`, `--all=false`) |
| — | — | helm releases (`:helm`, `:helm all` for every namespace; `:helm install` / `:helm upgrade` prefill the install form) |
| `~` | `keys.kind` | kind clusters (`:kind`, `:kind create [name]`) |
| — | — | namespace pulse: health of the pane's namespace (`:pulse`) |
| — | — | pods (`:pods`, `:pod`, `:po`, `:r`); the pods list to start a forward from (`:forward`, `:p`) |
| `tab` | `keys.complete` | complete a `:` command (targets, cluster resources, aliases); again cycles |
| `right` | — | take the dimmed suggestion after the cursor on the `:` line |
| `enter` | `keys.submit` | run the `:` line |
| `esc` | `keys.cancel` | close the `:` line |

Any resource name works too (`:deployments`, `:routes` …), plus your
`aliases:` and the built-in short names `:deploy` deployments, `:sts`
statefulsets, `:ds` daemonsets, `:rs` replicasets, `:cm` configmaps, `:sec`
secrets, `:svc` services, `:ing` ingresses, `:pvc` persistentvolumeclaims,
`:cj` cronjobs, `:ev` events, `:crd` CRDs, `:sub` subscriptions, `:csv`
clusterserviceversions, `:ip` installplans, `:og` operatorgroups, `:cs`
catalogsources, `:rt` routes, `:proj` projects, `:csr` cert-manager
CertificateRequests (Kubernetes CSRs: `:certificatesigningrequests`).
`:certs` lists the issuer. A name served by several
API groups can be spelled out: `:gateways.networking.istio.io`,
`:gateway.networking.k8s.io/v1/gatewayclasses`.

## Resource tables (every list)

| Key | Config | What it does |
|---|---|---|
| `space` (` `) | `keys.mark` | select or unselect the row. `ctrl+d` deletes every selected row, `M` tails exactly those pods. With nothing selected, both keys keep their old behaviour. With rows selected, `q` or `esc` clears them all (`cleared N selections`); press again to go back or to clear a filter |
| `k`, `up` | `keys.scroll_up` | previous row |
| `pgdown`, `}` | `keys.page_down` | page down |
| `pgup`, `{` | `keys.page_up` | page up |
| `g` | `keys.top` | first row |
| `G` | `keys.bottom` | last row |
| `/` | — | filter (shown in the title; `esc` clears) |
| `d` | `keys.describe` | kubectl describe the selected object |
| `e` | `keys.edit` | edit the selected object in vim (inside the pane) |
| `ctrl+d` | `keys.delete` | kubectl delete the selected object (asks in a red popup), also when other panes or tabs are open. On home, `ctrl+d` closes the pane, or the tab when it is the last pane; the only tab quits |
| `>` | `keys.sort_next` | sort by the next column |
| `<` | `keys.sort_prev` | sort by the previous column |
| `=` | `keys.sort_reverse` | flip ascending / descending |
| `w` | `keys.copy_name` | copy the name to the clipboard (`ui.clipboard`); legend `copy name` |
| `W` | `keys.copy_ref` | copy `kind/name -n ns` to the clipboard |
| `L` | `keys.label` | label popup (`key=value` or `key-`) |
| `A` | `keys.annotate` | annotate popup |
| `T` | `keys.copy_secret` | secrets: copy the selected secret to another namespace (legend `copy to ns`): pick the namespace (type to filter), keep or change the name, `y` to copy. An existing target asks to overwrite (red). uid, resourceVersion, timestamps, managedFields, ownerReferences, last-applied and Helm ownership (`meta.helm.sh/*`, `helm.sh/*`, `managed-by: Helm`) are dropped; type, data and other labels / annotations stay. Blocked in read-only (`R`) |
| `x` | `keys.decode` | secrets: decode the selected secret; in the secret's viewer it cycles encoded (YAML), decoded and, for x509 values, a parsed certificate view (subject, issuer, validity with days left, alt names, key, usages, SHA-256); without a certificate it toggles encoded / decoded; `r` watches any of them |
| `X` | `keys.xray` | owner tree (xray) of the selected row: deployments, statefulsets, daemonsets, replicasets, jobs, cronjobs, pods, services (k9s xray) |
| `M` | `keys.multi_logs` | logs of all pods of the selected workload / service (its selector) or of a pod's siblings, stern-like |
| `ctrl+f`, `F` | `keys.port_forward` | port-forward popup (pods, services, deployments, statefulsets) |
| `0` | `keys.all_namespaces` | all namespaces (title `all/<kind>`, NAMESPACE column, the selected object kept); again: back to the namespace before (remembered per pane, also across restarts). Cluster-scoped kinds: a `cluster-scoped` toast. An empty list says `no pods in shop — press 0 for all namespaces` |

## Pods

| Key | Config | What it does |
|---|---|---|
| `enter` | `keys.select` | logs of a one-container pod, otherwise its containers (see below) |
| `l` | `keys.pod_logs` | logs of the pod (container picker when it has several) |
| `p` | `keys.previous` | previous logs of the pod (container picker when it has several) |
| `F` | `keys.port_forward` | port-forward. `ctrl+f` still works; the pods legend shows `F` |
| `S` | `keys.scale` | scale the deployment or statefulset that owns this pod |
| `s` | `keys.shell` | shell in the pod, inside the pane (bash, else sh) |
| `ctrl+d` | `keys.delete` | delete the pod (asks in a red popup), also with other panes or tabs open |

### Containers (`enter` on a pod with several containers)

Title `Containers(shop/web-1)[3]`. Init containers come first and are marked
`(init)`, native sidecars `(sidecar)`. Columns: NAME, PF (`●` when a
forward of one of its ports is running), IMAGE, READY, STATE (Running, the
Waiting or Terminated reason such as CrashLoopBackOff or Completed), INIT,
RESTARTS (`3 (5m ago)`), PROBES(L:R) (`on`/`off` for liveness and
readiness), CPU and MEM (`kubectl top pod --containers`, `n/a` without
metrics-server), CPU/R:L (requests:limits in millicores), MEM/R:L (in Mi),
%CPU/R, %CPU/L, %MEM/R, %MEM/L (usage against requests and limits), PORTS
(`name:port`) and AGE. Narrow panes drop the least important columns first.
The list refreshes by itself; `/` filters, `esc`, `q` or `h` goes back to the
pods with the same pod selected.

| Key | Config | What it does |
|---|---|---|
| `enter`, `l` | `keys.select`, `keys.pod_logs` | logs of this container |
| `p` | `keys.previous` | previous logs of this container |
| `s` | `keys.shell` | shell in this container (blocked in read-only mode) |
| `F` | `keys.port_forward` | port-forward the pod, this container's ports first (blocked in read-only mode) |
| `d` | `keys.describe` | describe the pod |
| `esc` | `keys.back` | back to the pods |

Keys that act on the pod itself (delete, scale, set image, edit, xray, mark,
all namespaces) do nothing here and say so.

CPU and MEM show `value ███░░ 62%` (usage of the summed limits; `~62%` =
of the requests), green / yellow from `ui.metrics_warn` (70) / red from
`ui.metrics_crit` (90).
`:nodes` gets the same CPU and MEM columns from `kubectl top nodes`
(usage of the allocatable), left out without metrics-server.

## Workloads

| Key | Config | What it does |
|---|---|---|
| `S` | `keys.scale` | deployments, statefulsets, replicasets: scale popup. On a pod, the same key scales the deployment or statefulset that owns it |
| `i` | `keys.set_image` | pods, deployments, statefulsets, daemonsets, replicasets, cronjobs: set image popup (containers and init containers; only changed ones are sent, after a y/n) (k9s `i`) |
| `r` | `keys.restart` | deployments, statefulsets, daemonsets: rollout restart (asks y/n) |
| `t` | `keys.trigger` | cronjobs: create a job now (asks y/n) |
| `enter` | `keys.select` | jobs: list the job's pods |

## Owner tree (`X`, `:xray`)

Deployment → ReplicaSets → Pods → containers, with the services that select
the pods and their ingresses, and the config maps, secrets and PVCs the pods
use. Drawn as a table (NAME holds the tree). A whole namespace (`:xray
deploy`, no name) starts folded: one line per root with a summary (`2/2
ready · 1 rs · 2 pods · 0 restarts`); roots with not-ready or restarted
pods start open. `:xray deploy <name>` (or `X` on a row) shows one tree,
open; tab completes the kind and the name.

| Key | Config | What it does |
|---|---|---|
| `space` (` `) | `keys.xray_toggle` | collapse / expand the selected node (`▸` collapsed, `▾` open) |
| `+` | `keys.xray_expand_all` | expand every node |
| `-` | `keys.xray_collapse_all` | collapse every root to its summary line |
| `/` | `keys.xray_filter` | filter by name: matches with the path to them (empty clears) |
| `enter` | `keys.select` | jump to the node: its table with the row selected; a container opens its log; back returns to the tree |
| `d` | `keys.describe` | describe the node |
| `y` | `keys.yaml` | the node's yaml (back returns to the tree) |
| `M` | `keys.multi_logs` | logs of every pod under the node (a container: that container) |
| `0` | `keys.all_namespaces` | namespace-wide tree: all namespaces / back (the tree of one object stays in its namespace) |
| `w` / `W` | `keys.copy_name` / `keys.copy_ref` | copy the name / `kind/name -n ns` |

## Helm releases (`:helm`)

Read with kubectl from Helm's release secrets (no helm CLI needed to
browse). Install, upgrade, rollback and uninstall run the helm CLI
(`helm.binary`) and are offered only when it is found. `e` edits the
selected release's values in vim and, after a save, confirms an upgrade.
Read-only mode (`R`) blocks those and `e`.

`A` (or `:helm install NAME CHART [-n NS] [--create-namespace] [-f FILE] [--version V]`)
opens a form for the release name, chart (catalog names first, then
`helm search repo`; a compact list under the chart field shows matches,
and `tab` / `shift+tab` cycle them, filling release, namespace, create-ns
and default values unless you edited those fields; `↑` `↓` move between
fields),
version, namespace (with a create-if-missing toggle) and values file
(pick a path, or `ctrl+e` to edit it when the values field is focused).
Chart defaults is a read-only `helm show values` preview (`esc` returns
to the form) and never overwrites the values file.
Built-in catalog: istio (`base` then `istiod`), sealed-secrets, metallb,
argocd, cert-manager, metrics-server, kube-prometheus-stack, kafbat.
Override or extend under `helm.charts` in `~/.k2/config.yaml`. For
`sealed-secrets`, a master-key row appears (default
`helm.charts.sealed-secrets.master_key_path`; `ctrl+p` opens the file
picker). k2 applies that Secret YAML with `kubectl apply -f` before
`helm install`. k2 adds
and updates the chart's Helm repo before install.
Preview with `helm template` shows kinds and a scrollable manifest; a red
popup names the context, release, chart and namespace. On success k2 jumps
to the release and shows its notes. `u` (or `:helm upgrade …`) opens the
same form on the selected release: name, chart and namespace stay fixed,
version and values can change, and the preview is a per-resource diff
against the current manifest.

| Key | Config | What it does |
|---|---|---|
| `v`, `enter` | `keys.helm_values` | user-supplied values |
| `V` | `keys.helm_computed` | computed values (chart defaults merged with the user's) |
| `M` | `keys.helm_manifest` | the rendered manifest |
| `N` | `keys.helm_notes` | the chart notes |
| `t` | `keys.helm_history` | revision history: a table of revisions (see below) |
| `0` | `keys.all_namespaces` | all namespaces / the pane's namespace (k9s `0`) |
| `A` | `keys.helm_install` | install a chart (helm CLI; form, preview, red popup) |
| `u` | `keys.helm_upgrade` | upgrade the selected release (helm CLI; form, diff preview, red popup) |
| `e` | `keys.edit` | edit user-supplied values in vim, then confirm an upgrade (helm CLI; also from the `v` values view) |
| `r` | `keys.helm_rollback` | roll back to the previous revision (helm CLI; red popup) |
| `ctrl+d` | `keys.delete` | uninstall the release (helm CLI; red popup), also with other panes or tabs open |

An empty list in one namespace says so: `no releases in shop — press 0 for
all namespaces`.

### Helm history (`t` on a release)

Newest first; `●` marks the current revision, `◆` the marked one. Diffs
are computed by k2 from the release secrets (no helm CLI): per resource
(`=== ~ Deployment/web`, `=== + Service/web (added)`, `=== - … (removed)`)
plus the user-supplied values, `+` green, `-` red, `@@` dim. The diff is a
text viewer (`w` wrap, `/` search, `y` copy mode, `v` vim); back returns to
the history, back again to the releases.

| Key | Config | What it does |
|---|---|---|
| `r` | `keys.helm_rollback` | roll back to the SELECTED revision (`helm rollback NAME REV`; red popup naming both revisions, charts, app versions and the context; the current revision is refused) |
| `space` (` `) | `keys.helm_mark` | mark / unmark a revision to diff against |
| `D` | `keys.helm_diff` | diff the selected revision with the marked one, or with the current one |
| `v` `V` `M` `N` | `keys.helm_values` … | that revision's values, computed values, manifest, notes |

## Kind clusters (`:kind`, `~`)

Needs the kind CLI (`kind.binary`, default `kind`) and docker. Starter
charts also need helm. Read-only mode (`R`) blocks create, delete,
chart edits and the registry proxy start / remove. Kubeconfig is `~/.kube/k2` when the name is `k2`, otherwise
`~/.kube/k2-<name>`. `ctrl+d` removes only that file, never
`~/.kube/config`. If that cluster was the pane's active one and exactly
one other kind cluster still has a kubeconfig, the pane switches to it.
After MetalLB installs on a `kind-*` cluster, k2 applies an
IPAddressPool + L2Advertisement from the kind Docker network subnet.

Clusters show as full-width cards stacked in one column (name, square
status, uptime, a Kubernetes version line, control-plane and worker
counts, a background pod count, CPU as cores + share of host CPUs
(`1.4c (7%)`, bar and color follow that share), RAM usage + percent
+ bar, installed catalog charts wrapped across lines, kubeconfig). On a
wide pane, k8s / nodes / pods sit on the left and CPU / RAM on the right;
a narrow pane stacks those rows. The list scrolls if more cards are open
than fit. The active cluster shows `⬢ active` next to its name on the left of
the name line, and a filled hexagon (`⬢`) in the top-right corner, one
space from the border; other cards show just the name on the left and an
empty hexagon (`⬡`) on the right. `r` refreshes the list immediately, including pod counts. Re-opening the
view shows the last snapshot at once; stats, pod counts and catalog
charts otherwise refresh about every 15 seconds while the view is open.
A new cluster appears as a `deploying` card as soon as create starts;
`space` on a ready card uses that cluster in the pane. `enter` follows
live control-plane logs (`docker logs --tail 200 -f
<name>-control-plane`), or opens the job log of a deploying / failed /
deleting cluster (auto-scrolls unless you move up; `esc` returns to the
cards while a create or delete job keeps running). On success the card
turns `running` and the pane switches to that cluster. Failed jobs stay
in the list; `enter` still opens the log.

`A` or `:kind create [name]` opens a form (name, control-planes, workers,
a prefilled generated config path, node image, starter-chart checkboxes).
`ctrl+e` on the config field opens that YAML in `$EDITOR` (edits to
node counts and image flow back into the form, and form changes keep
the generated file in sync), or type your own path. The generated YAML
has a `KubeletConfiguration` patch for parallel image pulls
(`serializeImagePulls: false`, `maxParallelImagePulls: 4`); remove or
change it in `ctrl+e` and it stays that way. The image field lists the five latest stable `kindest/node`
tags under the input (`tab` / `shift+tab` cycle, or type a custom
image). A checked chart opens an indented `namespace` row under it
(catalog default, type to change; installs there with
`--create-namespace`, shown in the red popup). `v` on a catalog chart edits its values in `$EDITOR` and
passes them to Helm. On `sealed-secrets`, `m` picks a master-key YAML
(the same file browser as Kustomize `p`; default
`helm.charts.sealed-secrets.master_key_path`, else the last picked path, remembered in the state file) that k2 applies before
`helm install`. Enter asks in a red popup (`esc`/`q` back to the
form, `n` cancels), then streams `kind
create cluster`. On a running tile, `e` opens a checklist of the catalog
charts (already-installed ones are checked, their `namespace` row is
read-only); `v` edits values the same
way, `m` on `sealed-secrets` picks a master key, and enter confirms installs, value upgrades, and uninstalls (istio
in reverse: istiod, then istio-base) with the same live tile and log.

| Key | Config | What it does |
|---|---|---|
| `~` | `keys.kind` | open this view from anywhere; `q` goes back to the view before |
| `h` `j` `k` `l`, arrows, `tab` | `keys.left` / `keys.scroll_down` / `keys.scroll_up` | move up and down between cards |
| `space` | `space` | use a ready cluster in the pane, or open the job log of a deploying / failed / deleting one |
| `enter` | `keys.select` | use a running cluster and open its `:pods`; live control-plane logs of a stopped one, or the job log of a deploying / failed / deleting one |
| `A` | `keys.helm_install` / `keys.apply` | create a cluster (form, red popup) |
| `e` | `keys.edit` | manage catalog charts on the selected cluster |
| `ctrl+d` | `keys.delete` | delete the cluster and its k2 kubeconfig (red popup); blocked while that cluster is still deploying or deleting |
| `r` | `keys.watch` | refresh the grid and node states (in-flight jobs stay, finished and failed job overlays are cleared) |
| `p` | `keys.kind_proxy` | start the registry proxy (works with no clusters too) |
| `s` | `keys.kind_toggle` | stop the selected running cluster (`docker stop` on all its nodes, asks first) or start a stopped one; on the proxy card stops / starts the proxy container. Blocked in read-only mode |
| `enter` / `ctrl+d` | `keys.select` / `keys.delete` | on the registry proxy card: follow its logs / remove the container (cache stays on disk) |

### Registry proxy (image cache)

One shared pull-through cache for all kind clusters, using
`rpardini/docker-registry-proxy:0.8.0` (pinned, `:latest` breaks docker.io
token auth). `p` asks, then runs it on the `kind`
Docker network as `k2-registry-proxy`, with cached layers and its CA in
`~/.k2/kind-cache` (`cache/` and `ca/`). It shows as its own card with a
`⇄ registry proxy` badge: status, uptime, cache size, registries. Not a
cluster: `space` and `e` do nothing there. `ctrl+d` removes only the
container; deleting a cluster never touches it. Settings live under
`kind.registry_proxy` (`image`, `container`, `cache_dir`, `registries`).
Proxy timeouts are 1200s with request buffering off, so multi-GB layers
are not cut mid-pull; an old proxy container picks that up after
`ctrl+d` and `p` again.

Clear it from the shell: `k2 kind-cache clean cache` (layers only, keeps
the CA), `k2 kind-cache clean ca` (CA only), `k2 kind-cache clean all`
(both). Prints what it freed; `ca` / `all` refuse while the proxy runs
(stop it with `s` on its card). Root-owned files are removed through
docker when the host user can't.

In `A`, `registry cache` (`space` toggles) is `yes` when the proxy is
running. With `yes`, k2 starts the proxy if needed, then on every node
trusts its CA and points containerd at it (`HTTP_PROXY` / `HTTPS_PROXY`,
`NO_PROXY` for cluster and kind network ranges), before charts install.
Private images keep working with normal `imagePullSecrets`.

## Multi-pod logs (`M`, `:logs`)

A text viewer (all viewer keys work: `w` wrap, `y` copy mode, `v` vim, `G`
follow). Every line starts with its colored `pod/container`; new pods are
picked up every `logs.multi_every` seconds; each pod is streamed, not polled;
the newest
`logs.multi_max_lines` lines are kept. Closing k2 and opening it again
returns to this view and follows the same query from scratch, as a pod
log does. Back from a restored view goes home.

| Key | Config | What it does |
|---|---|---|
| `/` | `keys.log_filter` | show only lines containing a text (a regex when it is one); empty shows all |
| `0` | `keys.all_namespaces` | follow the pods in all namespaces (lines prefixed `namespace/pod/container`) / back |

## Favorites picker (`b`, `:fav`)

| Key | Config | What it does |
|---|---|---|
| `enter`, `1`-`9` | `keys.submit` | jump the focused pane there (and open its view) |
| `a` | `keys.fav_add` | pin the focused pane's kubeconfig, context, namespace and view |
| `x` | `keys.fav_remove` | unpin the selected favorite (not the ones from config.yaml) |
| `K` / `J` | `keys.fav_up` / `keys.fav_down` | move the selected favorite up / down |
| `j`, `k` | `keys.scroll_down` / `keys.scroll_up` | move the cursor |
| `esc` | `keys.cancel` | close |

## Favorite commands (`B`, `:cmds`)

Saved `:` commands, shared by every context. `:cmds <label, number or part of the command>` runs one without opening the popup.

| Key | Config | What it does |
|---|---|---|
| `enter`, `1`-`9` | `keys.submit` | run the command as if typed after `:` (read-only mode blocks the same ones it blocks when typed) |
| `a` | `keys.fav_add` | save the last command run from the `:` line |
| `e` | `keys.cmd_edit` | edit the selected command text, `enter` saves, `esc` keeps it (not the ones from config.yaml) |
| `n` | `keys.cmd_rename` | set or clear the label of the selected command |
| `c` | `keys.cmd_clone` | duplicate the selected favorite (a config.yaml one too) as the last entry, with ` copy` added to its label, and select it so `e` edits it |
| `x` | `keys.fav_remove` | remove the selected favorite (not the ones from config.yaml) |
| `K` / `J` | `keys.fav_up` / `keys.fav_down` | move the selected favorite up / down |
| `j`, `k` | `keys.scroll_down` / `keys.scroll_up` | move the cursor |
| `esc` | `keys.cancel` | close |

## Namespace pulse (`:pulse`)

A text viewer refreshed every `ui.refresh_every` seconds: pods by phase,
crash loops and restarts, workloads ready / total, jobs, PVCs, CPU / memory
use against the limits (`ui.metrics_warn` / `ui.metrics_crit` colors, needs
metrics-server), the objects in trouble and the newest warning events.
It reads one namespace: `0` (`keys.all_namespaces`) only says so.

## OLM / OpenShift views

| Key | Config | What it does |
|---|---|---|
| `a` | `keys.approve` | installplans: approve a Manual plan (asks y/n) |
| `enter` | `keys.select` | subscriptions: the installed CSV; CSVs: what they own; CRDs: their resources |

## Text viewers (logs, describe, yaml, events output, kubectl / kustomize output)

| Key | Config | What it does |
|---|---|---|
| `w` | `keys.wrap` | toggle line wrap in this pane (start state: `ui.wrap`) |
| `r` | `keys.watch` | on YAML (`enter`), describe (`d`) and a secret's decoded / certificate views: refresh that object every `ui.watch_every` seconds (2 by default). While it is on, `⟳ watch 2s` sits at the pane's top-right (a shorter mark if the title is tight). `r` again stops. The same letter is still rollout restart on a workload, and restart of a forward, because those are different screens |
| `y` | `keys.copy_mode` | copy mode: only the text on the whole screen (see below). The legend says so in the log view too, including when the pane is zoomed (`Z`) |
| `m` | `keys.log_mark` | pod logs and stern (`M`, `:logs`): draw a rule after the current line (the tail, or the line on screen when paused). New lines keep arriving under it. `m` again on that line removes the rule |
| `v` | `keys.vim` | open the text in vim |
| `p` | `keys.previous` | logs: the previous container's log and back |
| `1` | `keys.log_30m` | logs of one container (not stern): open the last 30 minutes in `less`, from the moment you press it. `q` closes it. The live tail stays at its 400-line cap |
| `2` | `keys.log_60m` | the same for the last 60 minutes |
| `/` | `keys.search` | find a text: scrolls to the next line containing it; `/` `enter` again for the next (wraps around) |
| `j`, `down` / `k`, `up` | `keys.scroll_down` / `keys.scroll_up` | scroll one line |
| `pgdown`, `}` / `pgup`, `{` | `keys.page_down` / `keys.page_up` | scroll one page |
| `g` | `keys.top` | top (pauses log tailing) |
| `G` | `keys.bottom` | bottom; resumes log tailing and catches up. While paused the lines on screen never move, even on a very busy log; the header counts what arrived meanwhile (`paused +120 new`) |

In output views (kustomize and kubectl results, viewer errors) error and
warning lines always wrap in full, whatever `w` says; `!`
(`keys.last_error`) shows the last error from anywhere in full. `enter` in that
popup opens the response as a log; `a` opens every error from this session.

## Copy mode (`y` in a text viewer)

Only the text, at the terminal's full width and height: no borders,
sidebar, title, line numbers or status line, so the terminal's own mouse
selection copies cleanly (k2 never captures the mouse). One hint line at
the bottom unless `ui.copy_mode_hint: false`.

| Key | Config | What it does |
|---|---|---|
| `j`, `down` / `k`, `up` | `keys.scroll_down` / `keys.scroll_up` | move the cursor line |
| `pgdown`, `}` / `pgup`, `{` | `keys.page_down` / `keys.page_up` | a page down / up |
| `g` / `G` | `keys.top` / `keys.bottom` | first / last line |
| `v` | `keys.copy_select` | start / stop a line selection (moving extends it) |
| `y` | `keys.copy_yank` | copy the selected lines, or the cursor line (`ui.clipboard`) |
| `Y` | `keys.copy_all` | copy the whole text |
| `w` | `keys.wrap` | toggle wrap (kept when you leave) |
| `q`, `esc`, `H` | `keys.back` | back to the viewer, normal or fullscreen, same position |

## kubectl view (`U`)

| Key | Config | What it does |
|---|---|---|
| `ctrl+o` | `keys.output` | cycle the output: default, wide, yaml, json, name |
| `ctrl+j` | `keys.jq` | toggle the jq filter field |
| `tab` | `keys.complete` | complete the line |
| `↑` `↓` | | previous / next command you ran, earlier sessions too (last 100 in `~/.k2/kubectl-history`); `↓` past the newest brings back what you were typing |
| `esc`, `q` | `keys.shell_back` | leave the jq field / the kubectl view |

`~` and `~/` in file paths expand to your home directory before kubectl runs, e.g. `apply -f ~/secrets/master.yml` or `--kubeconfig=~/.kube/config`. Quoted values are left alone.

## Kustomize browser (`K`)

| Key | Config | What it does |
|---|---|---|
| `s` | `keys.scan` | scan the tree for kustomizations |
| `p` | `keys.kustomize_path` | type a directory to jump to (`tab` completes) |
| `A` | `keys.apply` | kubectl apply -k on a kustomization, kubectl apply -f on a plain YAML file (popup: path, context, object count). From a diff (`D`) the same key confirms and applies that kustomization without leaving the viewer first |
| `D` | `keys.diff` | kubectl diff -k, or kubectl diff -f on a plain YAML file. If the cluster is missing a namespace, a popup offers to create it and re-run (`y`; default no). The same offer appears after a failed apply |
| `ctrl+d` | `keys.kustomize_delete` | kubectl delete -k, or kubectl delete -f on a plain YAML file (red popup: path, context, object count) |
| `e` | `keys.edit` | open the file / kustomization in vim |
| `.` | `keys.open_folder` | open the folder in a new VS Code window: `code <dir>` (`editor.open_folder`), detached from k2; the browsed directory, or the selected kustomization after `s` |

## Forwards list (`f`)

| Key | Config | What it does |
|---|---|---|
| `ctrl+d` | `keys.delete` | stop and remove the selected forward (red popup: local → target:remote), also with other panes or tabs open |
| `D` | `keys.delete_all` | stop and remove all forwards (red popup) |
| `r` | `keys.pf_restart` | restart the selected forward (dead or alive): same ports and target; a pod that is gone falls back to its recorded owner, e.g. `deploy/web` |
| `a` | `keys.pf_auto` | automatic restart on / off for the selected forward. On the next start k2 restarts a dead forward only when this is on, or when the forward has no override and `portforward.auto_restart` is true |

## Dialogs and inputs

| Key | Config | What it does |
|---|---|---|
| `y`, `Y` | `keys.yes` | confirm popup: yes, run it |
| `n`, `N`, `esc`, `q` | `keys.no` | confirm popup: no, cancel (`enter` does nothing: the default is no) |
| `enter` | `keys.submit` | submit a popup / the port input |
| `esc` | `keys.cancel` | close a popup |
| `b` | — | theme popup (`ctrl+t`): switch the background between `transparent` and `theme`, previewed live; `enter` saves it to `ui.background` with the theme, `esc` restores both |
| `tab` / `shift+tab`, `up` / `down` | `keys.complete`, `keys.scroll_down`, `keys.scroll_up` | port popup: cycle suggestions |
| `ctrl+a` / `ctrl+e`, `ctrl+k`, `ctrl+u`, `ctrl+w`, `ctrl+v` | — | text inputs: line start / end, kill to end, kill to start, delete word, paste |

Every printable key is typed in a text input, including `q`, `Q`, `F` and
the jump keys.

## In-pane vim / shell

| Key | Config | What it does |
|---|---|---|
| any key | — | goes to vim / the shell |
| `ctrl+arrows`, `ctrl+a` arrows | `keys.pane_left` … | focus another pane (the editor keeps running; arrows after the prefix repeat) |
| `ctrl+a` then a pane / tab key | `keys.prefix` | k2's pane and tab keys (`ctrl+a n`, `ctrl+a x` …) |
| `Z`, `ctrl+a z` / `ctrl+a Q` | `keys.fullscreen` / `keys.quit` | zoom the pane / quit k2 |
| `ctrl+a ctrl+a` | `keys.prefix` | send a literal `ctrl+a` (shell: line start; vim: increment the number) |
| `q`, `esc` | `keys.back` | close the last screen of a failed edit / shell |
