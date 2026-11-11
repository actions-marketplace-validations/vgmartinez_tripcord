# Badge

A repository can show that it was scanned with Tripcord. The badge is a
static shields.io image. It does not call Tripcord, and the `tripcord`
binary does not fetch it. There is no tracking endpoint.

The image links to this repository.

## Markdown

```markdown
[![Scanned with Tripcord](https://img.shields.io/badge/Scanned%20with-Tripcord-orange)](https://github.com/vgmartinez/tripcord)
```

## HTML

```html
<a href="https://github.com/vgmartinez/tripcord"><img alt="Scanned with Tripcord" src="https://img.shields.io/badge/Scanned%20with-Tripcord-orange"></a>
```

The scan itself is the [GitHub Action](github-action.md) or
`tripcord scan`. The badge does not report a result.
