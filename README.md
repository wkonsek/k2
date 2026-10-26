# k2 — kubectl, but Bjutiful 🙈

Kurwa Man.

I was using kubectl for so many years.

And of coz every time the same shit.

You have 5 clusters.  
10 namespaces.  
Some production where you should not even touch anything.  
And 9000 terminals opened.

You switch context.

You forgot which one.

You delete something.

Boom 💥

Production gone.

Bjutiful 🤦‍♂️

So I made k2.

**kubectl, elevated.**

Or as I would say, kubectl but without all this shity switching and remembering what the fuck you were doing.

## What is ziz?

k2 is terminal UI for Kubernetes.

Like k9s.

But but but.

With multiple clusters in one screen.

Tmux style.

Each pane has own kubeconfig, context and namespace.

You can have kind, staging, OpenShift and production at the same time.

And you can split them.

And have tabs.

And use vim inside.

And when you close this shit, it remembers what you were doing.

Rili? 🙈

Yes.

Under the hood it is using kubectl.

Not inventing another Kubernetes implementation, because of coz we already have enough crap.

CRDs, OLM, OpenShift Routes, Projects, all this stuff works if cluster supports it.

## Why another tool?

I know.

We have k9s.

And k9s is veri nice.

I am not saying it is shit.

But I wanted multiple clusters side by side.

And sessions that survive restart.

And port forwards that are not dying every time some idiot restarts pod.

And some other things.

So instead of doing something useful in my garden, I started another project.

My wife is veri happy about this.

🙈

## Features

### 1. Multiple clusters — one circus 🎪

You press `ctrl+a %`.

Another pane appears.

`ctrl+a "` — split down.

`ctrl+a c` — new tab.

`ctrl+a z` — zoom.

Like tmux.

Each pane is working with its own cluster.

You can even open vim in one and kubectl logs in another.

And everything works independently.

Amazing.

I mean hopefully.

😅

### 2. Persistent sessions

This one is important.

You close k2.

Go sleep.

Or babysitting.

Or working in garden because of coz you have some duties.

Next day you open k2.

And everything is there.

Tabs. Splits. Contexts. Namespaces. Favorites. Logs. Kustomize directory.

More less everything you left.

No need to configure this crap again.

State lives in `~/.k2/state.json`.

If this file is broken, k2 starts fresh.

No drama.

### 3. Port forwarding that understands life is shit

You know this situation?

You forward some port.

It works.

Someone deploys new version.

Pod dies.

And your port forward is dead.

Kurwa.

So k2 is watching this.

Press `ctrl+f` on pod, service, deployment or statefulset.

It suggests ports from spec.

It can detect when forwarded pod is gone.

It can find its owner.

And restart forwarding via deployment instead.

With `portforward.auto_restart: true` it can even do this automatically.

Of coz until someone breaks entire cluster.

Then I can't help you.

🤦‍♂️

### 4. Kustomize — Ansibul style, but different

Press `K`.

You get browser for directories and Kustomize.

Green dot means there is kustomization.

You can diff.

Apply.

Delete.

See progress.

`+` created.  
`~` configured.  
`−` deleted.  
`✗` fucked up.

I mean error.

🙈

There is summary at the end so you know what happened.

No need to read 9000 lines of kubectl output.

### 5. Xray — see who owns this shit

Press `X` or type:

```text
:xray deploy web
```

You get tree.

Deployment.

ReplicaSets.

Pods.

Containers.

Services. Ingresses. ConfigMaps. Secrets. PVCs.

Everything connected together.

With statuses, readiness and restarts.

So when some pod is dying you can see the entire family of this disaster.

Veri cool.

### 6. Logs from multiple pods

Man.

You have 10 pods.

And some shit is happening.

Which pod?

No idea.

So press `M`.

k2 follows logs from all pods together.

Like stern.

Every line has pod and container name.

You can also do:

```text
:logs app=web --since=10m -c app
```

New pods are included when they appear.

Press `m` to separate current logs from new ones.

Finally something useful.

After 7 years of Kafka.

🙈

### 7. Helm without Helm

Huhuhu.

You can list Helm releases with:

```text
:helm
```

See values.

Computed values.

Manifests.

Notes.

History.

Compare two revisions.

And see what changed.

All read from Helm release secrets.

You don't even need Helm CLI for reading.

But for rollback and uninstall you still need Helm installed.

I am not magician.

Also k2 does not install or upgrade charts.

### 8. Production safety ☠

This one is for idiots.

I mean for all of us.

You can configure:

```yaml
prod: [prod-eu-1]
```

And now production has RED border.

And skull.

☠

When you try to delete something, it asks you.

With name of target.

And context.

And red popup.

You need to press `y`.

Not Enter.

Because Enter is too easy and people are idiots.

Including me.

You can also configure different UI themes for different contexts.

So you know where you are before destroying something.

Bjutiful 🙈

### 9. Token lifetime

OpenShift tokens.

JWT.

You know those shity tokens expiring during work?

k2 shows remaining lifetime in sidebar.

Green.

Yellow.

Red.

Expired.

Then you go do `oc login` again.

Because of coz.

It does not print or store your token.

Only tracks necessary lifetime information.

### 10. Other crap

There are also some smaller things:

- `:pulse` — namespace health summary
- `b` — save favorite context and namespace
- `1`–`9` — jump to favorites
- `0` — all namespaces
- `i` — change container image
- `S` — scale workload
- `r` — restart rollout
- `U` — run kubectl command
- `:sub`, `:csv`, `:ip` — OLM stuff
- CPU and memory usage for pods, if metrics are available

And many more things.

I have no time to describe everything.

Need to finish my fence.

🙈

## Installation

Linux only.

And you need kubectl on PATH to actually operate clusters.

### From packages

For Fedora:

```bash
sudo dnf install ./k2-<version>-1.x86_64.rpm
```

For Ubuntu/Debian:

```bash
sudo apt install ./k2_<version>_amd64.deb
```

Get packages from GitHub releases.

Or build with `make rpm`, `make deb`, `make dist`.

### From source

You need Go 1.24+.

```bash
git clone https://github.com/wkonsek/k2.git
cd k2
make install
```

Or:

```bash
make build
```

Now you have `bin/k2`.

Congratulations.

You installed another tool you will probably use instead of doing actual work.

🤣

## Quick start

Run:

```bash
k2
```

First start creates config in:

```text
~/.k2/config.yaml
```

Then you can start playing.

| Key | What it does |
|---|---|
| `c` | Change context |
| `C` | Change kubeconfig |
| `n` | Change namespace |
| `:` | Commands |
| `enter` | Logs or YAML |
| `d` | Describe |
| `e` | Edit |
| `ctrl+f` | Port forward |
| `K` | Kustomize |
| `U` | Kubectl |
| `ctrl+a %` | Split right |
| `ctrl+a "` | Split down |
| `ctrl+a c` | New tab |
| `0` | All namespaces |
| `Q` | Quit |

The full list is in [CHEATSHEET.md](CHEATSHEET.md).

Of coz nobody will read it.

They will ask me.

How do I change namespace?

🤦‍♂️

## Shell integration

One cool thing.

When you quit k2, it can switch your shell to the kubeconfig and context you were using in focused pane.

Add this to `.zshrc` or bash config:

```bash
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

So you close k2 and continue with kubectl.

If k2 crashes, it writes crash information to:

```text
~/.k2/crash.log
```

And it does not evaluate the shell export.

At least something is safe.

## Configuration

Everything can be configured.

And when I say everything I mean so many settings that I don't even remember all of them.

Keyboard shortcuts.

Themes.

Production contexts.

Port forwards.

Metrics.

Refresh intervals.

Logs.

Look at:

```text
config.example.yaml
```

To check config:

```bash
k2 --check-config
```

To initialize:

```bash
k2 --init-config
```

If you put wrong value, k2 warns and uses default.

Because I know how people are configuring things.

🙈

## Themes

You can use Nord, Dracula, Catppuccin, Gruvbox, Tokyo Night, Matrix and many more.

Even custom colors.

I don't know why we need so many themes for looking at broken pods.

But of coz developers need nice colors while production is dying.

You can configure for example:

```yaml
theme: tokyo-night

ui:
  active_border: thick
  dim_inactive: true
```

Production stays red with skull.

No matter which beautiful color circus you selected.

## Requirements

- Linux
- kubectl
- Go 1.24+ if building from source

Optional stuff:

- vim — for editing
- jq — for filtering
- helm — for rollback and uninstall
- ss or lsof — for local ports
- metrics-server — for CPU and memory

Without some optional tools, related features will not work.

Surprise.

## Development

```bash
make build
make test
make install
```

Tests do not require cluster.

At least I don't need to destroy production every time I change something.

Source is mostly under:

```text
cmd/k2/
internal/ui/
internal/config/
internal/theme/
internal/layout/
internal/state/
internal/kube/
internal/forward/
internal/kustomize/
internal/shell/
```

It's written in Go.

Some parts are probably shity.

But but but.

It works.

And if you find problem, you can fix it.

Or ask AI.

I am doing everything with AI recently anyway.

World changed man.

🙈

## License

Apache License 2.0.

Created by Wojciech Konsek.

A lot of inspiration comes from k9s.

Which is great tool.

Just missing some things I wanted.

So now we have k2.

## Final words

Man.

I was supposed to do some actual work.

Instead I created another Kubernetes tool.

My garden is not finished.

My fence is not finished.

There are still tomatoes in greenhouse.

And I am debugging some shity terminal UI.

Bjutiful life.

But at least now I can watch 4 broken Kubernetes clusters at the same time.

And maybe restart something.

Without changing terminal 9000 times.

**Give it a try for 3 days you will see kurwa.** 🙈

If it works, great.

If not, create issue.

If you are from Audi OPS team, please don't.

I have already enough shit to handle.

Maj frient 🤗💪
