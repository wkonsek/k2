# multi-logs.gif: M follows every pod of a deployment, stern-style.
Source "docs/demo/settings.tape"
Output "docs/demo/multi-logs.gif"

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

Type ":deploy"
Enter
Sleep 1.5s
Type "jj"
Sleep 500ms
Type "M"
Sleep 4s
Type "m"
Sleep 3s
Type "q"
Sleep 800ms

Type ":logs app=cart"
Sleep 400ms
Enter
Sleep 3.5s
Type "/"
Sleep 300ms
Type "warn"
Enter
Sleep 3s
