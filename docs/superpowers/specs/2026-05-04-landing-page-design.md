# DocsClaw Landing Page Design

**Date**: 2026-05-04
**Status**: Draft

## Problem

DocsClaw needs a public-facing site at docsclaw.dev to help
enterprise platform teams evaluate the agent runtime. Currently
there's no landing page — only a README and slide decks.

## Design Decisions

- **Static HTML** — self-contained files, no build step, matches
  skillimage.dev pattern
- **Site directory**: `/site/` (separate from `/docs/` which holds
  development docs)
- **Design system**: reuse skillimage.dev CSS (Red Hat fonts, dark
  theme, Red Hat accent colors)
- **GitHub Pages**: serve from `/site` directory on `main` branch
- **3 pages**: landing, getting started, demos

## Pages

### Landing (`site/index.html`)

Sections in order:

1. **Hero**: title "ConfigMap-Driven Agentic Runtime for
   OpenShift", subtitle, install command with copy button,
   buttons (GitHub, Get Started, Demos, Presentations), tags
   (A2A Protocol, OpenShift, Multi-Provider LLM, Tool Use)
2. **Why DocsClaw**: 6 feature cards
   - Lightweight (~5 MiB per pod)
   - A2A Protocol Native
   - ConfigMap Personality
   - Multi-Provider LLM (Anthropic, OpenAI, LiteLLM)
   - Tool-Use Agentic Loop (parallel execution)
   - Server-Side Sessions (multi-turn conversations)
3. **Quick Start**: code block showing build, serve, chat
4. **Architecture**: brief description of the agentic loop,
   ConfigMap-driven personality, provider registration
5. **Presentations**: links to existing slide decks
6. **Footer**: Pavel Anni, Office of CTO, Red Hat + nav links

### Getting Started (`site/getting-started.html`)

1. **Install**: Homebrew, Go install, container, GitHub releases
2. **First Agent**: walkthrough of config directory
   (system-prompt.txt, agent-card.json, agent-config.yaml)
3. **Run**: `docsclaw serve` + `docsclaw chat` with expected output
4. **Next Steps**: links to demos page and GitHub issues

### Demos (`site/demos.html`)

Gallery of demo agent configurations from `testdata/`:

- Standalone agent (general-purpose with tools)
- Executive assistant
- HR analyst
- Links to demo YAML files in `docs/demo/`

## CSS Design System

Reuse skillimage.dev CSS variables and patterns:

```text
--bg-primary: #000000
--bg-secondary: #1f1f1f
--bg-surface: #292929
--accent: #ee0000 (Red Hat red)
--accent-light: #f56e6e
--font-display: Red Hat Display
--font-body: Red Hat Text
--font-mono: Red Hat Mono
```

Components: hero, feature-card, code-block, install-card,
tags, buttons (primary/outline), footer.

## Files to create

| File | Description |
|------|-------------|
| `site/index.html` | Landing page |
| `site/getting-started.html` | Installation and first agent guide |
| `site/demos.html` | Demo scenarios gallery |

## Verification

- Open each HTML file in a browser, verify rendering
- Check responsive layout at mobile widths
- Verify all links work (GitHub, inter-page, slide decks)
- Compare visual consistency with skillimage.dev
