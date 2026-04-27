package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// recommendation is the operator-facing slice of darwin's YAML output.
// Fields are flat (no nested proposed_change) to keep the hand-rolled
// YAML parser simple — see parseRecommendations for the assumptions.
type recommendation struct {
	ID            string
	TargetHarness string
	Type          string
	Rationale     string
	Skill         string
	From          string
	To            string
	Confidence    string
	Reversibility string
}

func runApply(args []string) error {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "print what would change")
	if err := fs.Parse(args); err != nil {
		return err
	}
	pos := fs.Args()
	if len(pos) != 1 {
		return errors.New("usage: darken apply [--dry-run] <recommendation-file>")
	}
	recs, err := parseRecommendations(pos[0])
	if err != nil {
		return err
	}
	for _, r := range recs {
		fmt.Printf("[%s] target=%s type=%s rationale=%s\n", r.ID, r.TargetHarness, r.Type, r.Rationale)
		if *dryRun {
			continue
		}
		fmt.Print("Apply? [y/n/skip/edit] ")
		choice := readChoice()
		switch choice {
		case "y":
			if err := applyRec(r); err != nil {
				return err
			}
		case "edit":
			fmt.Println("(edit mode unimplemented; mark as skip)")
		default:
			fmt.Println("skipped")
		}
	}
	return nil
}

func readChoice() string {
	s := bufio.NewScanner(os.Stdin)
	if s.Scan() {
		return strings.TrimSpace(s.Text())
	}
	return ""
}

// parseRecommendations parses the simple YAML format from spec §12.4.
//
// Hand-rolled to avoid a YAML dep (constitution §I). Assumptions about
// the file shape:
//   - top-level may have a `session:` scalar (ignored) and a single
//     `recommendations:` block sequence
//   - each recommendation begins with `- id: <value>`
//   - subsequent fields are flat scalars at any indent (we trim and
//     match prefix)
//   - `proposed_change:` nested keys (skill / from / to) are flattened
//     into the recommendation since we treat them all as scalars
//   - no anchors, no flow-style sequences, no comments-on-value
//
// If darwin's YAML grows beyond these assumptions, switch to a real
// parser; do not extend this function.
func parseRecommendations(path string) ([]recommendation, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var recs []recommendation
	var cur *recommendation
	for _, line := range strings.Split(string(body), "\n") {
		t := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(t, "- id:"):
			if cur != nil {
				recs = append(recs, *cur)
			}
			cur = &recommendation{ID: trimVal(t, "- id:")}
		case cur == nil:
			continue
		case strings.HasPrefix(t, "target_harness:"):
			cur.TargetHarness = trimVal(t, "target_harness:")
		case strings.HasPrefix(t, "type:"):
			cur.Type = trimVal(t, "type:")
		case strings.HasPrefix(t, "rationale:"):
			cur.Rationale = trimVal(t, "rationale:")
		case strings.HasPrefix(t, "skill:"):
			cur.Skill = trimVal(t, "skill:")
		case strings.HasPrefix(t, "from:"):
			cur.From = trimVal(t, "from:")
		case strings.HasPrefix(t, "to:"):
			cur.To = trimVal(t, "to:")
		case strings.HasPrefix(t, "confidence:"):
			cur.Confidence = trimVal(t, "confidence:")
		case strings.HasPrefix(t, "reversibility:"):
			cur.Reversibility = trimVal(t, "reversibility:")
		}
	}
	if cur != nil {
		recs = append(recs, *cur)
	}
	return recs, nil
}

func trimVal(line, prefix string) string {
	v := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	v = strings.Trim(v, `"'`)
	return v
}

// classifierRatifies is a placeholder hook for the escalation-classifier
// library (Slice 1). Until that lands, it always returns false — every
// recommendation requires explicit operator y/n.
func classifierRatifies(rec recommendation) bool { return false }

func applyRec(r recommendation) error {
	root, err := repoRoot()
	if err != nil {
		return err
	}

	// rule_add carries write authority over the constitution and is
	// never auto-ratified — even by classifierRatifies.
	if r.Type == "rule_add" && !classifierRatifies(r) {
		fmt.Println("rule_add requires explicit operator approval. Continue? [y/N]")
		if c := readChoice(); c != "y" {
			return errors.New("rule_add declined")
		}
	}

	switch r.Type {
	case "skill_add":
		c := exec.Command("bash",
			filepath.Join(root, "scripts", "stage-skills.sh"),
			r.TargetHarness, "--add", r.Skill)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			return err
		}
	case "skill_remove":
		c := exec.Command("bash",
			filepath.Join(root, "scripts", "stage-skills.sh"),
			r.TargetHarness, "--remove", r.Skill)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			return err
		}
	case "skill_upgrade":
		// Re-runs stage-skills.sh <harness>; picks up canonical-source
		// changes. Skill version applied is whatever's in the canonical
		// agent-skills repo at apply time — not pinned.
		c := exec.Command("bash",
			filepath.Join(root, "scripts", "stage-skills.sh"),
			r.TargetHarness)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			return err
		}
	case "model_swap":
		if err := swapModel(root, r.TargetHarness, r.From, r.To); err != nil {
			return err
		}
	case "prompt_edit":
		if err := editPrompt(root, r.TargetHarness, r.From, r.To); err != nil {
			return err
		}
	case "rule_add":
		path := filepath.Join(root, ".specify", "memory", "constitution.md")
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		if _, err := f.WriteString("\n" + r.Rationale + "\n"); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported rec type %q", r.Type)
	}

	// Commit + audit-log every applied change.
	msg := fmt.Sprintf("auto-apply(darwin): %s %s %s", r.ID, r.Type, r.TargetHarness)
	if err := exec.Command("git", "-C", root, "add", "-A").Run(); err != nil {
		return err
	}
	if err := exec.Command("git", "-C", root, "commit", "-m", msg).Run(); err != nil {
		return err
	}
	return appendAudit(root, r)
}

func swapModel(root, harness, from, to string) error {
	manifest := filepath.Join(root, ".scion", "templates", harness, "scion-agent.yaml")
	body, err := substrateResolver().ReadFile(".scion/templates/" + harness + "/scion-agent.yaml")
	if err != nil {
		return err
	}
	out := strings.Replace(string(body),
		"model: "+from, "model: "+to, 1)
	// Write back to the project-local manifest: model_swap mutates the
	// working repo's substrate, never the embedded or override layers.
	return os.WriteFile(manifest, []byte(out), 0o644)
}

// editPrompt does an exact-string before→after replace on the harness's
// system-prompt.md file. Single-occurrence — fails if the before string
// is absent or non-unique.
func editPrompt(root, harness, before, after string) error {
	path := filepath.Join(root, ".scion", "templates", harness, "system-prompt.md")
	body, err := substrateResolver().ReadFile(".scion/templates/" + harness + "/system-prompt.md")
	if err != nil {
		return err
	}
	if !strings.Contains(string(body), before) {
		return fmt.Errorf("prompt_edit: before-string not found in %s", path)
	}
	if strings.Count(string(body), before) > 1 {
		return fmt.Errorf("prompt_edit: before-string non-unique in %s", path)
	}
	out := strings.Replace(string(body), before, after, 1)
	// Write back to the project-local prompt: prompt_edit mutates the
	// working repo's substrate, never the embedded or override layers.
	return os.WriteFile(path, []byte(out), 0o644)
}

// appendAudit writes a JSONL row to .scion/audit.jsonl for each
// applied recommendation. Stdlib encoding/json only.
func appendAudit(root string, r recommendation) error {
	type entry struct {
		Timestamp        string `json:"timestamp"`
		RecommendationID string `json:"recommendation_id"`
		TargetHarness    string `json:"target_harness"`
		Type             string `json:"type"`
		Decision         string `json:"decision"`
		Operator         string `json:"operator"`
	}
	e := entry{
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
		RecommendationID: r.ID,
		TargetHarness:    r.TargetHarness,
		Type:             r.Type,
		Decision:         "applied",
		Operator:         os.Getenv("USER"),
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	path := filepath.Join(root, ".scion", "audit.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}
