# kustomize.gif: the Kustomize browser, a diff, and apply / delete with
# their output streamed line by line under the progress bar.
Source "docs/demo/settings.tape"
Output "docs/demo/kustomize.gif"

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

Type "K"
Sleep 1.5s
Type "p"
Sleep 800ms
Type "shop/ov"
Sleep 400ms
Tab
Sleep 900ms
Enter
Sleep 1.5s
Type "jj"
Sleep 700ms

Type "D"
Sleep 2s
Type "g"
Sleep 1.5s
Type "jjjjjj"
Sleep 1.5s
Type "q"
Sleep 700ms

Type "a"
Sleep 2.2s
Type "y"
Sleep 5s
Type "q"
Sleep 900ms

Type "d"
Sleep 2s
Type "y"
Sleep 9s
