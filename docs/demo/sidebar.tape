Source "docs/demo/settings.tape"
Output "docs/demo/sidebar.gif"
Set TypingSpeed 0

Hide
Type "source docs/demo/env.sh && demo_reset && clear"
Enter
Sleep 1s
Type "k2"
Enter
Sleep 2s
Type "/checkout"
Enter
Sleep 600ms
Down
Sleep 300ms
Type "F"
Sleep 1s
Enter
Sleep 1s
Escape
Sleep 300ms
Up
Sleep 4.5s
Show

Sleep 3s

Hide
Type "Q"
Sleep 1s
Type "sed -i 's/^ui:$/ui:\n  sidebar_position: right/' .k2/config.yaml && clear && k2"
Enter
Sleep 2.5s
Show

Sleep 3s

Hide
Type "Q"
Sleep 1s
Type "sed -i 's/sidebar_position: right/sidebar_position: top/' .k2/config.yaml && clear && k2"
Enter
Sleep 2.5s
Show

Sleep 3s

Hide
Type "Q"
Sleep 1s
Type "sed -i 's/sidebar_position: top/sidebar_position: bottom/' .k2/config.yaml && clear && k2"
Enter
Sleep 2.5s
Show

Sleep 3s
