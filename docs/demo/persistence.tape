# persistence.gif: forwards outlive k2, and the whole session comes back.
Source "docs/demo/settings.tape"
Output "docs/demo/persistence.gif"
Set TypingSpeed 45ms

Hide
Type "source docs/demo/env.sh && demo_reset && clear"
Enter
Sleep 1s
Show

Type "k2"
Sleep 300ms
Enter
Sleep 1.5s

Type "/web"
Sleep 300ms
Enter
Sleep 600ms
Type "F"
Sleep 1s
Down
Sleep 500ms
Enter
Sleep 900ms

Ctrl+A
Type "%"
Sleep 900ms
Type "c"
Sleep 700ms
Type "j"
Sleep 300ms
Enter
Sleep 700ms
Enter
Sleep 1.1s
Type "F"
Sleep 900ms
Down
Sleep 400ms
Enter
Sleep 900ms

Type "Q"
Sleep 900ms
Type `pgrep -af port-forward | cut -d' ' -f1-7`
Sleep 300ms
Enter
Sleep 2.5s

Type "k2"
Sleep 300ms
Enter
Sleep 2.5s
Type "f"
Sleep 2.5s
