# helm.gif: releases, history, a diff of two revisions, rollback.
Source "docs/demo/settings.tape"
Output "docs/demo/helm.gif"

Hide
Type "source docs/demo/env.sh && demo_reset && clear"
Enter
Sleep 1s
Show

Sleep 400ms
Type "k2"
Sleep 300ms
Enter
Sleep 2s

Type ":helm"
Enter
Sleep 1.5s
Type "0"
Sleep 1.5s
Type "jjj"
Sleep 600ms
Type "v"
Sleep 2s
Type "q"
Sleep 600ms
Type "t"
Sleep 1.8s
Type "j"
Sleep 500ms
Space
Sleep 800ms
Type "D"
Sleep 2.5s
Type "jjjjjj"
Sleep 1.5s
Type "q"
Sleep 800ms
Type "r"
Sleep 2.5s
Enter
Sleep 800ms
Type "y"
Sleep 3s
