package wiki

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vgmartinez/tripcord/internal/advisory"
	"github.com/vgmartinez/tripcord/internal/model"
	"github.com/vgmartinez/tripcord/internal/rules"
)

// CatalogMarker is replaced, in docs/rules/README.md, by the tables built
// from rules/*.yaml and rules/ADVISORIES.json.
const CatalogMarker = "<!-- wiki:rules-catalog -->"

// loadSpecs parses every rule file in rulesDir, sorted by id.
func loadSpecs(rulesDir string) ([]*rules.Spec, error) {
	paths, err := filepath.Glob(filepath.Join(rulesDir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	var specs []*rules.Spec
	for _, p := range paths {
		if filepath.Base(p) == "pack.yaml" {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		spec, err := rules.ParseSpec(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.ToSlash(p), err)
		}
		specs = append(specs, spec)
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].ID < specs[j].ID })
	return specs, nil
}

func loadAdvisories(rulesDir string) ([]advisory.Advisory, error) {
	data, err := os.ReadFile(filepath.Join(rulesDir, "ADVISORIES.json"))
	if err != nil {
		return nil, err
	}
	db, err := advisory.Parse(data)
	if err != nil {
		return nil, err
	}
	return db.Current(), nil
}

// catalog renders the rule and advisory tables. Links are relative to
// docs/rules/, so they go through the same rewrite as the prose around
// them. ruleDocs holds the ids that have a docs/rules/<id>.md page.
func catalog(specs []*rules.Spec, advisories []advisory.Advisory, ruleDocs map[string]bool) string {
	var b strings.Builder
	b.WriteString("## Rules\n\n")
	b.WriteString("Generated from `rules/*.yaml`. Severity is the catalog severity; ")
	b.WriteString("a rule with `severity_when` lists the other severities a match can have.\n\n")
	b.WriteString("| ID | Title | Severity | Harness | Fix |\n| --- | --- | --- | --- | --- |\n")
	for _, s := range specs {
		id := s.ID
		if ruleDocs[s.ID] {
			id = fmt.Sprintf("[%s](%s.md)", s.ID, s.ID)
		}
		harness := make([]string, len(s.Harness))
		for i, h := range s.Harness {
			harness[i] = "`" + h + "`"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n",
			id, cell(s.Title), severityCell(s), strings.Join(harness, ", "), cell(s.Fix))
	}

	b.WriteString("\n## Advisories\n\n")
	b.WriteString("Generated from `rules/ADVISORIES.json`, the offline list [TC-CFG-023](TC-CFG-023.md) matches. ")
	b.WriteString("Withdrawn advisories are left out.\n\n")
	b.WriteString("| ID | Aliases | Severity | Affected | Summary | Refs |\n| --- | --- | --- | --- | --- | --- |\n")
	for _, a := range advisories {
		var affected []string
		for _, af := range a.Affected {
			affected = append(affected, affectedCell(af))
		}
		var refs []string
		for _, r := range a.Refs {
			label := r
			if u, err := url.Parse(r); err == nil && u.Host != "" {
				label = u.Host
			}
			refs = append(refs, fmt.Sprintf("[%s](%s)", label, r))
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n",
			a.ID, cell(strings.Join(a.Aliases, ", ")), a.Severity,
			strings.Join(affected, "<br>"), cell(a.Summary), strings.Join(refs, ", "))
	}
	return b.String()
}

func severityCell(s *rules.Spec) string {
	base, err := model.ParseSeverity(s.Severity)
	if err != nil {
		return s.Severity
	}
	seen := map[model.Severity]bool{}
	var other []model.Severity
	for _, c := range s.SeverityWhen {
		sev, err := model.ParseSeverity(c.Severity)
		if err != nil || sev == base || seen[sev] {
			continue
		}
		seen[sev] = true
		other = append(other, sev)
	}
	if len(other) == 0 {
		return s.Severity
	}
	sort.Slice(other, func(i, j int) bool {
		c, _ := other[i].Compare(other[j])
		return c > 0
	})
	names := make([]string, len(other))
	for i, sev := range other {
		names[i] = sev.String()
	}
	return s.Severity + " (or " + strings.Join(names, ", ") + " per match)"
}

func affectedCell(af advisory.Affected) string {
	switch af.Ecosystem {
	case "git":
		commits := make([]string, len(af.AffectedCommits))
		for i, c := range af.AffectedCommits {
			commits[i] = "`" + shortSHA(c) + "`"
		}
		return fmt.Sprintf("git `%s` at %s", af.Repo, strings.Join(commits, ", "))
	case "module":
		var sigs []string
		for _, s := range af.Signatures {
			if s.Module != "" {
				sigs = append(sigs, "`"+s.Module+"`")
			}
			if s.PathSuffix != "" {
				sigs = append(sigs, "`…/"+s.PathSuffix+"`")
			}
		}
		return "module " + strings.Join(sigs, ", ")
	}
	var ranges []string
	for _, r := range af.Ranges {
		var parts []string
		if r.Introduced != "" && r.Introduced != "0" {
			parts = append(parts, ">= "+r.Introduced)
		}
		if r.Fixed != "" {
			parts = append(parts, "< "+r.Fixed)
		}
		if r.LastAffected != "" {
			parts = append(parts, "<= "+r.LastAffected)
		}
		if len(parts) == 0 {
			parts = append(parts, "all versions")
		}
		ranges = append(ranges, strings.Join(parts, ", "))
	}
	return fmt.Sprintf("%s `%s` %s", af.Ecosystem, af.Name, cell(strings.Join(ranges, "; ")))
}

func shortSHA(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}

// cell makes text safe for one Markdown table cell.
func cell(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	s = strings.ReplaceAll(s, "|", `\|`)
	s = strings.ReplaceAll(s, "<", "&lt;")
	return s
}
