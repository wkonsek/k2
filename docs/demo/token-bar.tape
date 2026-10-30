Source "docs/demo/settings.tape"
Output "docs/demo/token-bar.gif"

Hide
Type "source docs/demo/env.sh && demo_reset && fakekube token kind-dev 18s 40s && clear"
Enter
Sleep 1s
Show

Sleep 300ms
Type "k2"
Sleep 300ms
Enter
Sleep 3s

Ctrl+A
Type "%"
Sleep 700ms
Type "c"
Sleep 600ms
Type "j"
Enter
Sleep 2.5s
Ctrl+A
Type '"'
Sleep 700ms
Type "c"
Sleep 600ms
Type "jj"
Enter
Sleep 2.5s

Ctrl+A
Left
Sleep 9s
Ctrl+L
Sleep 2.5s
Escape
Sleep 1s
