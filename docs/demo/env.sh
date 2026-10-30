
K2_DEMO_REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
export K2_DEMO_DIR="${K2_DEMO_DIR:-/tmp/k2-demo}"
export HOME="$K2_DEMO_DIR/home"
export PATH="$K2_DEMO_DIR/bin:/usr/local/bin:/usr/bin:/bin"
unset KUBECONFIG HISTFILE NO_COLOR FORCE_COLOR CLICOLOR CLICOLOR_FORCE
export HISTFILE=/dev/null
export TERM=xterm-256color COLORTERM=truecolor LANG=C.UTF-8 LC_ALL=C.UTF-8
export EDITOR=vim
PS1='\[\e[38;5;111m\]~\[\e[0m\] \[\e[1;32m\]❯\[\e[0m\] '
PROMPT_COMMAND=

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

demo_reset() {
  pkill -f -- "--kubeconfig $K2_DEMO_DIR/home/.kube/config" 2>/dev/null
  rm -rf "$K2_DEMO_DIR/home" "$K2_DEMO_DIR/world.json" "$K2_DEMO_DIR/unknown.log"
  mkdir -p "$HOME/.k2" "$HOME/deploy"
  cp "$K2_DEMO_REPO/docs/demo/config.yaml" "$HOME/.k2/config.yaml"
  cp -r "$K2_DEMO_REPO/docs/demo/kustomize/." "$HOME/deploy/"
  rm -f "$HOME/deploy/README.md"
  printf 'syntax on\nset number\nset laststatus=2\nset background=dark\n' > "$HOME/.vimrc"
  fakekube init
  if [[ $1 != fresh ]]; then
    local kc="$HOME/.kube/config"
    cat > "$HOME/.k2/state.json" <<JSON
{"version": 2, "kubeconfig": "$kc", "context": "kind-dev", "namespace": "shop", "focus": 0,
 "panes": [{"stage": "workpods", "kubeconfig": "$kc", "context": "kind-dev", "namespace": "shop"}],
 "tabs": [{"layout": {"pane": 0}, "focus": 0}], "tab": 0, "forwards": [], "kustomRoot": "$HOME/deploy"}
JSON
  fi
  cd "$HOME" || return
}
