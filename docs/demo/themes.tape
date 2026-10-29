# themes.gif: ctrl+t previews every theme as the cursor moves.
Source "docs/demo/settings.tape"
Output "docs/demo/themes.gif"
Set TypingSpeed 0

Hide
Type "source docs/demo/env.sh && demo_reset && clear"
Enter
Sleep 1s
Type "k2"
Enter
Sleep 2s
Ctrl+A
Type "%"
Sleep 300ms
Type ":xray deploy checkout"
Enter
Sleep 1.5s
Ctrl+A
Left
Sleep 500ms
Show

Sleep 1s
Ctrl+T
Sleep 1s
Down@750ms 22
Sleep 1s
Escape
Sleep 1.5s
