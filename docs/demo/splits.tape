# splits.gif: tmux keys for splits, tabs and zoom, and vim inside a pane.
Source "docs/demo/settings.tape"
Output "docs/demo/splits.gif"

Hide
Type "source docs/demo/env.sh && demo_reset && clear"
Enter
Sleep 1s
Show

Type "k2"
Sleep 300ms
Enter
Sleep 1.5s

Ctrl+A
Type "%"
Sleep 900ms
Type ":"
Type "deploy"
Enter
Sleep 1.2s
Ctrl+A
Type '"'
Sleep 900ms
Enter
Sleep 1.2s
Type "/web"
Enter
Sleep 400ms
Enter
Sleep 1.2s

Ctrl+A
Up
Sleep 700ms
Type "jj"
Sleep 500ms
Type "e"
Sleep 1.5s
Type "jjjj"
Sleep 800ms
Ctrl+A
Type "z"
Sleep 1.5s
Ctrl+A
Type "z"
Sleep 600ms
Type ":q"
Sleep 300ms
Enter
Sleep 1.2s

Ctrl+A
Type "c"
Sleep 600ms
Ctrl+A
Type ","
Sleep 600ms
Ctrl+U
Type "staging"
Enter
Sleep 600ms
Type "c"
Sleep 600ms
Type "j"
Enter
Sleep 800ms
Enter
Sleep 1.2s
Ctrl+A
Type "n"
Sleep 1.8s
