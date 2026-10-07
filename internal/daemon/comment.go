package daemon

import (
	"fmt"
	"path"
	"strings"

	"prmax/internal/model"
)

func renderComment(r model.Review) string {
	var b strings.Builder
	meta := fmt.Sprintf("[Round %d](https://github.com/%s/commit/%s)", r.Round, r.Repo, r.HeadSHA)
	if r.SinceSHA != "" {
		meta += fmt.Sprintf(" · since `%s`", short(r.SinceSHA))
		if r.Scope == model.ScopeWhole {
			meta += " · whole PR"
		}
	}
	fmt.Fprintf(&b, "<sub>%s · %s</sub>\n", meta, r.ModelLabel)
	for _, c := range model.Categories {
		fs := r.ByCategory(c.ID)
		if len(fs) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n**%s**\n\n", c.Label)
		for i, f := range fs {
			title := strings.TrimSpace(f.Title)
			if f.StillOpen {
				title += " · *still open*"
			}
			fmt.Fprintf(&b, "%d. **%s** · %s\n", i+1, title, fileLink(r, f))
			for _, line := range strings.Split(strings.TrimSpace(f.Text), "\n") {
				fmt.Fprintf(&b, "   %s\n", line)
			}
			b.WriteString("\n")
		}
	}
	if len(r.Findings) == 0 {
		b.WriteString("\nNo findings.\n")
	}
	out := strings.TrimRight(b.String(), "\n") + "\n"
	return out + fmt.Sprintf("\n<!-- prmax:%s:%s -->\n", r.ID, r.HeadSHA)
}

func fileLink(r model.Review, f model.Finding) string {
	label := path.Base(f.File)
	url := fmt.Sprintf("https://github.com/%s/blob/%s/%s", r.Repo, r.HeadSHA, f.File)
	if f.Line > 0 {
		label = fmt.Sprintf("%s:%d", label, f.Line)
		url += fmt.Sprintf("#L%d", f.Line)
	}
	return fmt.Sprintf("[`%s`](%s)", label, url)
}
