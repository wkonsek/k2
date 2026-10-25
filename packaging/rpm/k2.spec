# k2 rpm for Fedora (x86_64), built by `make rpm` from the binary that
# `make build-linux` already compiled (dist/k2-linux-amd64): this spec
# compiles nothing. The Makefile passes:
#   --define "k2_version 0.6.0"   from the git tag (v stripped)
#   --define "k2_src /path/to/k2" the repository root
# kubectl is not required: people install it in many ways (dnf, the
# Kubernetes repo, a download, asdf ...), so it is only recommended.

%global debug_package %{nil}
%global __strip /bin/true

Name:           k2
Version:        %{k2_version}
Release:        1
Summary:        Terminal UI for working with several Kubernetes clusters side by side
License:        Apache-2.0
ExclusiveArch:  x86_64
Recommends:     kubectl
Suggests:       vim-enhanced
Suggests:       jq

%description
k2 is a keyboard-driven terminal UI around kubectl: several panes and tabs,
each with its own kubeconfig, context and namespace, tables, logs, describe,
port-forwards, kustomize diff/apply and an in-pane vim or shell.

On its first start k2 writes the commented example configuration to
~/.k2/config.yaml (the package itself writes nothing into home
directories); the same file is in /usr/share/k2/config.example.yaml.

%install
install -Dm0755 %{k2_src}/dist/k2-linux-amd64 %{buildroot}%{_bindir}/k2
install -Dm0644 %{k2_src}/config.example.yaml %{buildroot}%{_datadir}/k2/config.example.yaml
install -Dm0644 %{k2_src}/README.md %{buildroot}%{_docdir}/k2/README.md
install -Dm0644 %{k2_src}/CHEATSHEET.md %{buildroot}%{_docdir}/k2/CHEATSHEET.md
install -Dm0644 %{k2_src}/LICENSE %{buildroot}%{_docdir}/k2/LICENSE
install -Dm0644 %{k2_src}/NOTICE %{buildroot}%{_docdir}/k2/NOTICE

%files
%{_bindir}/k2
%dir %{_datadir}/k2
%{_datadir}/k2/config.example.yaml
%dir %{_docdir}/k2
%{_docdir}/k2/README.md
%{_docdir}/k2/CHEATSHEET.md
# The license files sit in /usr/share/doc/k2 (not %{_licensedir}), still
# marked %license so rpm treats them as such.
%license %{_docdir}/k2/LICENSE
%license %{_docdir}/k2/NOTICE
