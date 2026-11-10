# Component sources

The controls in this folder follow the selections recorded in
[docs/14-component-foundation.md](../../../../../docs/14-component-foundation.md).
They are built on [Radix UI](https://www.radix-ui.com/) primitives (MIT) and adapt the
visual treatment of free components from [Shadcn Space](https://shadcnspace.com/components),
used under the [Shadcn Space license](https://shadcnspace.com/license), which permits
commercial use of its free materials and asks that this notice be kept.

| Here | Adapted from | What changed |
| --- | --- | --- |
| Section tabs in `App.tsx` | Tabs 05 (Underline), Radix variant | Kept the sliding underline; removed the sliding hover pill, icons and the 48 px content slide (motion guide: no spring, no spatial page transitions); real tab panels that keep their state. |
| `form.tsx` Select | Select 01 | Our tokens; no zoom-in animation. |
| `form.tsx` Switch + Label | Switch 05 (with label) | Our tokens. |
| `dialog.tsx` | Dialog 01 | Plain title/description/actions; no icon medallion or slide-in. |
| `menu.tsx` | Context Menu 01 | Rebuilt on Radix ContextMenu/DropdownMenu so it works from the keyboard; kept the rounded panel and separated destructive item; removed staggered item entrance. |
| `../ImportDialog.tsx` | File Upload 01 | Kept the drop area and file list; native drag-and-drop instead of `react-dropzone`; real validation and per-file results. |

Not taken from a library: the wallpaper grid (a fixed-density grid; the Space UI
Adaptive Density Grid reduces to this once its density controls and reflow
animation are turned off, as the foundation document asks) and the full-size
viewer (Radix Dialog; the Planes Photo Lightbox is a paid component that has
not been purchased).
