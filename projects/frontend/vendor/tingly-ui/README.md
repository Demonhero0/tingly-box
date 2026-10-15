# @tingly/ui (vendored)

A copy of Tingly Box's `frontend/packages/ui`: the theme, icons and page
primitives Tingly Box is built from. Do not edit it here; change it in
tingly-box and refresh the copy with `task ui:sync`.

It is a copy only for the transition. Imports already use the package name
(`@tingly/ui`, `@tingly/ui/theme`, `@tingly/ui/icons`), so moving to the real
package later changes the dependency line in `package.json`, not the code.
