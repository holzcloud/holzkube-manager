# The mark

**Ringwabe** — annual rings cut into a hexagon. The hexagon is the node; the
rings are the Holzcloud mark, the same figure its CMS carries in a cloud. The
rings sit slightly off centre, the way they do in a trunk that did not grow
evenly.

| File | Use |
|---|---|
| `holzkube-mark.svg` | the mark with three rings — README, project page, anywhere it stands on its own |
| `../../web/public/favicon.svg` | the tab icon and the mark inside the app: **two** rings, because at 16 pixels the third is a smear |
| `../../web/public/apple-touch-icon.png`, `icon-192.png`, `icon-512.png` | the home-screen icon: the small mark on the ground colour, opaque, inside the middle 64% so a round or squircle mask never cuts it |
| `banner.png` | the banner at the top of the README |
| `social.png` | the card GitHub shows when the repository is shared (Settings → Social preview), 1280×640 |

Colours are the interface's own: brass `#F0AE5F` cut by ink `#150E08`, on the
ground `#0A0705`.

The PNGs here, the home-screen icons and every picture in `../screenshots` are rendered, not drawn:

```sh
task build && node web/scripts/readme-images.mjs
```

They show the invented homelab from `web/fixtures/demo.json` and never a real
cluster — this repository is public.
