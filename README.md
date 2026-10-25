<div align="center">

# k2

**kubectl, elevated.** A terminal UI for working with several Kubernetes clusters side by side.

[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![Go 1.24+](https://img.shields.io/badge/go-1.24%2B-00ADD8.svg)](go.mod)

</div>

k2 puts your clusters in tmux-style panes and tabs. Every pane has its own
kubeconfig, context and namespace, so a kind cluster, a staging cluster and
an OpenShift cluster can sit next to each other on one screen. Pane and tab
keys work like tmux behind a `ctrl+a` prefix, and vim, shells and
`kubectl edit` run *inside* a pane while the others keep working.

k2 is inspired by [k9s](https://k9scli.io/)
([github.com/derailed/k9s](https://github.com/derailed/k9s)), which does
cluster navigation in the terminal remarkably well. Many of its ideas, `:`
commands and short names (`:dp`, `:sts`, xray, `0` for all namespaces, `i`
to set an image) will feel familiar here. k2 adds splits, persistent
sessions and a few workflows of its own.

Under the hood k2 runs `kubectl` for everything, so it works with any
resource the cluster serves: built-in kinds, CRDs, OLM objects and OpenShift
Routes and Projects. When you quit, your shell lands on the cluster and
namespace of the focused pane.

<!-- GIF: docs/demo/hero.gif -->

## Contents

- [Highlights](#highlights)
- [Install](#install)
- [Quick start](#quick-start)
- [Reference](#reference)
  - [Views and keys](#views-and-keys)
  - [Port-forwards](#port-forwards)
  - [What k2 remembers](#what-k2-remembers)
  - [Configuration](#configuration)
  - [Themes and look](#themes-and-look)
  - [Token lifetime](#token-lifetime)
  - [Requirements](#requirements)
  - [Development](#development)
- [License](#license)

## Highlights

**Sessions that survive a restart.** Quit k2 and start it again: your tabs
(with their names), split layouts, each pane's kubeconfig, context and
namespace, the list or logs it showed, a stern view, your favorites and the
last Kustomize directory come back as you left them. Port-forwards are
detached `kubectl` processes that keep running after k2 exits; on the next
start k2 picks up the ones still alive and restarts the ones that died.

<!-- GIF: docs/demo/persistence.gif -->

**Splits, tabs and vim in a pane.** `ctrl+a %` splits right, `ctrl+a "`
splits down, `ctrl+a c` opens a tab, `ctrl+a z` zooms: the keys mean what
they mean in tmux. `e` (`kubectl edit`), `v` (any text) and `s` (a shell in
the pod) open inside the pane, and every pane can run its own editor.

<!-- GIF: docs/demo/splits.gif -->

**Self-healing port-forwards.** `ctrl+f` on a pod, service, deployment or
statefulset suggests ports from the spec. k2 does not trust a running
`kubectl port-forward` blindly: it watches kubectl's output and checks the
forwarded pods, so a forward whose pod went away in a rolling restart is
marked dead with the reason. `r` restarts it through its owner
(`pod gone → forwarding via deploy/kafka-connect`), and with
`portforward.auto_restart` k2 does it on its own.

<!-- GIF: docs/demo/port-forward.gif -->

**Kustomize browser.** `K` browses directories, marks kustomizations with a
green dot and diffs, applies or deletes them. Apply and delete show a
progress bar over the objects the kustomization renders to, every output
line gets an icon and a color (`+` created, `~` configured, `−` deleted,
`✗` error), and a bordered summary closes the run. Diffs get one bold header
per resource.

<!-- GIF: docs/demo/kustomize.gif -->

**Owner tree (`:xray`).** `X` on a row or `:xray deploy web` shows a
workload with everything it owns: Deployment → ReplicaSets → Pods →
containers, plus the services and ingresses in front of it and the config
maps, secrets and PVCs it uses, with status colors, READY and RESTARTS.
Namespace-wide trees start folded with a one-line summary per root, and
roots in trouble start open.

<!-- GIF: docs/demo/xray.gif -->

```
:xray deploy web
NAME                  KIND        STATUS            READY  RESTARTS  AGE
▾ web                 Deployment  Progressing       1/2              13h
├ ▾ web               Service     ClusterIP
│ └ web               Ingress
├ ▾ web-7d9f          ReplicaSet  Progressing       1/2              13h
│ ├ ▾ web-7d9f-a      Pod         CrashLoopBackOff  1/2    3         13h
│ │ ├ app             container   Running           1/1    3         13h
│ │ ├ side            container   CrashLoopBackOff  0/1    0         13h
│ │ ├ migrate (init)  container   Completed                0         13h
│ │ ├ web-config      ConfigMap
```

**Multi-pod logs.** `M` on a workload, service or pod follows the logs of
all its pods at once, stern-style, each line prefixed with a colored
`pod/container`. `:logs app=web --since=10m -c app` takes a label selector
or a pod-name pattern (not `kind name`); new pods are picked up as they
appear. `m` draws a rule after the current line so the tail stays separated.

<!-- GIF: docs/demo/multi-logs.gif -->

**Helm releases without the helm CLI.** `:helm` lists releases and shows
values, computed values, manifest, notes and a revision history, all
decoded by k2 from Helm's release secrets. Mark two revisions and diff them
per resource; roll back to any revision (this one step uses the helm CLI,
when installed).

<!-- GIF: docs/demo/helm.gif -->

**Safe on production.** List your production contexts under `prod:` and
their panes get a red border and ☠; every destructive action asks in a red
popup that names the target and context, and `enter` never confirms: only
`y` does. Separately, `ui.context_themes` gives a context its own theme, and
the whole UI re-colors live as you move between panes.

<!-- GIF: docs/demo/prod-safety.gif -->

**Token lifetime at a glance.** For OpenShift `oc login` tokens and JWTs the
sidebar shows how long the focused context's token is still valid, as a bar
that shrinks live and turns yellow and red before it expires.

<!-- GIF: docs/demo/token-bar.gif -->

**And the small things.** `:pulse` shows a namespace's health on one screen.
`b` pins favorite context + namespace (+ view) targets and jumps back with
`1`-`9`. `0` toggles all namespaces on every list. Pod lists have live CPU
and MEM bars against limits. `i` sets images, `S` scales, `r` restarts a
rollout, and OLM Subscriptions, CSVs and InstallPlans get their own views.

<!-- GIF: docs/demo/pulse-favorites.gif -->

## Install

k2 runs on Linux. It needs `kubectl` on `PATH`.

**From a package (linux/amd64).** Use a package from the
[releases page](https://github.com/wkonsek/k2/releases), or build it
with `make rpm` / `make deb` (`make dist` for both; the files land in
`dist/`):

```zsh
sudo dnf install ./k2-<version>-1.x86_64.rpm      # Fedora
sudo apt install ./k2_<version>_amd64.deb         # Ubuntu / Debian
```

The packages install `/usr/bin/k2`, the example config in
`/usr/share/k2/config.example.yaml`, and the README, cheatsheet, LICENSE
and NOTICE in `/usr/share/doc/k2/`. kubectl is recommended, not required, so
install it the way you like. Building them needs `rpmbuild`
(`sudo dnf install rpm-build`; `sudo apt install rpm` on Debian/Ubuntu) and
`dpkg-deb` (`sudo apt install dpkg-dev`; `sudo dnf install dpkg` on Fedora).
To update, install the newer package the same way.

**From source** (Go 1.24 or newer):

```zsh
git clone https://github.com/wkonsek/k2.git && cd k2
make install    # installs $(go env GOPATH)/bin/k2 (or: go install ./cmd/k2)
make build      # only builds bin/k2
```

To update, pull and run `make install` again.

**Shell function.** On exit k2 prints a single `export KUBECONFIG="..."`
line for the focused pane. Add this function to `~/.zshrc` (bash works the
same) so your shell evaluates it:

```zsh
k2() {
  if (( $# )); then
    command k2 "$@"
    return
  fi
  local out
  out="$(command k2)" || return
  if [[ $out == 'export KUBECONFIG='* && $out != *$'\n'* ]]; then
    eval "$out"
  fi
}
```

It evaluates only a clean exit: k2 must exit 0 and print exactly one
`export KUBECONFIG=...` line. With arguments (`k2 --version`, `k2 --help`,
`k2 --init-config`) k2 runs directly. If k2 fails, nothing is
evaluated; a crash restores the terminal, writes the stack trace to
`~/.k2/crash.log` and prints one line to standard error.

## Quick start

```zsh
k2
```

On the first start k2 writes a commented config to `~/.k2/config.yaml`.
Then:

| Key | |
|---|---|
| `c` / `C` / `n` | pick a context / kubeconfig / namespace |
| `:` | command line: `:pods`, `:dp`, `:helm`, `:xray deploy`, `:logs app=web`, `:pulse` … (`tab` completes) |
| `enter`, `d`, `e` | open (logs, YAML), describe, edit |
| `ctrl+f`, `f` | port-forward the selected row, list forwards |
| `K`, `U` | Kustomize browser, kubectl view |
| `ctrl+a %`, `ctrl+a "` | split right, split down |
| `ctrl+a c`, `ctrl+a n` | new tab, next tab |
| `ctrl+arrows` | move between panes |
| `0` | all namespaces and back |
| `q` / `esc` | back |
| `Q` | quit; your shell switches to the focused pane's cluster |

Every key is listed in the **[keyboard cheatsheet](CHEATSHEET.md)**.

## Reference

### Views and keys

The full key list, with the config name of each key, is in
[CHEATSHEET.md](CHEATSHEET.md). In short:

**Prefix.** Pane and tab keys come after `ctrl+a` (`keys.prefix`): press it,
let go, press one key. The status line shows `prefix ctrl+a …` and the
legend lists the keys while k2 waits (1.5 s, `esc` cancels). The prefix
works in tables, viewers and the in-pane vim / shell; `ctrl+a ctrl+a` sends
a literal `ctrl+a` there. In k2's own text inputs (`:` line, `/` filter,
kubectl line, path field, popups) `ctrl+a` keeps its editing meaning (line
start): press `esc` first or use `ctrl+arrows`. `keys.prefix` takes any
ctrl+, alt+ or function key, e.g. `ctrl+\`, which also works in inputs.

**Panes and tabs.** After the prefix, `"` `%` `c` `&` `,` `n` `p` `o` `;`
`z` `x`, the arrows and `1`-`9` mean what they mean in tmux, and k2's own
`h` (split down), `v` (split right), `t` (new tab) and `w` (close tab) work
too. Splits nest up to 4 panes per tab (`max_panes`). `ctrl+arrows` or the
prefix plus arrows move to the nearest pane; more arrows within 1 s keep
moving. `ctrl+a o` cycles panes, `ctrl+a ;` jumps back, `ctrl+a z` or `F`
zooms. `]` / `[` switch tabs on their own (inside inputs and vim use
`ctrl+a n` / `ctrl+a p`). `ctrl+a ,` renames a tab (empty restores the
automatic name). `ctrl+a x` and `ctrl+a &` ask before closing a pane or a
tab, in red when vim or a shell runs there. `ctrl+d` on the home screen
closes a pane without asking; the last pane closes the tab, and the only
tab quits, like `Q`. Off home, `ctrl+d` still deletes and still asks. The
tab bar appears from two tabs on (`ui.tabbar`) and marks a tab with a
production pane in red with ☠.

**Sidebar.** It never takes focus. It shows the focused pane's target,
cluster info, token lifetime and forwards; its entries open in the focused
pane with `C` `c` `n` `K` `U` `f` or `:kubeconfig` (`:kc`), `:context`
(`:ctx`), `:namespace` (`:ns`), `:kustomize` (`:k`), `:kubectl`,
`:forwards` (`:pf`, `:fwd`). On OpenShift the namespace picker lists
projects. Picking a namespace from a pod or resource list returns to that
list.

**Command line (`:`).** Any resource name the cluster reports works
(`:deployments`, `:subscriptions`, `:routes` …), plus your `aliases:` and
built-in k9s-style short names (`:dp`, `:sts`, `:ds`, `:cj`, `:ev`, `:sub`,
`:csv`, `:rt` …) unless your config or the cluster gives a name another
meaning. `tab` completes targets, resources, aliases, and for `:xray`,
`:logs` / `:stern` and `:fav` the kinds and names from the cluster. A kind
the cluster does not serve says `… is not installed on this cluster (no
OLM)` or `(not OpenShift)`.

**Every table.** Live refresh with a counting AGE column. `/` filters,
`>` / `<` sort by the next / previous column and `=` flips the direction,
`0` toggles all namespaces (the selected object stays selected; remembered
per pane), `pgdown` / `}` and `pgup` / `{` page, `w` / `W` copy the name /
`kind/name -n ns`, `L` / `A` label / annotate, `d` describes (warning
events colored), `e` edits, `ctrl+d` deletes, `enter` shows the YAML. STATUS
is colored by meaning; names turn the warning color only for real trouble
(not ready, Pending, CrashLoopBackOff, Terminating …); RESTARTS reads
`3 (5m ago)` and is red when the last restart was under an hour ago. Empty
lists say where to look: `no pods in shop — press 0 for all namespaces`.

**Pods.** `enter` follows the logs (`p` switches to the previous
container's log, `m` draws a separator under the current line), `s` opens a
shell in the pane (bash, else sh), `ctrl+f` port-forwards; pods with several
containers ask which one. CPU and MEM columns come from `kubectl top pods`
every `ui.metrics_every` seconds (15) and show `45m ███░░ 62%`: usage
against the summed limits, or the requests (`~62%`) when not every
container has a limit; yellow from `ui.metrics_warn` (70), red from
`ui.metrics_crit` (90). Without metrics-server they show `N/A` and the list
keeps working.

**Workloads.** `S` scales, `r` runs `kubectl rollout restart`, `i` sets
images: a popup lists every container and init container, and only the
changed ones are sent after a confirmation showing `old → new`. Jobs show
status, completions and duration (`enter` lists their pods); cronjobs show
the schedule and last run, and `t` runs one now. `:events` (`:ev`) lists
events oldest first.

**OLM and OpenShift.** `:sub` Subscriptions (`enter` jumps to the installed
CSV), `:csv` ClusterServiceVersions with a colored PHASE (`enter` shows what
the CSV owns), `:ip` InstallPlans (`a` approves a Manual plan), `:catsrc`
CatalogSources, `:og` OperatorGroups, `:crd` CRDs (`enter` lists their
custom resources), `:rt` Routes (HOST as a URL), `:proj` projects. Picking
an OpenShift context takes that context's namespace (the kubeconfig field,
or the first part of an `oc` name `namespace/cluster/user`).

**Owner tree (`X`, `:xray <kind> [name]`).** Kinds: deploy, sts, ds, rs,
job, cronjob, pod, svc. `space` folds a node, `+` / `-` expand / collapse
all, `/` filters by name keeping the path, `enter` jumps to the node's table
(a container opens its log) and back returns, `d` describes, `y` shows the
YAML, `M` follows the logs of every pod under the node.

**Multi-pod logs (`M`, `:logs`).** The pods of the selected workload or
service (its selector) or a pod's siblings, or `:logs app=web`,
`:logs "tier in (a,b)"`, `:logs ^web- --since=10m -c app --tail=50`.
`:logs sts kafka` is not a StatefulSet: without `=` it is a pod-name
pattern. Polled every `logs.multi_every` seconds (2), bounded by a ring
buffer of `logs.multi_max_lines` (5000). `/` filters (text or regex); `m`
marks the current line; across namespaces lines are prefixed
`namespace/pod/container`.

**Helm (`:helm`, `:helm all`).** NAME, REVISION, STATUS, CHART, APP VERSION,
UPDATED. `v` values, `V` computed values, `M` manifest, `N` notes, `t`
history as a table (`●` current revision). In the history `space` marks a
revision and `D` diffs it with another (or the selected one with the
current one), per resource plus user values. `r` rolls back (to the previous
revision from the list, to the selected one from the history) and `ctrl+d`
uninstalls; both need the helm CLI (`helm.binary`) and ask in a red popup.
k2 never installs or upgrades charts.

**Favorites (`b`, `:fav`).** `enter` or `1`-`9` jumps, `a` pins the focused
pane's context, namespace and view, `x` unpins, `K` / `J` reorder.
`favorites:` in the config adds fixed entries on top.

**Namespace pulse (`:pulse`).** Pods by phase with crash loops and
restarts, workloads ready of total, jobs, PVCs, CPU and memory against
limits, the objects in trouble and the newest warning events; refreshed
every `ui.refresh_every` seconds.

**Kustomize (`K`).** Every file and directory is listed (only `.git`,
`node_modules` and `vendor` are skipped); a green dot marks kustomizations,
non-YAML files are dimmed. `s` scans for kustomizations, `p` jumps to a
typed path with shell-like `tab` completion, `e` / `enter` opens a file in
vim. `a` apply, `D` diff, `d` delete act on green-dot rows; apply and
delete ask first, naming the path, context and object count. After a
delete k2 shows `waiting for N to terminate…`. The summary ends with
`── exit N · done|failed|differences ──`, and the cursor stays on the same
entry across runs and rescans.

**kubectl view (`U`).** Runs any kubectl line against the pane's target.
The prompt shows `◉ context · namespace` (red with ☠ on production) and
colors the line as you type; `tab` completes, `ctrl+o` cycles the output
(default, wide, yaml, json, name), `ctrl+j` adds a jq filter. The result
header shows exit code, outcome and duration.

**Text viewers** (logs, describe, YAML, events, command output). `w` toggles
wrap (`ui.wrap`), `/` searches, `g` / `G` go to top / bottom (`G` resumes
tailing), `v` opens the text in vim. `y` enters copy mode: only the text at
full screen, so the terminal's own mouse selection copies cleanly (k2 never
captures the mouse); there `v` selects lines, `y` copies them, `Y` copies
everything (`ui.clipboard`). The legend still shows `y` when the pane is
zoomed. Error and warning lines in command output always wrap in full, and
`!` opens the last error in full from anywhere.

**Confirmations.** Deletes, stopping forwards, Kustomize apply / delete,
closing panes and tabs with the prefix, rollout restart, cronjob trigger,
OLM approve, set image, Helm rollback / uninstall, and on production also
scale, label and annotate, ask in one centered popup with the full target.
Only `y` runs it; `n`, `esc` or `q` cancel and `enter` does nothing.

**In-pane vim and shells.** All keys go to the program except `ctrl+arrows`
and the prefix (`ctrl+a F` fullscreen, `ctrl+a x` close, `ctrl+a Q` quit …).
vim follows splits and resizes; `kubectl edit` uses `KUBE_EDITOR=vim`, and a
failed edit leaves its last screen for you to read. Quitting k2 kills all
editors. `ui.editor_inline: false` opens vim fullscreen instead.

### Port-forwards

- `ctrl+f` opens a popup with `local:remote` suggestions from the spec and
  matching services (`tab` / arrows cycle). If the local port is busy, k2
  offers to stop the holder and forward again (finding it uses `ss` or
  `lsof`).
- Each forward is a `kubectl port-forward` started detached (`setsid` +
  `nohup`), with its output in `~/.k2/logs/`. It keeps running after k2
  exits; quitting k2 does not stop it.
- `f` lists forwards with STATUS (`alive`, `restarting (3)`, `dead`), ports,
  target, namespace, context, age and REASON. `ctrl+d` stops one, `D` stops
  all; a stopped forward is removed and never restarted. The sidebar shows
  them too.
- Health: k2 reads kubectl's output for failure signals (`lost connection
  to pod`, `pods "x" not found`, `connection refused` …) and every
  `portforward.health_every` seconds (5, `0` = off) checks the forwarded
  pods. A forward whose pod is gone, terminating, finished or not ready is
  marked dead with the reason, an error toast and the full error under `!`.
- `r` restarts the selected forward with the same ports, kubeconfig,
  context and namespace. Service, deployment and statefulset targets are
  simply started again. For a pod, k2 records its top controller
  (Deployment, StatefulSet, DaemonSet, Job) when the forward is created and
  falls back to it when the pod is gone; for a forward without a recorded
  owner it asks the pod, or guesses a Deployment or StatefulSet from the
  pod name only when exactly one matches (`guessed owner
  deploy/kafka-connect`).
- `portforward.auto_restart: true` (off by default) does this by itself
  after `restart_backoff` seconds (1), doubling up to 30 s, at most
  `max_restarts` times in a row (10, `0` = no limit); 30 s alive resets the
  count. `↻` marks such forwards, and the TARGET column shows where a
  forward goes now (`deploy/kafka-connect → pod/kafka-connect-5d…`). `a`
  toggles it per forward.

### What k2 remembers

k2 saves its session to `~/.k2/state.json` (the directory is `state_dir`)
and restores it on the next start:

- the tabs, their names and split layouts, and the focused tab and pane;
- each pane's kubeconfig, context and namespace, and the namespace `0`
  returns to;
- what each pane showed: the pod list, a resource table, a pod's logs, a
  stern view (`:logs` / `M`, the same query, lines read again), the
  forwards list or a picker. Describe and YAML come back as their list.
  The owner tree, Helm and pulse come back as the pane's home screen, and
  back from a restored stern view goes home too;
- the port-forwards: on start, forwards whose process is still running are
  kept as they are, and dead ones are restarted with the same logic as `r`;
- the favorites you pinned and the last Kustomize directory.

A missing or broken state file never stops k2; it just starts fresh. The
theme and every other setting live in `~/.k2/config.yaml`.

On exit k2 also writes the focused pane's context and namespace into that
pane's kubeconfig (current-context) and prints
`export KUBECONFIG="<that kubeconfig>"` for the shell function.

### Configuration

Every key and UI constant is configurable in `~/.k2/config.yaml`.
[config.example.yaml](config.example.yaml) is the whole list: each setting
on one line, commented out, at its default. Uncomment a line to change it.
A bad value is skipped. k2 writes that file to `~/.k2/config.yaml` on its
first start if no config exists (a notice says `created ~/.k2/config.yaml`),
and never touches an existing file. If only `~/.k2/config` exists, k2 reads
that instead. The example is built into the binary and also shipped as
`/usr/share/k2/config.example.yaml`.

```zsh
k2 --init-config            # write ~/.k2/config.yaml if it is missing
k2 --init-config --force    # config exists: write ~/.k2/config.yaml.new beside it
k2 --check-config           # print every skipped line; exit 1 if there is one
```

A typo or bad value shows a warning and keeps the default for that entry. A
key that clashes with another action on the same screen is rejected the
same way. Hints in k2 always name the keys you configured. What each key
does is in [CHEATSHEET.md](CHEATSHEET.md).

**Production vs. a context's color** are two separate settings:

| | `prod:` | `ui.context_themes` |
|---|---|---|
| matches | exact context names | globs on the context name and/or kubeconfig path, first match wins |
| effect | red border, red context and ☠ in the pane title, red ☠ confirmations (also for scale, label, annotate), in every theme | the whole UI uses another theme while the focused pane is on that context |
| default | empty | empty (`theme:` everywhere) |

```yaml
prod: [prod-eu-1]
ui:
  context_themes:
    - {context: kind-kindest, theme: matrix}
    - {context: "*-prod*", theme: dracula}       # * also matches "/"
    - {kubeconfig: ~/.kube/lab.yaml, theme: nord}
```

The theme follows the focused pane and switches without a restart. One
window has one palette, so all panes show the focused pane's theme. An
unknown theme name or an entry without a context or kubeconfig is ignored
with a warning.

### Themes and look

| theme | style | |
|---|---|---|
| `nord` (default) | arctic blue-gray with frost cyan accents | dark |
| `dracula` | purple and pink on dark gray | dark |
| `catppuccin` | Mocha: pastel mauve on deep blue-gray | dark |
| `gruvbox` | retro yellow, orange warnings, on brown-gray | dark |
| `tokyo-night` | Night: blue and orange on deep navy | dark |
| `kanagawa` | Wave: ink blue and violet, after Hokusai | dark |
| `rose-pine` | Main: soft rose and iris on dark purple | dark |
| `everforest` | Dark medium: calm forest green on gray-green | dark |
| `one-dark` | Atom One Dark: blue and purple on charcoal | dark |
| `solarized-dark` | cyan and blue on deep teal | dark |
| `material` | Oceanic: teal on blue-gray | dark |
| `matrix` | digital rain: phosphor green on near black, red production | dark |
| `solarized-light` | blue on cream | light |
| `night-owl` | blue and gold on deep navy | dark |
| `ayu` | warm yellow on blue-gray | dark |
| `monokai` | green and pink on olive black | dark |
| `github-dark` | blue on near black | dark |
| `flexoki` | warm ink on near black | dark |
| `oxocarbon` | cool gray and cyan | dark |
| `poimandres` | ice blue on blue-black | dark |
| `horizon` | coral and peach on indigo | dark |
| `zenburn` | low-contrast cream on gray | dark |
| `catppuccin-latte` | mauve on warm paper | light |
| `custom` | your own `colors:` block | either |

```yaml
# ~/.k2/config.yaml (restart k2 to apply; ui.context_themes switch live)
theme: tokyo-night
# theme: custom
# colors:
#   bg: "#1a1b26"
#   muted: "#737aa2"
#   text: "#c0caf5"
#   accent: "#7aa2f7"
#   accent2: "#bb9af7"
#   accent3: "#ff9e64"
#   danger: "#f7768e"
#   ok: "#9ece6a"
#   warn: "#e0af68"
#   bright: "#ffffff"
ui:
  background: transparent   # transparent (default) | theme
  active_border: thick      # focused pane: normal (default) | thick | double
  dim_inactive: true        # mute the other panes' borders and titles (default false)
  logo: default             # default | simple
  tagline: "kubectl, elevated"   # any text, "" none
  logo_title_color: green   # the bold "k2": green | accent
```

- Colors are the themes' official ones (nord's dimmed gray is lightened for
  readability). The selected row has at least 4.5:1 contrast, dimmed text at
  least 3:1, and the production color is always a red.
- `background: transparent` keeps the terminal background; `theme` paints
  the whole screen. Light themes paint by default; forcing `transparent`
  on them shows a one-time hint.
- `active_border: thick` with `dim_inactive: true` makes the focused pane
  stand out best. A production pane stays red with ☠ either way.
- The sidebar shows `k2`, `kubectl`, `server` and (when readable) `ocp`
  versions of the focused context, fetched in the background
  (`ui.cluster_info: false` hides them). The logo and the cluster info are
  dropped first when the sidebar is short.
- Pane titles show the context next to the view (`1 kind-kindest ·
  all/pods`), plus the kubeconfig's short name when it is not
  `~/.kube/config`.
- The sidebar logo is `default` (the block mountain) or `simple`. The line
  under `k2` is `tagline`; leave it empty for none.

### Token lifetime

Under the cluster info the sidebar shows how long the focused context's
token stays valid: `token    ████████▌░░░ 5h12m`. The bar shrinks live
(computed locally, no API calls), turns yellow below `ui.token_warn` (25 %)
or under an hour, red below `ui.token_crit` (10 %) or under 15 minutes, and
says `token expired · run oc login` once it has expired.

| credential of the context's user | sidebar |
|---|---|
| OpenShift `oc login` token (`sha256~…`), OCP 4.6+ | bar from the user's own `useroauthaccesstokens` object |
| JWT bearer token with `exp` | bar from `exp` |
| token without a known expiry, client certificate, exec / auth-provider plugin | nothing |

For OpenShift k2 asks the cluster once in the background (5 s timeout), and
again when the context changes or the kubeconfig gets a new token, so after
`oc login` in another terminal the bar starts over by itself. When the
cluster sets `inactivityTimeoutSeconds`, k2 adds `idle limit 8h00m` under
the bar, since the token may end earlier. The token itself is never
printed, logged, stored or passed to kubectl; k2 keeps only a hash of it in
memory. `ui.token_bar: false` turns it off.

### Requirements

- Linux, `kubectl` on `PATH`; Go 1.24 or newer to build.
- Optional: `vim` (viewing and editing), `jq` (the kubectl view's filter),
  `ss` or `lsof` (finding what holds a local port), `helm` (only for Helm
  rollback and uninstall), metrics-server (CPU / MEM columns and pulse).

### Development

```
make build       bin/k2
make install     $(go env GOPATH)/bin/k2
make test        no cluster needed
make rpm / deb   packages in dist/
```

```
example.go          embeds config.example.yaml (written on first start)
cmd/k2/             main: runs the UI, prints the export line, --version, --init-config
internal/ui/        Bubble Tea model, views and key handling
internal/config/    ~/.k2/config.yaml, key bindings and key parsing
internal/theme/     color palettes
internal/layout/    the split tree of a tab
internal/state/     state.json
internal/kube/      kubectl-backed cluster access
internal/forward/   detached kubectl port-forward processes
internal/kustomize/ kustomize directory browsing and apply/diff/delete
internal/shell/     the command runner (real and fake, for tests)
packaging/          rpm spec and deb control
```

## License

k2 is written by Wojciech Konsek and licensed under the
[Apache License 2.0](LICENSE); see also [NOTICE](NOTICE).

k2 owes a lot to [k9s](https://github.com/derailed/k9s); if you have not
tried it, do.
