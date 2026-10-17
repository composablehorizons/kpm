---
name: create-readme
description: Create or substantially restructure an open-source project README, especially for a CLI. Use when choosing its sections, ordering, examples, and visual presentation; not for isolated factual corrections.
---

# Create README

Help a visitor understand the tool, see its result, try it, and find deeper
reference material. Lead with the user experience; put contributor setup later.

Inspect the existing README, commands, installation/release setup, and nearby
docs before writing. Use verified commands and capabilities. Preserve useful
existing content and links; do not invent distribution channels or features.

## Recommended structure

Use this as a starting order, adapting or omitting sections to fit the project.
Small CLIs can combine sections. Avoid empty headings and repetitive content.

1. **Identity:** project name and a one-sentence description of what it does and
   who it helps. Use a Markdown image placeholder for a future branded header
   when appropriate. Keep the essential description readable outside it too.
2. **Status and navigation:** a small set of relevant badges, such as release,
   build, and license, backed by real project resources. Add short navigation
   links or a contents list when the README is long enough to need them.
3. **Show the result:** a Markdown image placeholder near the top for a future
   screenshot, animation, or input-to-output graphic. Branding and product
   demonstration have different jobs; a logo does not explain behavior.
4. **Installation:** the recommended supported method first, then alternatives.
   Include prerequisites and platform support where needed. Distinguish using
   a released tool from building it for development.
5. **Quick start:** the shortest copyable example that accomplishes a useful
   task. Explain the expected result and where outputs appear. Supply or link
   required sample inputs so the example is usable.
6. **Features:** a concise list of concrete capabilities and useful defaults.
   Emphasize what helps someone decide whether the tool fits their task.
7. **Examples:** a few common workflows, each with a purpose, command, and
   expected effect. Start with ordinary use; put advanced variations later.
8. **Options and integration:** document key flags, defaults, units, and
   configuration. Show how outputs fit the user's workflow. Link to `--help`
   or detailed docs rather than reproducing an entire manual.
9. **Limitations and troubleshooting:** explain relevant boundaries, surprising
   defaults, and common failures with actionable fixes. Comparisons or
   benchmarks belong here only when they help a real selection decision and
   have evidence.
10. **Contributing and license:** link contribution guidance and the license.
    Include concise build/test instructions if they are not already maintained
    elsewhere. Add support or security-reporting links when the project has them.

A brief demo or first example can precede installation when it explains the
value more clearly. The structure is a synthesis, not a universal standard.

## Visuals and final pass

- Do not generate, draw, capture, download, or create graphics as part of this
  skill. Leave Markdown image slots with empty destinations instead, such as
  `![Project header]()` or `![Source artwork becoming Android and iOS icons]()`.
  Do not invent asset paths or invoke image-generation or screenshot tools.
- Use descriptive alt text to explain what belongs in each slot. Prefer one
  useful demonstration slot to a gallery of decorative placeholders. Preserve
  existing user-supplied image references when they remain relevant.
- Verify commands against the current CLI and check paths, links, image assets,
  supported installation methods, and claims. Review the rendered Markdown
  when changing visual layout. Remove repetition and move lengthy platform or
  advanced reference material into linked docs when that improves navigation.

## Reference examples

This structure was distilled from [Gum](https://github.com/charmbracelet/gum),
[bat](https://github.com/sharkdp/bat), [fzf](https://github.com/junegunn/fzf),
[fd](https://github.com/sharkdp/fd),
[ripgrep](https://github.com/BurntSushi/ripgrep), and
[GitHub CLI](https://github.com/cli/cli). They illustrate different balances of
branding, demos, examples, and reference depth; do not copy their full complexity
into a smaller project or assume their current installation commands apply.
