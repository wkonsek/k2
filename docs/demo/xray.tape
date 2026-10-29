# xray.gif: :xray with tab completion, a folded namespace tree, expand and
# collapse.
Source "docs/demo/settings.tape"
Output "docs/demo/xray.gif"

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

Type ":xray de"
Sleep 500ms
Tab
Sleep 1.2s
Enter
Sleep 2.5s
Type "j"
Sleep 500ms
Space
Sleep 1.5s
Type "+"
Sleep 2s
Type "jjjjjj"
Sleep 1s
Type "-"
Sleep 1.5s

Type ":xray deploy che"
Sleep 500ms
Tab
Sleep 900ms
Enter
Sleep 3s
Type "jjjjj"
Sleep 800ms
Space
Sleep 1.5s
Space
Sleep 1.5s
