# The mark

**Wood plus element** — the Holzcloud family: a wooden plank carries each
project's element. For holzkube-manager the element is the node, an isometric
cube in **Glut** `#EF7A4D`, with the annual rings of the Holzcloud mark cut into
its top face.

| File | Use |
|---|---|
| `holzkube-mark.svg` | the mark on its own, without a tile — README, banner, social card, anywhere it stands on its own |
| `../../web/public/favicon.svg` | the app icon: the mark on its dark tile `#140806` — the tab icon and the mark inside the app (sidebar, sign-in, setup) |
| `../../web/public/apple-touch-icon.png`, `icon-192.png`, `icon-512.png` | the home-screen icon: the mark on the tile colour `#140806`, opaque, inside the middle 64% so a round or squircle mask never cuts it |
| `banner.png` | the banner at the top of the README |
| `social.png` | the card GitHub shows when the repository is shared (Settings → Social preview), 1280×640 |

Glut `#EF7A4D` is the brand colour of the mark, the banner and the README
badges, and since 2026-10 also the accent of the interface: `--hc-brass` in
`web/src/index.css` holds `#EF7A4D` (the name is older than the colour), and on
light paper text and borders take the darker `#8F4B31`, which keeps the
contrast the brass-era ink had. The plank is wood: `#C98B4F`, lit `#E0A869`,
grain `#9B6534`.

The PNGs here, the home-screen icons and every picture in `../screenshots` are rendered, not drawn:

```sh
task build && node web/scripts/readme-images.mjs
```

They show the invented homelab from `web/fixtures/demo.json` and never a real
cluster — this repository is public.
