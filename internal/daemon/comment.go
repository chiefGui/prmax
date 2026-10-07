package daemon

import (
	"fmt"
	"strings"

	"prmax/internal/model"
)

func renderComment(r model.Review) string {
	var b strings.Builder
	if r.Kind == model.KindIncremental {
		fmt.Fprintf(&b, "### prmax re-review · `%s` (since `%s`) · %s\n\n", short(r.HeadSHA), short(r.SinceSHA), r.ModelLabel)
	} else {
		fmt.Fprintf(&b, "### prmax review · `%s` · %s\n\n", short(r.HeadSHA), r.ModelLabel)
	}
	if s := strings.TrimSpace(r.Summary); s != "" {
		b.WriteString(s + "\n\n")
	}
	if len(r.Previous) > 0 {
		b.WriteString("**Previous findings**\n\n")
		for _, p := range r.Previous {
			icon := map[string]string{"fixed": "✅", "open": "⚠️", "obsolete": "➖"}[p.Status]
			fmt.Fprintf(&b, "- %s **%s** — %s", icon, p.Status, p.Title)
			if n := strings.TrimSpace(p.Note); n != "" {
				fmt.Fprintf(&b, ": %s", n)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if len(r.Findings) == 0 {
		if r.Kind == model.KindIncremental {
			b.WriteString("✅ No new issues found.\n")
		} else {
			b.WriteString("✅ No issues found.\n")
		}
	} else {
		if r.Kind == model.KindIncremental {
			b.WriteString("**New findings**\n\n")
		} else {
			b.WriteString("**Findings**\n\n")
		}
		for i, f := range r.Findings {
			loc := f.File
			if f.Line > 0 {
				loc = fmt.Sprintf("%s:%d", f.File, f.Line)
			}
			fmt.Fprintf(&b, "%d. **[%s]** `%s` — %s\n", i+1, f.Severity, loc, f.Title)
			if d := strings.TrimSpace(f.Detail); d != "" {
				for _, line := range strings.Split(d, "\n") {
					fmt.Fprintf(&b, "   %s\n", line)
				}
			}
			b.WriteString("\n")
		}
	}
	fmt.Fprintf(&b, "\n<!-- prmax:%s:%s -->\n", r.ID, r.HeadSHA)
	return b.String()
}
