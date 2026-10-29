# hero.gif: start k2, browse pods, split, a quick tour.
Source "docs/demo/settings.tape"
Output "docs/demo/hero.gif"

Hide
Type "source docs/demo/env.sh && demo_reset && clear"
Enter
Sleep 1s
Show

Sleep 400ms
Type "k2"
Sleep 300ms
Enter
Sleep 2.5s

Type "j"
Sleep 350ms
Type "j"
Sleep 350ms
Type "j"
Sleep 900ms
Type "d"
Sleep 2s
Type "q"
Sleep 600ms

Ctrl+A
Type "%"
Sleep 1.2s
Type "c"
Sleep 900ms
Type "j"
Sleep 400ms
Enter
Sleep 1.2s
Enter
Sleep 2s

Type ":"
Sleep 300ms
Type "deploy"
Sleep 300ms
Enter
Sleep 1.5s

Ctrl+A
Left
Sleep 1.2s
Ctrl+A
Type "z"
Sleep 2.5s
Ctrl+A
Type "z"
Sleep 1.5s
