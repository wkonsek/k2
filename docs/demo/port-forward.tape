# port-forward.gif: a forward whose pod goes away is marked dead with the
# reason, and r brings it back through the deployment.
Source "docs/demo/settings.tape"
Output "docs/demo/port-forward.gif"

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

Type "/web"
Sleep 300ms
Enter
Sleep 600ms
Ctrl+F
Sleep 1.2s
Down
Sleep 500ms
Enter
Sleep 1s
Type "f"
Sleep 1.5s
Type "a"
Sleep 1.2s

Ctrl+A
Type "%"
Sleep 800ms
Enter
Sleep 1s
Type "/web"
Sleep 300ms
Enter
Sleep 600ms
Ctrl+D
Sleep 1.5s
Type "y"
Sleep 4s

Ctrl+A
Left
Sleep 1.5s
Type "r"
Sleep 3.5s
